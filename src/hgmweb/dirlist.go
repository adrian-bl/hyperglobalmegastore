/*
 * Copyright (C) 2013-2014 Adrian Ulrich <adrian@blinkenlights.ch>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program. If not, see <http://www.gnu.org/licenses/>.
 */

package hgmweb

import (
	"fmt"
	"html"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
)

func serveDirectoryList(w http.ResponseWriter, fspath string, pconf *proxyParams) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	dirList, _ := ioutil.ReadDir(fspath)

	io.WriteString(w, `<html><head><meta charset="UTF-8"><meta name="HandheldFriendly" content="True"><meta name='MobileOptimized' content='320'>`)
	io.WriteString(w, fmt.Sprintf("<link rel=\"stylesheet\" type=\"text/css\" href=\"%s\">", getAssetPath("basic.css", pconf)))
	io.WriteString(w, `</head><body>`)

	/* begin filelist table */
	io.WriteString(w, `<table class="pure-table-horizontal pure-table pure-table-striped">`)
	io.WriteString(w, `<thead><tr onclick="document.location='../';"><th class="highlight"><span class='entypo-left' /></th><th class="highlight"><i>Back</i></th></tr></thead>`)

	io.WriteString(w, `<tbody>`)
	for fidx := range dirList {
		fi := dirList[fidx]
		linkName := url.QueryEscape(fi.Name())
		htmlName := html.EscapeString(fi.Name())
		linkIcon := "floppy"
		if fi.IsDir() {
			linkIcon = "folder"
			linkName = fmt.Sprintf("%s/", linkName)
		}
		io.WriteString(w, fmt.Sprintf("<tr onclick=\"document.location='%s';\"><td><span class='entypo-%s' /></td><td>%s</td></tr>", linkName, linkIcon, htmlName))
	}
	/* end filelist table */
	io.WriteString(w, `</tbody></table>`)

	io.WriteString(w, `</body></html>`)

}
