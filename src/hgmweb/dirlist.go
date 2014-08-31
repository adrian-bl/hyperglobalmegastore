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
	"regexp"
)

var reIsMovie = regexp.MustCompile("(?i)\\.(mkv|avi|mp4|m4v|mpeg)$")
var reIsMusic = regexp.MustCompile("(?i)\\.(mp3|ogg|flac|m4a|wav)$")
var reIsPicture = regexp.MustCompile("(?i)\\.(jpeg|jpg|gif|png|bmp)$")

func serveDirectoryList(w http.ResponseWriter, fspath string, pconf *proxyParams) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	dirList, _ := ioutil.ReadDir(fspath)

	io.WriteString(w, `<!doctype html><html lang="en"><head><title>HGMS</title><meta charset="UTF-8"><meta name="HandheldFriendly" content="True"><meta name='MobileOptimized' content='320'>`)
	io.WriteString(w, fmt.Sprintf("<link rel=\"stylesheet\" type=\"text/css\" href=\"%s\">", getAssetPath("basic.css", pconf)))
	io.WriteString(w, `</head><body><div class="hgms-wrapper">`)

	io.WriteString(w, getCell("entypo-left", "../", "<i>Back</i>", "cb"));

	i := 0
	for fidx := range dirList {
		fi := dirList[fidx]
		linkName := url.QueryEscape(fi.Name())
		htmlName := html.EscapeString(fi.Name())
		linkIcon := "entypo-docs"
		if fi.IsDir() {
			linkIcon = "entypo-folder"
			linkName = fmt.Sprintf("%s/", linkName)
		} else if reIsMovie.MatchString(htmlName) {
			linkIcon = "entypo-video"
		} else if reIsMusic.MatchString(htmlName) {
			linkIcon = "entypo-note-beamed"
		} else if reIsPicture.MatchString(htmlName) {
			linkIcon = "entypo-picture"
		}
		i++;
		colorClass := "fc"
		if i % 2 == 0 { colorClass = "f0" }
		io.WriteString(w, getCell(linkIcon, linkName, htmlName, colorClass))
	}

	io.WriteString(w, `<br><i>Powered by HyperGlobalMegaStore <span class='entypo-infinity'></span></i>`)

	io.WriteString(w, `</div></body></html>`)

}



func getCell(iconName string, linkHref string, htmlName string, colorClass string) string {

	return fmt.Sprintf("<a href=\"%s\"><div class=\"pure-g g-color-%s\">"+
	"<div class=\"pure-u-1-24\"><div class=\"g-padding\"><span class=\"%s\"></span></div></div>"+
	"<div class=\"pure-u-1-24\"></div>"+
	"<div class=\"pure-u-22-24\"><div class=\"g-padding\">%s</div></div>"+
	"</div></a>", linkHref, colorClass, iconName, htmlName);
}

