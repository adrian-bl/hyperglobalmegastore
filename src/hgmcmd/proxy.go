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
	"html"
	"io"
	"io/ioutil"
	"os"
	"libhgms/crypto/aestool"
	"libhgms/flickr/png"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

/* Custom HTTP Client, setup done in main() */
var backendClient *http.Client
var reHttpRange = regexp.MustCompile("^bytes=([0-9]+)-$");

/* Prefix to use for request handler */
var wwwPrefix = "";

type RqMeta struct {
	Location [][]string /* Raw HTTP URL            */
	Key      string     /* 7-bit ascii hex string  */
	Created int64       /* file-creation timestamp */
	BlobSize int64
	RangeFrom int64
}


func LaunchProxy(bindAddr string, bindPort string, rqPrefix string) {
	tr := &http.Transport{ResponseHeaderTimeout: 5 * time.Second, Proxy: http.ProxyFromEnvironment}
	backendClient = &http.Client{Transport: tr}
	wwwPrefix = rqPrefix;

	/* Fixme: IPv6 and basic validation (port 0 should be refused */
	bindString := fmt.Sprintf("%s:%s", bindAddr, bindPort);
	fmt.Printf("Proxy accepting connections at http://%s/%s\n", bindString, wwwPrefix)

	http.HandleFunc(fmt.Sprintf("/%s", wwwPrefix), handleAlias)
	http.ListenAndServe(bindString, nil)
}

func handleAlias(w http.ResponseWriter, r *http.Request) {
	
	unEscapedRqUri, err := url.QueryUnescape(r.RequestURI)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Failed to parse URI")
		return
	}
	
	unEscapedRqUri = unEscapedRqUri[len(wwwPrefix):];
	
	aliasPath := fmt.Sprintf("./_aliases/%s", unEscapedRqUri)
	fmt.Printf("+ GET <%s> (raw: %s)\n", aliasPath, r.RequestURI)

	fi, err := os.Stat(aliasPath)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "File not found\n")
		return
	}

	if fi.IsDir() {
		/* primitive redirect: fixme: what happens behind a reverse proxy? */
		if aliasPath[len(aliasPath)-1:] != "/" {
			http.Redirect(w, r, r.RequestURI+"/", http.StatusFound)
			return
		}

		/* check if we have an index.html */
		idxAliasPath := aliasPath+"/index.html"
		_, err := os.Stat(idxAliasPath)

		if err != nil {
			/* no index, handle dirlist: */
			if 1 == 0 {
				w.WriteHeader(http.StatusForbidden)
				io.WriteString(w, "Directory listing disabled\n")
			} else {
				writeDirectoryList(w, aliasPath)
			}
			return
		} else {
			aliasPath = idxAliasPath
		}
	}
	
	/* normal file */
	content, err := ioutil.ReadFile(aliasPath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Failed to read json file")
		return
	}
	
	var js RqMeta
	err = json.Unmarshal([]byte(content), &js)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Corrupted metadata")
		return
	}

	clientIMS, _ := time.Parse(http.TimeFormat, r.Header.Get("If-Modified-Since"))
	if clientIMS.Unix() > 0 && js.Created <= clientIMS.Unix() {
		w.WriteHeader(http.StatusNotModified)
		io.WriteString(w, "Not modified")
		return
	}
	
	rangeMatches := reHttpRange.FindStringSubmatch(r.Header.Get("Range"))
	if len(rangeMatches) == 2 {/* [0]=text, [1]=range_bytes */
		js.RangeFrom, _ = strconv.ParseInt(rangeMatches[1], 10, 64)
	} else {
		js.RangeFrom = 0
	}
	
	fmt.Printf(">>>>>> %d >>> %s\n", js.RangeFrom, r.Header.Get("Range"))
	
	/* We got all required info: serve HTTP request to client */
	serveFullURI(w, r, js)
}

func writeDirectoryList(w http.ResponseWriter, fspath string) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	dirList, _ := ioutil.ReadDir(fspath)
	
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

/*
 * Handle request for given targetURI
 */

func serveFullURI(dst http.ResponseWriter, rq *http.Request, rqm RqMeta) {

	/* Our encryption key is stored as an hex-ascii string
	 * within the JSON file */
	key := make([]byte, len(rqm.Key)/2)
	hex.Decode(key, []byte(rqm.Key))

	headersSent := false                /* True if we already sent the http header */
	locArray := rqm.Location            /* Array with all blob locations           */
	numCopies := len(locArray)          /* Number of replicas in rqmeta            */
	numBlobs := int64(len(locArray[0])) /* Total number of blobs                   */
	skipBytes := rqm.RangeFrom          /* How many bytes shall we throw away?     */

/* fixme: div-by-zero: should we care? */
	bIdx := int64( skipBytes / rqm.BlobSize )
	skipBytes -= bIdx * rqm.BlobSize

	fmt.Printf("# stream has %d location(s) and %d chunks, firstBlob is: %d, skip=%d\n", numCopies, numBlobs, bIdx, skipBytes)

	for ; bIdx<numBlobs; bIdx++ {
		copyList := rand.Perm(numCopies)
		fmt.Printf("== serving blob %d/%d\n", bIdx+1, numBlobs);

		servedCopy := false
		for _, ci := range copyList {
			currentURI := locArray[ci][bIdx]
			fmt.Printf("  >> replica %d -> checking %s\n", ci, currentURI)

			backendRQ, err := http.NewRequest("GET", currentURI, nil)
			if err != nil {
				continue
			}

			backendResp, err := backendClient.Do(backendRQ)
			if err != nil {
				continue
			}

			pngReader, err := flickr.NewReader(backendResp.Body)
			if err != nil {
				backendResp.Body.Close()
				continue
			}

			err = pngReader.InitReader()
			if err != nil {
				backendResp.Body.Close()
				continue
			}

			if headersSent == false {
				headersSent = true
				dst.Header().Set("Last-Modified", time.Unix(rqm.Created, 0).Format(http.TimeFormat))
				
				if rqm.RangeFrom == 0 {
					dst.Header().Set("Content-Length", fmt.Sprintf("%d", pngReader.ContentSize))
					dst.WriteHeader(http.StatusOK)
				} else {
					dst.Header().Set("Content-Length", fmt.Sprintf("%d", pngReader.ContentSize - rqm.RangeFrom))
					dst.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", rqm.RangeFrom, pngReader.ContentSize-1, pngReader.ContentSize))
					dst.WriteHeader(http.StatusPartialContent)
				}
			}

			aes, err := aestool.New(pngReader.BlobSize, key, pngReader.IV)
			if err != nil {
				panic(err) /* this would most likely be a bug in pngReader.InitReader() */
			}

			fmt.Printf("  >> replica %d is ok, starting copy stream..., sb=%d\n", ci, skipBytes)
			aes.SetSkipBytes(&skipBytes)
			err = aes.DecryptStream(dst, pngReader)
			backendResp.Body.Close()

			if err != nil {
				panic(err)
			}

			servedCopy = true
			break
		}

		if servedCopy == false {
			if headersSent == false {
				dst.WriteHeader(http.StatusInternalServerError)
				io.WriteString(dst, "Internal server error :-(\n")
			}
			fmt.Printf("failed to deliver blob %d, aborting request\n", bIdx+1)
			break
		}
	}

}
