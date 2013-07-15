package flickr

import (
	"github.com/tadzik/simpleaes"
	"os"
)

func CryptAes(key string, infile string, outfile string, encrypt bool) {
	
	fhIn, err := os.Open(infile)
	if err != nil { panic(err) }
	defer fhIn.Close()
	
	fhOut, err := os.Create(outfile)
	if err != nil { panic(err) }
	defer fhOut.Close()
	
	aes, err := simpleaes.New(16, key); /* fixme: random IV! */
	if err != nil { panic(err) }
	
	if encrypt {
		err = aes.EncryptStream(fhIn, fhOut);
	} else {
		err = aes.DecryptStream(fhIn, fhOut);
	}
	
	if err != nil {
		panic(err)
	}
	
}
