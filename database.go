// Machine temporary database
//
// Copyright (C) 2021  Tateru Authors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/gorilla/mux"
	lru "github.com/hashicorp/golang-lru"
)

type SSHPorts struct {
	Installer int `json:"installer,omitempty"`
}

type Machine struct {
	Name          string   `json:"name,omitempty"`
	UUID          string   `json:"uuid"`
	SerialNumber  string   `json:"serialNumber,omitempty"`
	AssetTag      string   `json:"assetTag,omitempty"`
	Type          string   `json:"type"`
	ManagerName   string   `json:"-"`
	ManagedBy     string   `json:"managedBy"`
	SSHPorts      SSHPorts `json:"sshPorts",omitempty"`
	InstallerAddr string   `json:"installerAddress,omitempty"`

	installRequest *InstallRequest
	mutex          sync.RWMutex
}

func (m *Machine) State() string {
	if m.installRequest == nil {
		return "provisioned"
	}

	return m.installRequest.State.String()
}

// When calling this function, you should hold a read-lock on the db object
func (m *Machine) getInstallRequest(db *tateruDB) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	installRequest, ok := db.installRequests.Peek(m.UUID)
	if ok {
		ir := installRequest.(InstallRequest)
		m.installRequest = &ir

		// TODO: implement a custom JSON encoder instead of manually duplicating attributes
		m.InstallerAddr = m.installRequest.InstallerAddr
		m.SSHPorts = m.installRequest.SSHPorts
	}
}

func NewInstallRequestStateMachine() *StateMachine {
	states := StateMap{
		"provisioned": {
			Events: EventMap{
				"RECEIVED_BOOT_INSTALLER_REQUEST": Event{
					NewState: "pending",
				},
			},
		},
		"pending": {
			Events: EventMap{
				"SENT_BOOT_INSTALLER_REQUEST_TO_MANAGER": Event{
					NewState: "booting",
				},
			},
		},
		"booting": {
			Events: EventMap{
				"RECEIVED_INSTALLER_CALLBACK": Event{
					NewState: "booted",
				},
			},
		},
		"booted": {
			Events: EventMap{
				"RECEIVED_INSTALLER_EXITING_CALLBACK": Event{
					NewState: "provisioned",
				},
			},
		},
	}

	return NewStateMachine("pending", states)
}

type InstallRequest struct {
	Nonce         string
	State         *StateMachine
	SSHPubKey     string
	InstallerAddr string
	SSHPorts      SSHPorts
}

type BootInstallerRequest struct {
	Nonce     string `json:"nonce"`
	SSHPubKey string `json:"ssh_pub_key"`
}

type ManagerBootInstallerRequest struct {
	Nonce string `json:"nonce"`
}

type CallbackRequest struct {
	// Why are Serial Number and AssetTag included here?
	// Should we abort if they do not match what is set on the Machine (via the manager)?
	SerialNumber string   `json:"serialNumber,omitempty"`
	AssetTag     string   `json:"assetTag,omitempty"`
	SSHPorts     SSHPorts `json:"sshPorts,omitempty"`
}

type CallbackResponse struct {
	SSHPubKey string `json:"ssh_pub_key"`
}

type tateruDB struct {
	machinesMutex   sync.RWMutex
	machines        []Machine
	installRequests *lru.Cache
	indexTmpl       *template.Template
}

func (db *tateruDB) GetMachineByUUID(uuid string) (Machine, bool) {
	db.machinesMutex.RLock()
	defer db.machinesMutex.RUnlock()

	for _, m := range db.machines {
		if m.UUID == uuid {
			return m, true
		}
	}

	return Machine{}, false
}
func (db *tateruDB) HandleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "text/html; charset=utf-8")

	var d struct {
		Machines []Machine
	}

	db.machinesMutex.RLock()
	machs := []Machine{}
	for _, m := range db.machines {
		m.getInstallRequest(db)
		machs = append(machs, m)
	}
	d.Machines = machs

	b := &bytes.Buffer{}
	if err := db.indexTmpl.Execute(b, d); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		log.Printf("Template rendering error: %v", err)
		return
	}
	db.machinesMutex.RUnlock()

	w.Write(b.Bytes())
	return
}

func (db *tateruDB) Poll() {
	log.Printf("Polling of managers started")
	for {
		machs := []Machine{}
		for maddr, mcfg := range cfg.Managers {
			u, err := url.Parse(maddr)
			if err != nil {
				panic(err)
			}
			u.Path += "/v1/machines"
			resp, err := http.Get(u.String())
			if err != nil {
				log.Printf("Poll of %q failed: %v", mcfg.Name, err)
				continue
			}
			if resp.StatusCode != 200 {
				log.Printf("Poll of %q failed: status code %d", mcfg.Name, resp.StatusCode)
				continue
			}
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Printf("Poll read of %q failed: %v", mcfg.Name, err)
				continue
			}
			var mr []struct {
				UUID         string
				SerialNumber string
				AssetTag     string
				Name         string
			}
			if err := json.Unmarshal(body, &mr); err != nil {
				log.Printf("Poll of %q failed to parse: %v", mcfg.Name, err)
				continue
			}
			for _, m := range mr {
				machs = append(machs, Machine{
					Name:         m.Name,
					SerialNumber: m.SerialNumber,
					AssetTag:     m.AssetTag,
					UUID:         m.UUID,
					ManagerName:  mcfg.Name,
					Type:         mcfg.Type,
					ManagedBy:    maddr,
				})
			}
		}
		db.machinesMutex.Lock()
		db.machines = machs
		db.machinesMutex.Unlock()

		time.Sleep(time.Second * 30)
	}
}

func (db *tateruDB) HandleMachinesAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "application/json; charset=utf-8")

	// Filter machines if applicable
	var filterAlias string
	query := r.URL.Query()
	if query.Has("alias") {
		// Only use one (the first) alias
		filterAlias = query.Get("alias")
	}

	db.machinesMutex.RLock()
	machines := []Machine{}
	for _, machine := range db.machines {
		if filterAlias != "" && filterAlias != machine.Name {
			continue
		}

		machine.getInstallRequest(db)

		machines = append(machines, machine)
	}

	b, err := json.MarshalIndent(machines, "", " ")
	db.machinesMutex.RUnlock()
	if err != nil {
		http.Error(w, "Failed to render JSON", http.StatusInternalServerError)
		log.Printf("Failed to marshal machines JSON: %v", err)
		return
	}

	w.Write(b)
	return
}

func (db *tateruDB) HandleFetchMachineAPI(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	uuid := vars["uuid"]

	machine, found := db.GetMachineByUUID(uuid)
	if !found {
		http.Error(w, "No machine with this UUID found", http.StatusNotFound)
		return
	}

	machine.mutex.RLock()
	b, err := json.MarshalIndent(machine, "", " ")
	machine.mutex.RUnlock()
	if err != nil {
		http.Error(w, "Failed to render JSON", http.StatusInternalServerError)
		log.Printf("Failed to marshal machine JSON: %v", err)
		return
	}

	w.Header().Add("content-type", "application/json; charset=utf-8")
	w.Write(b)
	return
}

func (db *tateruDB) HandleBootInstallerAPI(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	uuid := vars["uuid"]

	machine, found := db.GetMachineByUUID(uuid)
	if !found {
		http.Error(w, "No machine with this UUID found", http.StatusNotFound)
		return
	}

	// Parse payload
	var bir BootInstallerRequest
	err := json.NewDecoder(r.Body).Decode(&bir)
	if err != nil {
		http.Error(w, "Unable to parse request body as BootInstallerRequest", http.StatusUnprocessableEntity)
		log.Printf("Unable to parse request body as BootInstallerRequest: %v", err)
		return
	}

	machine.getInstallRequest(db)
	// Create (new) installRequest when there is none or a new nonce is used
	machine.mutex.Lock()
	if machine.installRequest == nil || machine.installRequest.Nonce != bir.Nonce {
		installRequest := InstallRequest{
			State:     NewInstallRequestStateMachine(),
			SSHPubKey: bir.SSHPubKey,
			Nonce:     bir.Nonce,
		}
		db.installRequests.Add(uuid, installRequest)
		machine.installRequest = &installRequest
	}
	machine.mutex.Unlock()

	// Send boot-installer request to manager for machine
	managerBir := ManagerBootInstallerRequest{
		Nonce: bir.Nonce,
	}
	b, err := json.MarshalIndent(managerBir, "", " ")
	if err != nil {
		http.Error(w, "Failed to generate BootInstallerRequest for manager", http.StatusInternalServerError)
		log.Printf("Failed to marshal ManagerBootInstallRequest JSON: %v", err)
		return
	}

	client := &http.Client{}
	machine.mutex.RLock()
	bootInstallerURL := fmt.Sprintf("%s/v1/machines/%s/boot-installer", machine.ManagedBy, machine.UUID)
	machine.mutex.RUnlock()
	resp, err := client.Post(bootInstallerURL, "application/json", bytes.NewBuffer(b))
	if err != nil {
		http.Error(w, "Error when sending boot-installer request to manager", http.StatusInternalServerError)
		log.Printf("Error when sending boot-installer request to manager: %v", err)
		return
	}

	if resp.StatusCode != 200 {
		http.Error(w, "Manager could not successfully process boot-installer request", http.StatusInternalServerError)
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Manager replied with '%s' to boot-installer request. Error reading response: %s", resp.Status, err)
		} else {
			log.Printf("Manager replied with '%s' to boot-installer request: %s", resp.Status, body)
		}
		return
	}

	machine.mutex.Lock()
	machine.installRequest.State.Transition("SENT_BOOT_INSTALLER_REQUEST_TO_MANAGER")
	// Update cached installRequest
	db.installRequests.Add(uuid, *(machine.installRequest))
	machine.mutex.Unlock()

	machine.installRequest.State.WaitFor("booted")

	return
}

func (db *tateruDB) HandleInstallerCallbackAPI(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	uuid := vars["uuid"]

	machine, found := db.GetMachineByUUID(uuid)
	if !found {
		http.Error(w, "No machine with this UUID found", http.StatusNotFound)
		return
	}

	machine.getInstallRequest(db)
	machine.mutex.RLock()
	if machine.installRequest == nil {
		machine.mutex.RUnlock()
		http.Error(w, "No install request found for this machine", http.StatusNotFound)
		log.Printf("Received installer callback for machine '%s', but there was previous InstallerRequest", machine.UUID)
		return
	}
	machine.mutex.RUnlock()

	// Parse payload
	var cr CallbackRequest
	err := json.NewDecoder(r.Body).Decode(&cr)
	if err != nil {
		http.Error(w, "Unable to parse request body as CallbackRequest", http.StatusUnprocessableEntity)
		log.Printf("Unable to parse request body as CallbackRequest: %v", err)
		return
	}

	// Update installRequest with information
	installerAddr := r.Header.Get("X-Forwarded-For")
	if installerAddr == "" {
		installerAddr = r.RemoteAddr
		if strings.HasPrefix(installerAddr, "[") {
			parts := strings.SplitN(installerAddr, "]:", 2)
			installerAddr = strings.Trim(parts[0], "[")
		} else {
			parts := strings.SplitN(installerAddr, ":", 2)
			installerAddr = parts[0]
		}
	}
	machine.mutex.Lock()
	machine.installRequest.InstallerAddr = installerAddr
	machine.installRequest.SSHPorts = cr.SSHPorts
	machine.installRequest.State.Transition("RECEIVED_INSTALLER_CALLBACK")
	// Update cached installRequest
	db.installRequests.Add(uuid, *(machine.installRequest))

	cresp := CallbackResponse{
		SSHPubKey: machine.installRequest.SSHPubKey,
	}
	b, err := json.MarshalIndent(cresp, "", " ")
	machine.mutex.Unlock()
	if err != nil {
		http.Error(w, "Failed to render JSON", http.StatusInternalServerError)
		log.Printf("Failed to marshal CallbackResponse JSON: %v", err)
		return
	}

	w.Header().Add("content-type", "application/json; charset=utf-8")
	w.Write(b)
	return
}

func (db *tateruDB) installRequestEvict(key interface{}, value interface{}) {
	installRequest := value.(*InstallRequest)

	state := installRequest.State.Current()
	if state != "provisioned" {
		log.Printf("ERROR: InstallRequest for %s was evicted while still running (state was %s)!", key.(string), state)
	}
}
