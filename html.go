// HTML template functions
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
	"net/url"
	"text/template"
)

var (
	tmplFuncMap template.FuncMap
)

func netboxSearchLink(query string) string {
	u, err := url.Parse(cfg.Netbox)
	if err != nil {
		return "#"
	}
	u.Path += "/search/"
	v := url.Values{}
	v.Add("q", query)
	u.RawQuery = v.Encode()
	return u.String()
}

func initTemplate(name string, page []byte) *template.Template {
	tmplFuncMap = template.FuncMap{
		"NetboxSearch": netboxSearchLink,
	}
	return template.Must(template.New(name).Funcs(tmplFuncMap).Parse(string(page)))
}
