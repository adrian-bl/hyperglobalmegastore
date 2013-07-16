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
	
	if subModule == "encrypt" && len(os.Args) == 5 {
		flickr.CryptAes(os.Args[2], os.Args[3], os.Args[4], true)
	} else if subModule == "decrypt" && len(os.Args) == 5 {
		flickr.CryptAes(os.Args[2], os.Args[3], os.Args[4], false)
	} else if subModule == "proxy" && len(os.Args) == 4 {
		LaunchProxy(os.Args[2], os.Args[3])
	} else {
		fmt.Printf("Usage: %s encrypt|decrypt|proxy\n", os.Args[0]);
	}
	
}
