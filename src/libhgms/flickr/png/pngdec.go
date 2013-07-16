package flickr

import (
	"fmt"
	"io"
	"bytes"
	"compress/zlib"
)

type reader struct {
	r io.Reader          /* raw-file reader          */
	zr io.Reader         /* zlib reader              */
	uncompressed []byte  /* raw data with scanlines  */
	decoded []byte       /* decoded -> scanline-free */
	slsize int           /* scanline length          */
	IV []byte            /* IV used by this image    */
	KeySize int          /* AES keysize in BYTES     */
}


func NewReader(r io.Reader) (*reader, error) {
	pr := new(reader)
	pr.r = r
	return pr, nil
}

func xunpack(b []byte) (int) {
	return ((int(b[0]) << 24) | (int(b[1]) << 16) | (int(b[2]) << 8) | int(b[3]))
}

func (pr *reader) InitReader() {
	chunk := make([]byte, 8)
	parseHeader := true
	
	pr.r.Read(chunk) /* fixme: check png magic */
	
	for parseHeader {
		pr.r.Read(chunk) /* size of next chunk */
		qlen := xunpack(chunk)
		
		if(string(chunk[4:]) != "IDAT" && qlen < 4096) {
			payload := make([]byte, qlen+4) /* 4 = length of CRC */
			pr.r.Read(payload)
			/* We read the CRC (to get to the correct position) but we are not using it
			 * -> Therefore it's ok to throw it away now. FIXME: We should probably check the CRC at some later point */
			payload = payload[:qlen]
			
			if(string(chunk[4:]) == "IHDR") {
				bytesPerPixel := 3 /* fixme */
				scanlineVal := xunpack(payload)
				pr.slsize = scanlineVal * bytesPerPixel
				fmt.Printf("> scanline configured to %d\n", pr.slsize)
			} else if string(chunk[4:]) == "tEXt" {
				pairs := bytes.SplitN(payload, []byte("="), 2)
				fmt.Printf(">> [%s]=[%s]\n", pairs[0], pairs[1]);
				if string(pairs[0]) == "IV" { pr.IV = pairs[1] }
			}
		} else {
			parseHeader = false
		}
		
	}
	
	zr, err := zlib.NewReader(pr.r)
	if err != nil { panic(err) } /* fixme */
	pr.zr = zr
	
	if pr.slsize == 0 { panic(nil) } /* fixme */
	pr.KeySize = 16 /* fixme: should parse ENCRYPTION=aes128 field */
}

func (pr *reader) Read(p []byte) (n int, err error) {
	return pr.realRead(p)
}


func (pr *reader) realRead(p []byte) (n int, err error) {
	ucChunk := make([]byte, 1024)
	running := true
	for running {
		if len(pr.decoded) < len(p) {
			/* Read a compressed chunk */
			zbread, err := pr.zr.Read(ucChunk[0:])
			if err != nil { running = false }
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
