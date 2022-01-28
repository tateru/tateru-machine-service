// Tateru Machine Service main program
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
	"embed"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"gopkg.in/gorilla/handlers.v1"
	"gopkg.in/gorilla/mux.v1"
	"gopkg.in/yaml.v2"
)

type msConfig struct {
	Netbox   string
	Managers map[string]struct {
		Name string
		Type string
	}
}

//go:embed resources/*.png
var resources embed.FS

//go:embed resources/index.html
var index []byte

var cfg *msConfig

func main() {
	log.Printf("Tateru Machine Service starting...")
	indexTmpl := initTemplate("index", index)
	cfile, err := ioutil.ReadFile("machine-service.yml")
	if err != nil {
		log.Fatalf("config read error: %v", err)
	}
	cfg = &msConfig{}
	if err := yaml.Unmarshal([]byte(cfile), &cfg); err != nil {
		log.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	db := &tateruDB{indexTmpl: indexTmpl, installRequests: make(map[string]InstallRequest)}
	go db.Poll()
	router := mux.NewRouter()
	rf, _ := fs.Sub(resources, "resources")
	router.PathPrefix("/r/").Handler(http.StripPrefix("/r/", http.FileServer(http.FS(rf))))
	router.HandleFunc("/v1/machines", db.HandleMachinesAPI).Methods("GET")
	router.HandleFunc("/v1/machines/{uuid}", db.HandleFetchMachineAPI).Methods("GET")
	router.HandleFunc("/v1/machines/{uuid}/boot-installer", db.HandleBootInstallerAPI).Methods("POST")
	router.HandleFunc("/v1/machines/{uuid}/installer-callback", db.HandleInstallerCallbackAPI).Methods("POST")
	router.HandleFunc("/", db.HandleIndex).Methods("GET")
	log.Fatal(http.ListenAndServe("[::]:7865", handlers.LoggingHandler(os.Stdout, router)))
}
