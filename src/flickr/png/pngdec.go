package flickr

import (
	"fmt"
	"io"
	"compress/zlib"
)

type reader struct {
	r io.Reader          /* raw-file reader          */
	zr io.Reader         /* zlib reader              */
	slsize int           /* scanline length          */
	uncompressed []byte  /* raw data with scanlines  */
	decoded []byte       /* decoded -> scanline-free */
}

/* Our data stream is:
 * #1: zlib compressed
 * #2: png-scanline encoded
 * #3: encrypted
*/
func NewReader(r io.Reader) (io.ReadCloser, error) {
	pr := new(reader)
	pr.r = r
	return pr, nil
}

func xunpack(b []byte) (int) {
	return ((int(b[0]) << 24) | (int(b[1]) << 16) | (int(b[2]) << 8) | int(b[3]))
}

func (pr *reader) Read(p []byte) (n int, err error) {
	if(pr.slsize == 0) {
		chunk := make([]byte, 8)
		parseHeader := true
		
		pr.r.Read(chunk) /* fixme: check png magic */
		
		for parseHeader {
			pr.r.Read(chunk) /* size of next chunk */
			fmt.Printf("Q=%X\n", chunk)
			qlen := xunpack(chunk)
			fmt.Printf("> QL =%d -> %s\n", qlen, chunk[4:])
			
			if(string(chunk[4:]) != "IDAT" && qlen < 4096) {
				ihdr := make([]byte, 4+qlen)
				pr.r.Read(ihdr)
				
				if(string(chunk[4:]) == "IHDR") {
					bytesPerPixel := 3 /* fixme */
					scanlineVal := xunpack(ihdr)
					pr.slsize = scanlineVal * bytesPerPixel
					fmt.Printf("> scanline configured to %d\n", pr.slsize)
				}
			} else {
				parseHeader = false
			}
			
		}
		
		zr, err := zlib.NewReader(pr.r)
		if err != nil { panic(err) } /* fixme */
		if pr.slsize == 0 { panic(nil) } /* fixme */
		pr.zr = zr
	}
	return pr.realRead(p)
}

func (pr *reader) SetKey(fo string) {
}

func (pr *reader) realRead(p []byte) (n int, err error) {
	ucChunk := make([]byte, 1024)
	running := true

	for running {
		if len(pr.decoded) < len(p) {
			/* Read a compressed chunk */
			zbread, err := pr.zr.Read(ucChunk[0:])
			if err != nil { running = false }
			fmt.Printf("D")
			pr.uncompressed = append(pr.uncompressed, ucChunk[0:zbread]...)
			for len(pr.uncompressed) > pr.slsize {
				pr.decoded = append(pr.decoded, pr.uncompressed[1:pr.slsize+1]...)
				pr.uncompressed = pr.uncompressed[pr.slsize+1:]
				fmt.Printf("S")
			}
		} else {
			running = false
		}
	}

	if len(pr.decoded) > 0 {
		canCopy := len(pr.decoded)
		if(canCopy > len(p)) { canCopy = len(p) }
		copy(p, pr.decoded[0:canCopy])
		pr.decoded = pr.decoded[canCopy:]
		return canCopy, nil
	}
	
	/* FIXME: EOF? */
	panic(nil)
	return 0, nil
}

/* fixme: ilmplement me */
func (pr *reader) Close() error {
	return nil
}
