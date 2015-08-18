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
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"strconv"
)

type reader struct {
	r            io.Reader /* raw-file reader          */
	zr           io.Reader /* zlib reader              */
	uncompressed []byte    /* raw data with scanlines  */
	decoded      []byte    /* decoded -> scanline-free */
	slsize       int       /* scanline length          */
	IV           []byte    /* IV used by this image    */
	ContentSize  int64     /* Content-Size sent in HTTP header */
	BlobSize     int64     /* Size of this blob        */
}

// Returns a new PNG Reader.
// Note: You must call InitReader before reading any data
func NewReader(r io.Reader) (*reader, error) {
	pr := new(reader)
	pr.r = r
	return pr, nil
}

// Initializes the PNG reader: Read the initial PNG magic and seek to the IDAT marker.
// The function will initialize IV, ContentSize and BlobSize
func (pr *reader) InitReader() error {
	chunk := make([]byte, 8)
	parseHeader := true

	/* Verify PNG-Header magic */
	pr.r.Read(chunk)
	if string(chunk) != "\x89PNG\x0D\x0A\x1A\x0A" {
		return errors.New("Invalid PNG header magic")
	}

	for parseHeader {
		pr.r.Read(chunk) /* size of next chunk */
		qlen := xunpack(chunk)

		if string(chunk[4:]) != "IDAT" && qlen < 4096 {
			payload := make([]byte, qlen+4) /* 4 = length of CRC */
			pr.r.Read(payload)

			/* We read the CRC (to get to the correct position) but we are not using it
			 * -> Therefore it's ok to throw it away now. FIXME: We should probably check the CRC at some later point */
			payload = payload[:qlen]

			if string(chunk[4:]) == "IHDR" {
				scanlineVal := xunpack(payload[0:4])
				bytesPerPixel := 0
				if payload[9] == 0x2 {
					bytesPerPixel = 3
				} else if payload[9] == 0x6 {
					bytesPerPixel = 4
				}
				pr.slsize = scanlineVal * bytesPerPixel
			} else if string(chunk[4:]) == "tEXt" {
				pairs := bytes.SplitN(payload, []byte("="), 2)

				if string(pairs[0]) == "IV" {
					pr.IV = pairs[1]
				}
				if string(pairs[0]) == "CONTENTSIZE" {
					pr.ContentSize, _ = strconv.ParseInt(string(pairs[1]), 10, 64)
				}
				if string(pairs[0]) == "BLOBSIZE" {
					pr.BlobSize, _ = strconv.ParseInt(string(pairs[1]), 10, 64)
				}
			}
		} else {
			parseHeader = false
		}

	}

	/* scanline size must be > 0 */
	if pr.slsize < 1 {
		return errors.New("Invalid scanline size, corrupted or unsupported PNG header")
	}

	/* Add zlib reader to our struct */
	zr, err := zlib.NewReader(pr.r)
	if err != nil {
		return err
	}
	pr.zr = zr

	return nil
}

// Our public Read function. Returns the bytes read, err on error.
func (pr *reader) Read(p []byte) (n int, err error) {
	ucChunk := make([]byte, 1024*16)
	running := true
	for running {
		if len(pr.decoded) < len(p) {
			/* Read a compressed chunk */
			zbread, err := pr.zr.Read(ucChunk[0:])
			if err != nil {
				running = false
			}
			pr.uncompressed = append(pr.uncompressed, ucChunk[0:zbread]...)
			for len(pr.uncompressed) > pr.slsize {
				pr.decoded = append(pr.decoded, pr.uncompressed[1:pr.slsize+1]...)
				pr.uncompressed = pr.uncompressed[pr.slsize+1:]
			}
		} else {
			running = false
		}
	}

	if len(pr.decoded) > 0 {
		canCopy := len(pr.decoded)
		if canCopy > len(p) {
			canCopy = len(p)
		}
		copy(p, pr.decoded[0:canCopy])
		pr.decoded = pr.decoded[canCopy:]
		return canCopy, nil
	}

	return 0, errors.New("Nothing to decode")
}

// Unpacks a 32bit integer
func xunpack(b []byte) int {
	return ((int(b[0]) << 24) | (int(b[1]) << 16) | (int(b[2]) << 8) | int(b[3]))
}
