package flickr

import (
	"os"
	"libhgms/crypto/aestool"
)

func CryptAes(key []byte, iv []byte, infile string, outfile string, encrypt bool) {
	
	fhIn, err := os.Open(infile)
	if err != nil { panic(err) }
	defer fhIn.Close()
	
	fhOut, err := os.Create(outfile)
	if err != nil { panic(err) }
	defer fhOut.Close()
	
	aes, err := aestool.New(-1, key, iv);
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
