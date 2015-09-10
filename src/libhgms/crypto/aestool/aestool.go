/*
 * Copyright (C) 2013-2015 Adrian Ulrich <adrian@blinkenlights.ch>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program. If not, see <http://www.gnu.org/licenses/>.
 */

package aestool

import (
	"github.com/spacemonkeygo/openssl"
	"io"
)

type AesTool struct {
	encrypter openssl.EncryptionCipherCtx
	decrypter openssl.DecryptionCipherCtx
	blocksize int
	streamlen int64
	skipbytes *int64
}

// Returns a new aestool instance. The streamlen parameter specifies
// how many bites we are going to decrypt (the real filesize is unknown
// to the decryptor due to padding)
func New(streamlen int64, key []byte, iv []byte) (*AesTool, error) {
	aesTool := AesTool{}

	aesCipher, err := openssl.GetCipherByName("aes-256-cbc")
	if err != nil {
		return nil, err
	}

	eCtx, err := openssl.NewEncryptionCipherCtx(aesCipher, nil, key, iv)
	if err != nil {
		return nil, err
	}

	dCtx, err := openssl.NewDecryptionCipherCtx(aesCipher, nil, key, iv)
	if err != nil {
		return nil, err
	}

	sbNil := int64(0)
	aesTool.encrypter = eCtx
	aesTool.decrypter = dCtx
	aesTool.blocksize = aesCipher.BlockSize()
	aesTool.streamlen = streamlen
	aesTool.SetSkipBytes(&sbNil)

	return &aesTool, nil
}

// Returns the block size of the configured cipher
func (self *AesTool) GetBlockSize() int {
	return self.blocksize
}

func (self *AesTool) SetSkipBytes(sb *int64) {
	self.skipbytes = sb
}

// Handles all decryption and encryption work
func (self *AesTool) cryptWorker(dst io.Writer, src io.Reader, decrypt bool) (err error) {
	blockSize := 1024 * 512
	blockBuf := make([]byte, blockSize)

	var ctxt []byte
	var cerr error

	for self.streamlen != 0 {
		wFrom := int64(0)
		wTo := int64(0)

		br, er := src.Read(blockBuf[0:]) // expected to always return a multiple of cb.Blocksize

		if er == io.EOF && br == 0 {
			break /* not really an error for us */
		}
		if er != nil {
			panic(er)
		}

		// De- or Encrypt the data
		// We expect to get padded data, so no need to call Finish
		if decrypt == true {
			ctxt, cerr = self.decrypter.DecryptUpdate(blockBuf[0:br])
		} else {
			ctxt, cerr = self.encrypter.EncryptUpdate(blockBuf[0:br])
		}
		if cerr != nil {
			panic(cerr)
		}
		copy(blockBuf, ctxt)
		wTo = int64(len(ctxt)) // may be different (IV)

		if self.streamlen > -1 && wTo > self.streamlen {
			wTo = self.streamlen
		}

		if *self.skipbytes != 0 {
			/*			fmt.Printf("> should skip a total of %d bytes\n", *self.skipbytes) */
			canSkipNow := wTo - wFrom
			if canSkipNow > *self.skipbytes {
				canSkipNow = *self.skipbytes
			}
			wFrom = canSkipNow
			/*			fmt.Printf("--> will skip %d bytes: [%d:%d]\n", canSkipNow, wFrom, wTo) */
			*self.skipbytes -= canSkipNow
			self.streamlen -= canSkipNow

			if wFrom == wTo {
				// fmt.Printf("Skipping zero-sized write\n")
				continue
			}
		}

		nw, ew := dst.Write(blockBuf[wFrom:wTo])
		self.streamlen -= int64(nw)

		if int64(nw) != (wTo - wFrom) {
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

// Decrypts given input stream. This is a shortcut to cryptWorker
func (self *AesTool) DecryptStream(writer io.Writer, reader io.Reader) error {
	return self.cryptWorker(writer, reader, true)
}

// Encrypts given input stream. This is a shortcut to cryptWorker
func (self *AesTool) EncryptStream(writer io.Writer, reader io.Reader) error {
	return self.cryptWorker(writer, reader, false)
}
