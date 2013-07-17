package main

import (
	"net/http"
	"fmt"
	"io"
	"time"
	"libhgms/flickr/png"
	"libhgms/crypto/aestool"
)

/* Custom HTTP Client, setup done in main() */
var backendClient *http.Client



func LaunchProxy(bindAddr string, bindPort string) {
	tr := &http.Transport{ ResponseHeaderTimeout: 5*time.Second }
	backendClient = &http.Client{Transport: tr}
	http.HandleFunc("/_raw/", handleRaw)
	http.HandleFunc("/", handleAny)
	http.ListenAndServe(":8080", nil) 
}

func handleAny(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "Welcome to Hyper Global Mega Store\n")
}

/*
 * Handles a request in /HOSTNAME/URI format
 */
func handleRaw(w http.ResponseWriter, r *http.Request) {
	fmt.Printf(">> %s\n", r.RequestURI)
	target := fmt.Sprintf("http://%s", r.RequestURI[6:])
	serveFullURI(w, r, target)
}

/*
 * Handle request for given targetURI
 */

func serveFullURI(dst http.ResponseWriter, rq *http.Request, startURI string) {
	
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
		
		aes, _ := aestool.New(pngReader.KeySize, pngReader.BlobSize, []byte("wurstsalat"), pngReader.IV);
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
