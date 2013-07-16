package flickr

import (
	"os"
	"fmt"
	"libhgms/crypto/aestool"
)

func CryptAes(key string, infile string, outfile string, encrypt bool) {
	
	fhIn, err := os.Open(infile)
	if err != nil { panic(err) }
	defer fhIn.Close()
	
	fhOut, err := os.Create(outfile)
	if err != nil { panic(err) }
	defer fhOut.Close()
	fmt.Printf("IN=%s, OUT=%s\n", infile, outfile)
	aes, err := aestool.New(16, key, ""); /* fixme: random IV! */
	if err != nil { panic(err) }
	
	if encrypt {
		err = aes.EncryptStream(fhOut, fhIn);
	} else {
		err = aes.DecryptStream(fhOut, fhIn);
	}
	
	if err != nil {
		panic(err)
	}
	
}
