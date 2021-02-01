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
	db := &tateruDb{indexTmpl: indexTmpl}
	go db.Poll()
	rf, _ := fs.Sub(resources, "resources")
	fs := http.FileServer(http.FS(rf))
	http.Handle("/r/", http.StripPrefix("/r/", fs))
	http.HandleFunc("/v1/machines", db.HandleMachinesAPI)
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		// The "/" pattern matches everything, so we need to check
		// that we're at the root here.
		if req.URL.Path != "/" {
			log.Printf("Received request to unmapped path: %q", req.URL.Path)
			http.NotFound(w, req)
			return
		}
		db.HandleIndex(w, req)
	})
	log.Fatal(http.ListenAndServe("[::]:7865", nil))
}
