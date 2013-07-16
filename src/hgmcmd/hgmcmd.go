package main

import(
	"fmt"
	"os"
	"libhgms/flickr/png"
)


func main() {
	subModule := ""
	if len(os.Args) > 1 {
		subModule = os.Args[1]
	}
	
	if subModule == "encrypt" && len(os.Args) == 6 {
		flickr.CryptAes(os.Args[2], os.Args[3], os.Args[4], os.Args[5], true)
	} else if subModule == "decrypt" && len(os.Args) == 6 {
		flickr.CryptAes(os.Args[2], os.Args[3], os.Args[4], os.Args[6], false)
	} else if subModule == "proxy" && len(os.Args) == 4 {
		LaunchProxy(os.Args[2], os.Args[3])
	} else {
		fmt.Printf("Usage: %s encrypt pass IV in out|decrypt pass IV in out|proxy bindaddr port\n", os.Args[0]);
	}
	
}