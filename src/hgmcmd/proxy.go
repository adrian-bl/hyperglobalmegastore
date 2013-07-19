/*
 * Copyright (C) 2013 Adrian Ulrich <adrian@blinkenlights.ch>
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

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"libhgms/crypto/aestool"
	"libhgms/flickr/png"
	"net/http"
	"net/url"
	"time"
)

/* Custom HTTP Client, setup done in main() */
var backendClient *http.Client

type AliasJSON struct {
	Location [][]string /* Raw HTTP URL           */
	Key      string     /* 7-bit ascii hex string */
}

func LaunchProxy(bindAddr string, bindPort string) {
	tr := &http.Transport{ResponseHeaderTimeout: 5 * time.Second}
	backendClient = &http.Client{Transport: tr}

	/* Fixme: IPv6 and basic validation (port 0 should be refused */
	bindString := fmt.Sprintf("%s:%s", bindAddr, bindPort);
	fmt.Printf("Proxy accepting connections at http://%s\n", bindString)

	http.HandleFunc("/", handleAlias)
	http.ListenAndServe(bindString, nil)
}

func handleAlias(w http.ResponseWriter, r *http.Request) {
	/* passing this directly to the FS should be ok:
	 * the http package won't accept paths to ../../, but a basic cleanup wouldn't hurt (FIXME) */
	aliasPath := fmt.Sprintf("./_aliases/%s", r.RequestURI)

	fmt.Printf("+ GET %s\n", aliasPath)

	fi, err := os.Stat(aliasPath)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "File not found\n")
		return
	}

	if fi.IsDir() {
		if 1 == 0 {
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, "Directory listing disabled\n")
		} else {
			writeDirectoryList(w, aliasPath)
		}
		return
	}
	
	/* normal file */
	content, err := ioutil.ReadFile(aliasPath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Failed to read json file")
		return
	}
	
	var js AliasJSON
	err = json.Unmarshal([]byte(content), &js)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Corrupted metadata")
		return
	}

	/* Our encryption key is stored as an hex-ascii string
	 * within the JSON file */
	byteKey := make([]byte, len(js.Key)/2)
	hex.Decode(byteKey, []byte(js.Key))

	/* We got all required info: serve HTTP request to client */
	serveFullURI(w, r, byteKey, js.Location)
}

func writeDirectoryList(w http.ResponseWriter, fspath string) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	dirList, _ := ioutil.ReadDir(fspath)
	
	imgBase := "http://tiny.cdn.eqmx.net/icons/tango/16x16/status/"
	
	io.WriteString(w, "<html><head><body><h3>Index of /neverland</h3><hr>\n")
	
	io.WriteString(w, fmt.Sprintf("<img src=\"%s../actions/back.png\"> <a href=../>back</a><br>\n", imgBase))
	
	for fidx := range dirList {
		fi := dirList[fidx]
		saveName := url.QueryEscape(fi.Name())
		desc := fmt.Sprintf("%s/stock_attach.png", imgBase)
		if fi.IsDir() {
			desc = fmt.Sprintf("%s/stock_open.png", imgBase)
			saveName = fmt.Sprintf("%s/", saveName)
		}

		io.WriteString(w, fmt.Sprintf("<img src=\"%s\"> <a href=\"%s\">%s</a><br>\n", desc, saveName, saveName))
	}
	
	io.WriteString(w, "</hr></body></html>\n")
	
}

/*
 * Handle request for given targetURI
 */

func serveFullURI(dst http.ResponseWriter, rq *http.Request, key []byte, locArray [][]string) {

	for i := 0; ; i++ {

		currentURI := locArray[0][i]

		fmt.Printf("PART %d OF STREAM -> %s\n", i, currentURI)

		backendRQ, err := http.NewRequest("GET", currentURI, nil)
		if err != nil {
			if i == 0 {
				dst.WriteHeader(http.StatusInternalServerError)
				io.WriteString(dst, "Internal server errror :-(\n")
			}
			break
		}

		backendResp, err := backendClient.Do(backendRQ)
		if err != nil {
			if i == 0 {
				dst.WriteHeader(http.StatusServiceUnavailable)
				io.WriteString(dst, "Could not connect to remote server\n")
			}
			break
		}

		pngReader, err := flickr.NewReader(backendResp.Body)
		if err != nil {
			panic(err)
		}
		pngReader.InitReader()

		if i == 0 {
			dst.Header().Set("Content-Length", fmt.Sprintf("%d", pngReader.ContentSize))
			dst.WriteHeader(backendResp.StatusCode)
		}

		aes, _ := aestool.New(pngReader.BlobSize, key, pngReader.IV)
		err = aes.DecryptStream(dst, pngReader)

		backendResp.Body.Close()

		if err != nil || len(locArray[0]) == i+1 {
			break
		}
	}
}
