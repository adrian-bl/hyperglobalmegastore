package aestool

import(
	"crypto/aes"
	"crypto/cipher"
	"io"
)

type AesTool struct {
	encrypter cipher.BlockMode
	decrypter cipher.BlockMode
	streamlen int64
}

/*
 * Returns a new aestool object instance
 */
func New(keySize int, streamlen int64, key []byte, iv []byte) (*AesTool, error) {
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
	aesTool.streamlen = streamlen
	
	return &aesTool, nil
}

/* fuck it */
func (self *AesTool) cryptWorker(dst io.Writer, src io.Reader, cb cipher.BlockMode) (err error) {
	blockSize := cb.BlockSize();
	blockBuf := make([]byte, blockSize);
	
	for self.streamlen != 0 {
		_, er := io.ReadFull(src, blockBuf)
		
		if er == io.EOF {
			break /* not really an error for us */
		}
		if er != nil && er != io.ErrUnexpectedEOF {
			err = er
			break
		}
		/* still here? -> we got new data to write */
		cb.CryptBlocks(blockBuf, blockBuf)
		
		if self.streamlen > -1 && int64(len(blockBuf)) > self.streamlen {
			blockBuf = blockBuf[:self.streamlen]
		}
		nw, ew := dst.Write(blockBuf)
		self.streamlen -= int64(nw)
		
		if nw != len(blockBuf) {
			err = io.ErrShortWrite
			break
		}
		if ew != nil {
			err = ew
			break
		}
	}
	
	return err
}

func (self *AesTool) DecryptStream(writer io.Writer, reader io.Reader) error {
	return self.cryptWorker(writer, reader, self.decrypter)
}

func (self *AesTool) EncryptStream(writer io.Writer, reader io.Reader) error {
	return self.cryptWorker(writer, reader, self.encrypter)
}

