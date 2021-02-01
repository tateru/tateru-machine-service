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
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sync"
	"text/template"
	"time"
)

type Machine struct {
	Name         string `json:"name,omitempty"`
	UUID         string `json:"uuid"`
	SerialNumber string `json:"serialNumber,omitempty"`
	AssetTag     string `json:"assetTag,omitempty"`
	Type         string `json:"type"`
	ManagerName  string `json:"-"`
	ManagedBy    string `json:"managedBy"`
}

type tateruDb struct {
	machinesMutex sync.RWMutex
	machines      []Machine
	indexTmpl     *template.Template
}

func (db *tateruDb) HandleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "text/html; charset=utf-8")

	var d struct {
		Machines []Machine
	}
	db.machinesMutex.RLock()
	d.Machines = db.machines

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

func (db *tateruDb) Poll() {
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

func (db *tateruDb) HandleMachinesAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "application/json; charset=utf-8")

	if r.Method != "GET" {
		http.Error(w, "Unsupported method", http.StatusMethodNotAllowed)
		return
	}

	// TODO: Add filter of alias query param

	db.machinesMutex.RLock()
	b, err := json.MarshalIndent(db.machines, "", " ")
	if err != nil {
		http.Error(w, "Failed to render JSON", http.StatusInternalServerError)
		log.Printf("Failed to marshal machines JSON: %v", err)
		return
	}
	db.machinesMutex.RUnlock()
	w.Write(b)
	return
}
