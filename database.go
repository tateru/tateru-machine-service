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
	"log"
	"net/http"
	"sync"
	"text/template"
	"time"
)

type Machine struct {
	Name         string
	UUID         string
	SerialNumber string
	AssetTag     string
	Type         string
	ManagerName  string
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
	for {
		machs := []Machine{}
		for maddr, mcfg := range cfg.Managers {
			log.Printf("Polling %q @ %s", mcfg.Name, maddr)
			machs = append(machs, Machine{
				Name:         "test.kamel.network",
				UUID:         "4de6d578-21ea-4fac-aa79-26ece3425368",
				SerialNumber: "XXXXXXXXXXXX",
				AssetTag:     "00431",
				Type:         mcfg.Type,
				ManagerName:  mcfg.Name,
			})
		}
		db.machinesMutex.Lock()
		db.machines = machs
		db.machinesMutex.Unlock()
		time.Sleep(time.Second * 30)
	}
}
