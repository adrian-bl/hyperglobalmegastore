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
