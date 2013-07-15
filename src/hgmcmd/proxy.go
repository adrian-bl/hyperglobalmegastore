package main

import (
   "net/http"
   "fmt"
   "io"
   "time"
   "flickr/png"
   "github.com/tadzik/simpleaes"
)

/* Log structure expected by emitLog() */
type LogEntry struct {
	StatusCode int
	ContentLength int64
	TimeRqStart int64
	Method string
	RemoteAddr string
	Host string
	RequestURI string
	TargetURI string
}

/* Custom HTTP Client, setup done in main() */
var backendClient *http.Client



func LaunchProxy(bindAddr string, bindPort string) {
	tr := &http.Transport{ ResponseHeaderTimeout: 5*time.Second }
	backendClient = &http.Client{Transport: tr}
	http.HandleFunc("/", serve)
	http.ListenAndServe(":8080", nil) 
}



func emitLog(l *LogEntry) {
	fmt.Printf("%d %d %s %d %d %s http://%s%s -> http://target%s\n", time.Now().Unix(),
	             (time.Now().UnixNano()-l.TimeRqStart)/10e6,
	              l.RemoteAddr, l.ContentLength, l.StatusCode, l.Method, l.Host, l.RequestURI, l.TargetURI);
}


func serve(clientW http.ResponseWriter, clientRQ *http.Request) {
	/* Holy macaroni! A new http request!!!111
	 * -> Setup some basic logging stuff */
	logItem := LogEntry{ TimeRqStart: time.Now().UnixNano(),
	                     Method: clientRQ.Method,
	                     Host: clientRQ.Host,
	                     RequestURI: clientRQ.RequestURI,
	                     RemoteAddr: clientRQ.RemoteAddr }
	defer emitLog(&logItem)
	
	targetURI := clientRQ.RequestURI
	
	/* Assemble a new GET request to our guessed target URL and copy almost all HTTP headers */
	backendRQ, err := http.NewRequest("GET", fmt.Sprintf("http://farm4.staticflickr.com/3809/9293034678_ee5dd4a670_o.png"), nil)
	if err != nil {
		logItem.StatusCode = http.StatusInternalServerError
		clientW.WriteHeader(logItem.StatusCode)
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
		logItem.StatusCode = http.StatusServiceUnavailable
		clientW.WriteHeader(logItem.StatusCode)
		io.WriteString(clientW,"Backend server down :-(\n")
		return
	}
	/* We got a backend connection, so we should close it at some later time */
	defer backendResp.Body.Close();
	
	/* Update Log entry with backend reply */
	logItem.StatusCode = backendResp.StatusCode
	logItem.ContentLength = backendResp.ContentLength
	logItem.TargetURI = targetURI
	
	/* All done! -> Send headers and stream body to client */
	clientW.WriteHeader(backendResp.StatusCode)
	
	pngReader, err := flickr.NewReader(backendResp.Body)
	if err != nil { panic(err) }
	
	aes, _ := simpleaes.New(16, "wurstsalat");
	fmt.Printf("%s\n", aes)
	aes.DecryptStream(pngReader, clientW)
	io.Copy(clientW, pngReader)
	
}
