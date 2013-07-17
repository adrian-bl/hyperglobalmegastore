package main

import (
	"net/http"
	"fmt"
	"io"
	"io/ioutil"
	"time"
	"encoding/json"
	"libhgms/flickr/png"
	"libhgms/crypto/aestool"
)

/* Custom HTTP Client, setup done in main() */
var backendClient *http.Client

type AliasJSON struct {
	Location string
	Key string
}


func LaunchProxy(bindAddr string, bindPort string) {
	tr := &http.Transport{ ResponseHeaderTimeout: 5*time.Second }
	backendClient = &http.Client{Transport: tr}
	http.HandleFunc("/_raw/", handleRaw)
	
	http.HandleFunc("/", handleAlias)
	http.ListenAndServe(":8080", nil) 
}

func handleAlias(w http.ResponseWriter, r *http.Request) {
	/* fixme: directory traversal */
	aliasPath := fmt.Sprintf("./_aliases/%s", r.RequestURI)
	
	fmt.Printf("GET %s -> %s\n", r.RequestURI, aliasPath)
	
	content, err := ioutil.ReadFile(aliasPath);
	
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "File not found\n")
		return
	}
	
	var jsx AliasJSON
	err = json.Unmarshal([]byte(content), &jsx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "corrupted metadata\n");
		fmt.Printf("json error: %s\n", err)
		return
	}
	
	target := fmt.Sprintf("http://%s", jsx.Location)
	serveFullURI(w, r, jsx.Key, target)
}


/*
 * Handles a request in /HOSTNAME/URI format
 */
func handleRaw(w http.ResponseWriter, r *http.Request) {
	fmt.Printf(">> %s\n", r.RequestURI)
	target := fmt.Sprintf("http://%s", r.RequestURI[6:])
	serveFullURI(w, r, "wurstsalat", target)
}

/*
 * Handle request for given targetURI
 */

func serveFullURI(dst http.ResponseWriter, rq *http.Request, key string, startURI string) {
	
	currentURI := startURI
	fmt.Printf("======================================================\n")
	for i:=0; ; i++ {
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
				io.WriteString(dst, "Backend down")
			}
			break
		}
		
		pngReader, err := flickr.NewReader(backendResp.Body)
		if err != nil { panic(err) }
		pngReader.InitReader()
		
		if i == 0 {
			dst.Header().Set("Content-Length", fmt.Sprintf("%d", pngReader.ContentSize))
			dst.WriteHeader(backendResp.StatusCode)
		}
		
		aes, _ := aestool.New(pngReader.KeySize, pngReader.BlobSize, []byte(key), pngReader.IV);
		err = aes.DecryptStream(dst, pngReader)
		
		backendResp.Body.Close()
		
		if err == nil && pngReader.NextBlob != "" {
			currentURI = pngReader.NextBlob
		} else {
			break
		}
	}
	
	fmt.Printf("== http request finished\n")
}
