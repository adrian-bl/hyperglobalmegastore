/*
 * Copyright (C) 2013 Adrian Ulrich <adrian@blinkenlights.ch>
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
	"crypto/aes"
	"crypto/cipher"
	"io"
	"fmt"
)

type AesTool struct {
	encrypter cipher.BlockMode
	decrypter cipher.BlockMode
	streamlen int64
	skipbytes *int64
}

// Returns a new aestool instance. The streamlen parameter specifies
// how many bites we are going to decrypt (the real filesize is unknown
// to the decryptor due to padding)
func New(streamlen int64, key []byte, iv []byte) (*AesTool, error) {
	aesTool := AesTool{}

	aesCipher, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	sbNil := int64(0)
	aesTool.encrypter = cipher.NewCBCEncrypter(aesCipher, iv)
	aesTool.decrypter = cipher.NewCBCDecrypter(aesCipher, iv)
	aesTool.streamlen = streamlen
	aesTool.SetSkipBytes(&sbNil)

	return &aesTool, nil
}

func (self *AesTool) SetSkipBytes(sb *int64) {
	fmt.Printf("SKIP: %d\n", *sb)
	self.skipbytes = sb
}

// Handles all decryption and encryption work
func (self *AesTool) cryptWorker(dst io.Writer, src io.Reader, cb cipher.BlockMode) (err error) {
	blockSize := cb.BlockSize()
	blockBuf := make([]byte, blockSize)

	for self.streamlen != 0 {
		wFrom := int64(0)
		wTo := int64(blockSize)
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
			/* blockBuf = blockBuf[:self.streamlen] */
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
				fmt.Printf("Skipping zero-sized write\n")
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
	return self.cryptWorker(writer, reader, self.decrypter)
}

// Encrypts given input stream. This is a shortcut to cryptWorker
func (self *AesTool) EncryptStream(writer io.Writer, reader io.Reader) error {
	return self.cryptWorker(writer, reader, self.encrypter)
}
