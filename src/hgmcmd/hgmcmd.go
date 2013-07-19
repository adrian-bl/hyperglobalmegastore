package main

import(
	"fmt"
	"os"
	"encoding/hex"
	"libhgms/flickr/png"
)


func main() {
	subModule := ""
	iv := make([]byte, 16)
	kk := make([]byte, 16)
	
	if len(os.Args) > 1 {
		subModule = os.Args[1]
	}
	
	if subModule == "encrypt" && len(os.Args) == 6 {
		hex.Decode(iv, []byte(os.Args[3]))
		hex.Decode(kk, []byte(os.Args[2]))
		flickr.CryptAes(kk, iv, os.Args[4], os.Args[5], true)
	} else if subModule == "decrypt" && len(os.Args) == 6 {
		hex.Decode(iv, []byte(os.Args[3]))
		hex.Decode(kk, []byte(os.Args[2]))
		flickr.CryptAes(kk, iv, os.Args[4], os.Args[5], false)
	} else if subModule == "proxy" && len(os.Args) == 4 {
		LaunchProxy(os.Args[2], os.Args[3])
	} else {
		fmt.Printf("Usage: %s encrypt pass IV in out|decrypt pass IV in out|proxy bindaddr port\n", os.Args[0]);
	}
	
}
