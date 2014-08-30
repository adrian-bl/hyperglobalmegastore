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

package main

import (
	"encoding/hex"
	"fmt"
	"hgmfs"
	"hgmweb"
	"libhgms/flickr/png"
	"os"
)

func main() {
	subModule := ""

	if len(os.Args) > 1 {
		subModule = os.Args[1]
	}

	if subModule == "encrypt" && len(os.Args) == 6 {
		flickr.CryptAes(strToSlice(os.Args[2]), strToSlice(os.Args[3]), os.Args[4], os.Args[5], true)
	} else if subModule == "decrypt" && len(os.Args) == 6 {
		flickr.CryptAes(strToSlice(os.Args[2]), strToSlice(os.Args[3]), os.Args[4], os.Args[5], false)
	} else if subModule == "proxy" && len(os.Args) >= 4 {
		webrootPrefix := ""
		if len(os.Args) > 4 {
			webrootPrefix = os.Args[4]
		}
		hgmweb.LaunchProxy(os.Args[2], os.Args[3], webrootPrefix)
	} else if subModule == "mount" && len(os.Args) == 3 {
		hgmfs.MountFilesystem(os.Args[2])
	} else {
		fmt.Printf("Usage: %s encrypt pass IV in out|decrypt pass IV in out|proxy bindaddr port [prefix/]\n", os.Args[0])
	}

}

func strToSlice(input string) []byte {
	rv := make([]byte, len(input)/2)
	hex.Decode(rv, []byte(input))
	return rv
}
