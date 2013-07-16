package aestool

import(
	"crypto/aes"
	"crypto/cipher"
	"io"
)

type AesTool struct {
	encrypter cipher.BlockMode
	decrypter cipher.BlockMode
}

/*
 * Returns a new aestool object instance
 */
func New(keySize int, key []byte, iv []byte) (*AesTool, error) {
	aesTool := AesTool{}
	
	paddedKey := make([]byte, keySize)
	paddedIV := make([]byte, keySize)
	copy(paddedKey, []byte(key))
	copy(paddedIV, []byte(iv))
	
	
	aesCipher, err := aes.NewCipher(paddedKey)
	if err != nil {
		return nil, err
	}
	
	aesTool.encrypter = cipher.NewCBCEncrypter(aesCipher, paddedIV)
	aesTool.decrypter = cipher.NewCBCDecrypter(aesCipher, paddedIV)
	
	return &aesTool, nil
}

func (self *AesTool) cryptWorker(writer io.Writer, reader io.Reader, cb cipher.BlockMode) error {
	blockSize := self.encrypter.BlockSize()
	blockBuff := make([]byte, blockSize)
	var outBuff []byte
	
	for {
		outBuff = []byte("")
		
		for len(outBuff) < 8192 {
			_, err := io.ReadFull(reader, blockBuff)
			
			if err != nil && err != io.ErrUnexpectedEOF {
				if err == io.EOF { err = nil } /* not really an error: expected result */
				return err
			}
			cb.CryptBlocks(blockBuff, blockBuff)
			outBuff = append(outBuff, blockBuff...)
		}
		_, err := writer.Write(outBuff)
		if err != nil { return err }
	}
	
	return nil /* not reached */
}

func (self *AesTool) DecryptStream(writer io.Writer, reader io.Reader) error {
	return self.cryptWorker(writer, reader, self.decrypter)
}

func (self *AesTool) EncryptStream(writer io.Writer, reader io.Reader) error {
	return self.cryptWorker(writer, reader, self.encrypter)
}

