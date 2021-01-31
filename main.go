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
	"bytes"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"text/template"
)

type Machine struct {
	Name         string
	UUID         string
	SerialNumber string
	AssetTag     string
	Type         string
	ManagerName  string
}

type templateData struct {
	Machines []Machine
}

//go:embed resources/*.png
var resources embed.FS

//go:embed resources/index.html
var index []byte

var (
	indexTmpl   *template.Template
	tmplFuncMap template.FuncMap
)

func netboxSearchLink(query string) string {
	u := &url.URL{}
	u.Scheme = "https"
	u.Host = "netbox.kamel.network"
	u.Path = "/search/"
	v := url.Values{}
	v.Add("q", query)
	u.RawQuery = v.Encode()
	return u.String()
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "text/html; charset=utf-8")
	d := templateData{}
	d.Machines = append(d.Machines, Machine{
		Name:         "test.kamel.network",
		UUID:         "4de6d578-21ea-4fac-aa79-26ece3425368",
		SerialNumber: "XXXXXXXXXXXX",
		AssetTag:     "00431",
		Type:         "vcenter",
		ManagerName:  "Kista Gate vCenter Manager",
	})
	d.Machines = append(d.Machines, Machine{
		Name:         "data.kamel.network",
		UUID:         "38f5930d-7ca1-4941-bcdc-c0e7864836fb",
		SerialNumber: "Babalbaboo",
		AssetTag:     "00121",
		Type:         "redfish",
		ManagerName:  "Kista Gate Redfish Manager",
	})
	d.Machines = append(d.Machines, Machine{
		Name:         "openvms.kamel.network",
		UUID:         "faac90ab-e4c4-4423-a02d-5d8d223ae9ac",
		SerialNumber: "ALPHA-VX-1A",
		AssetTag:     "00011",
		Type:         "openvms",
		ManagerName:  "Kista Gate AlphaServer Manager",
	})

	b := &bytes.Buffer{}
	if err := indexTmpl.Execute(b, d); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		log.Printf("Template rendering error: %v", err)
		return
	}
	w.Write(b.Bytes())
	return
}

func main() {
	tmplFuncMap = template.FuncMap{
		"NetboxSearch": netboxSearchLink,
	}
	indexTmpl = template.Must(template.New("index").Funcs(tmplFuncMap).Parse(string(index)))
	rf, _ := fs.Sub(resources, "resources")
	fs := http.FileServer(http.FS(rf))
	http.Handle("/r/", http.StripPrefix("/r/", fs))
	http.HandleFunc("/", handleIndex)
	log.Fatal(http.ListenAndServe(":7865", nil))
}
