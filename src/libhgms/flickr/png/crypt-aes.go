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

package flickr

import (
	"io"
	"libhgms/crypto/aestool"
	"os"
)

type paddingReader struct {
	r        io.Reader
	padbytes int
}

// Encrypts or decrypts `infile` into `outfile` using given key and IV.
// The `key` value is expected to be padded to the correct size (for aes 128, 196 or 256)
func CryptAes(key []byte, iv []byte, infile string, outfile string, encrypt bool) {

	fhIn, err := os.Open(infile)
	if err != nil {
		panic(err)
	}
	defer fhIn.Close()

	fhOut, err := os.Create(outfile)
	if err != nil {
		panic(err)
	}
	defer fhOut.Close()

	aes, err := aestool.New(-1, key, iv)
	if err != nil {
		panic(err)
	}

	pr := newPaddingReader(fhIn, aes.GetBlockSize())

	if encrypt {
		err = aes.EncryptStream(fhOut, pr)
	} else {
		err = aes.DecryptStream(fhOut, pr)
	}

	if err != nil {
		panic(err)
	}

}

// Wrapper around io.Reader - ensures that the *last* read
// is padded to `padbytes' bytes
func newPaddingReader(r io.Reader, padbytes int) *paddingReader {
	pr := new(paddingReader)
	pr.r = r
	pr.padbytes = padbytes
	return pr
}

// Padding read call
// Expects b to be a multiple of padbytes
func (pr *paddingReader) Read(b []byte) (int, error) {
	rb, re := io.ReadAtLeast(pr.r, b, len(b))

	if rb > 0 && (re == io.EOF || re == io.ErrUnexpectedEOF) {
		claim := pr.padbytes * (int(rb/pr.padbytes) + 1)
		rb = claim
		re = nil
	}

	return rb, re
}
