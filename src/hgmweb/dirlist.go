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


func serveDirectoryList(w http.ResponseWriter, fspath string, pconf proxyParams) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	dirList, _ := ioutil.ReadDir(fspath)
	fmt.Printf(getAssetPath("basic.css", pconf))
	imgBase := "http://tiny.cdn.eqmx.net/icons/tango/16x16/status/"
	io.WriteString(w, "<html><head><meta charset='UTF-8'><meta name='HandheldFriendly' content='True'>");
	io.WriteString(w, "<meta name='MobileOptimized' content='320'></head><body>\n");
	io.WriteString(w, fmt.Sprintf("<img src=\"%s../actions/back.png\"> <a href=../>back</a><br>\n", imgBase))
	
	for fidx := range dirList {
		fi := dirList[fidx]
		linkName := url.QueryEscape(fi.Name())
		htmlName := html.EscapeString(fi.Name())
		desc := fmt.Sprintf("%s/stock_attach.png", imgBase)
		if fi.IsDir() {
			desc = fmt.Sprintf("%s/stock_open.png", imgBase)
			linkName = fmt.Sprintf("%s/", linkName)
		}

		io.WriteString(w, fmt.Sprintf("<img src=\"%s\"> <a href=\"%s\">%s</a><br>\n", desc, linkName, htmlName))
	}
	
	io.WriteString(w, "</hr><br><br><font size=-2><i>Powered by HyperGlobalMegaStore</i></font></body></html>\n")
	
}

