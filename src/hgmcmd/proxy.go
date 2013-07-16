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
	http.HandleFunc("/", serve)
	http.ListenAndServe(":8080", nil) 
}




func serve(clientW http.ResponseWriter, clientRQ *http.Request) {
	/* Holy macaroni! A new http request!!!111
	 * -> Setup some basic logging stuff */
	
//	targetURI := clientRQ.RequestURI
	
	/* Assemble a new GET request to our guessed target URL and copy almost all HTTP headers */
	backendRQ, err := http.NewRequest("GET", fmt.Sprintf("http://farm4.staticflickr.com/3758/9297644987_259646b8ff_o.png"), nil)
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
