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
	http.HandleFunc("/raw/", handleRaw)
	http.ListenAndServe(":8080", nil) 
}


/*
 * Handles a request in /HOSTNAME/URI format
 */
func handleRaw(w http.ResponseWriter, r *http.Request) {
	fmt.Printf(">> %s\n", r.RequestURI)
	target := fmt.Sprintf("http://%s", r.RequestURI[5:])
	serveFullURI(w, r, target)
}

/*
 * Handle request for given targetURI
 */
func serveFullURI(clientW http.ResponseWriter, clientRQ *http.Request, targetURI string) {
	
	fmt.Printf("GET %s\n", targetURI)
	
	/* Assemble a new GET request to our guessed target URL and copy almost all HTTP headers */
	backendRQ, err := http.NewRequest("GET", targetURI, nil)
	if err != nil {
		clientW.WriteHeader(http.StatusInternalServerError)
		io.WriteString(clientW, "Internal server errror :-(\n")
		return
	}
	
	for key, value := range clientRQ.Header {
		if key != "Connection" {
			backendRQ.Header.Set(key, value[0]);
		}
	}
	
	/* Our request is ready: Send it to the backend server */
	backendResp, err := backendClient.Do(backendRQ)
	if err != nil {
		clientW.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(clientW,"Backend server down :-(\n")
		return
	}
	/* We got a backend connection, so we should close it at some later time */
	defer backendResp.Body.Close();
	
	/* All done! -> Send headers and stream body to client */
	clientW.WriteHeader(backendResp.StatusCode)
	
	pngReader, err := flickr.NewReader(backendResp.Body)
	if err != nil { panic(err) }
	pngReader.InitReader()
	
	aes, _ := aestool.New(pngReader.KeySize, []byte("wurstsalat"), pngReader.IV); /* fixme: this should take bytes */
	aes.DecryptStream(clientW, pngReader)
}
