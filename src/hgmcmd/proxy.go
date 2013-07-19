package main

import (
	"net/http"
	"fmt"
	"io"
	"io/ioutil"
	"time"
	"encoding/json"
	"encoding/hex"
	"libhgms/flickr/png"
	"libhgms/crypto/aestool"
)

/* Custom HTTP Client, setup done in main() */
var backendClient *http.Client

type AliasJSON struct {
	Location [][]string  /* Raw HTTP URL           */
	Key string           /* 7-bit ascii hex string */
}


func LaunchProxy(bindAddr string, bindPort string) {
	tr := &http.Transport{ ResponseHeaderTimeout: 5*time.Second }
	backendClient = &http.Client{Transport: tr}
	
	http.HandleFunc("/", handleAlias)
	http.ListenAndServe(":8080", nil) 
}

func handleAlias(w http.ResponseWriter, r *http.Request) {
	/* fixme: directory traversal */
	aliasPath := fmt.Sprintf("./_aliases/%s", r.RequestURI)
	
	fmt.Printf("+ GET %s\n", aliasPath)
	
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
	
	/* Our encryption key is stored as an hex-ascii string
	 * within the JSON file */
	byteKey := make([]byte, len(jsx.Key)/2)
	hex.Decode(byteKey, []byte(jsx.Key))
	
	/* We got all required info: serve HTTP request to client */
	serveFullURI(w, r, byteKey, jsx.Location)
}


/*
 * Handle request for given targetURI
 */

func serveFullURI(dst http.ResponseWriter, rq *http.Request, key []byte, locArray [][]string) {
	
	for i:=0; ; i++ {
		
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
		
		fmt.Printf(">KeySize =%, ivsize=%dd\n", len(key), len(pngReader.IV))
		
		aes, _ := aestool.New(pngReader.BlobSize, key, pngReader.IV);
		err = aes.DecryptStream(dst, pngReader)
		
		backendResp.Body.Close()
		
		if err != nil || len(locArray[0]) == i+1 {
			break
		}
	}
}
