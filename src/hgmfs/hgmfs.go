package hgmfs

import (
	"os"
	"io/ioutil"
	"encoding/json"
	"fmt"
	"log"
	"time"
	"net/http"
	"syscall"
	"sync"
	"math/rand"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
)

/* fixme: duplicate code */
type JsonMeta struct {
	Location [][]string
	Key string
	Created int64
	ContentSize uint64
}

type HgmFs struct {
	pathfs.FileSystem
	rqx int64
}

type hgmFile struct {
	sync.Mutex
	fuseFilename string
	jsonMetafile string
	jsonMetadata JsonMeta
	offset int64
	resp *http.Response
	readQueue map[int32]int64
}



func NewHgmFile(fname string) nodefs.File {
	jmetafile := getLocalPath(fname)
	jmetadata, err := getJsonMeta(jmetafile)
	if err != nil {
		return nil
	}
	
	hgmf := hgmFile{fuseFilename: fname, jsonMetafile: jmetafile, jsonMetadata: jmetadata}
	hgmf.readQueue = make(map[int32]int64)
	return &hgmf
}

func (f *hgmFile) Truncate(size uint64) fuse.Status {
	return fuse.EPERM
}
func (f *hgmFile) Allocate(off uint64, sz uint64, mode uint32) fuse.Status {
	return fuse.EPERM
}
func (f *hgmFile) Chmod(mode uint32) fuse.Status {
	return fuse.EPERM
}
func (f *hgmFile) Chown(guid uint32, gid uint32) fuse.Status {
	return fuse.EPERM
}
func (f *hgmFile) Flush() fuse.Status {
	return fuse.OK
}
func (f *hgmFile) Fsync(flags int) fuse.Status {
	return fuse.OK
}
func (f *hgmFile) InnerFile() nodefs.File {
	return nil
}
func (f *hgmFile) Release() {
	if f.resp != nil {
		f.resp.Body.Close()
	}
}
func (f *hgmFile) SetInode(n *nodefs.Inode) {
}
func (f *hgmFile) Write(content []byte, off int64) (uint32, fuse.Status) {
	return 0, fuse.EPERM
}

func (f *hgmFile) String() string {
	return fmt.Sprintf("FooBar!")
}

func (f *hgmFile) Utimens(a *time.Time, m *time.Time) fuse.Status {
	return fuse.EPERM
}


func (f *hgmFile) nextInQueue(want int64) (bool) {

	if want < f.offset {
		return false
	}

	for _, v := range f.readQueue {
		if v >= f.offset && v < want {
			return false
		}
	}

	return true
}

func (f *hgmFile) Read(dst []byte, off int64) (fuse.ReadResult, fuse.Status) {
	/* Generate a random id, aids printf() debugging :-) */
	rqid := rand.Int31()
	
	f.Lock()
	f.readQueue[rqid] = off
	f.Unlock()
	
	retry := 0
	for loop := true ; loop ; {
		f.Lock()
//		fmt.Printf("<%08X> want offset %d, connection is at %d, waiting\n", rqid, off, f.offset)
		
		if (f.offset == off) || (len(f.readQueue) == 1 && retry > 5) || retry > 2 && f.nextInQueue(off) || (retry > 10) {
//			fmt.Printf("<%08X> BREAKOUT: rqlen=%d, retry=%d, got=%d, want=%d, niQ=%s\n", rqid, len(f.readQueue), retry, f.offset, off, f.nextInQueue(off))
			loop = false
			defer f.Unlock()
		} else {
			f.Unlock()
			retry++
			time.Sleep(0.02*1e9)
		}
	}
	
//	fmt.Printf("<%08X> starting actual read request\n", rqid)
	
	
	if off != f.offset && f.resp != nil {
		mustSeek := off - f.offset
		
		if mustSeek > 0 && mustSeek < 1024*1024*3 {
			fmt.Printf("<%08X> Could do a quick seek by dropping %d bytes\n", rqid, mustSeek)
			for ; mustSeek != 0 ; {
				fmt.Printf("<%08X> QuickFWD: %d\n", rqid, mustSeek)
				tmpBuf := make([]byte, mustSeek)
				didRead, err := f.resp.Body.Read(tmpBuf)
				if err != nil {
					return nil, fuse.EPERM
				}
				mustSeek -= int64(didRead)
				f.offset += int64(didRead)
			}
		} else {
			fmt.Printf("<%08X> !!!!!!!!!!! Closing existing http request: want=%d, got=%d\n", rqid, off, f.offset)
			f.resp.Body.Close()
			f.resp = nil
			f.offset = 0
		}
	}
	
	if f.resp == nil {
		fmt.Printf("<%08X> Establishing a new connection, need to seek to %d\n", rqid, off)

		req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:8080/%s", f.fuseFilename), nil)
		if err != nil {
			return nil, fuse.EPERM
		}

		req.Header.Add("Range", fmt.Sprintf("bytes=%d-", off))
		hclient := &http.Client{}
		resp, err := hclient.Do(req)
		if err != nil {
			return nil, fuse.EPERM
		}

		if resp.StatusCode != 200 && resp.StatusCode != 206 {
			fmt.Printf("<%08X> FATAL: Wrong status code: %d\n", resp.StatusCode)
			resp.Body.Close()
			return nil, fuse.EPERM
		}
		f.offset = off
		f.resp = resp
	}
	
	bytesRead := 0
	mustRead := len(dst)
	for bytesRead != mustRead {
		canRead := mustRead - bytesRead
		tmpBuf := make([]byte, canRead)
		didRead, err := f.resp.Body.Read(tmpBuf)
		if err != nil && didRead == 0 {
			break
		}
		
		for i:=0;i<didRead;i++ {
			dst[bytesRead+i] = tmpBuf[i]
		}
		
		bytesRead += didRead
	}
	
	f.offset += int64(bytesRead)
	rr := fuse.ReadResultData(dst[0:bytesRead])
	
	delete(f.readQueue, rqid)
	
	return rr, fuse.OK
}

func (f *hgmFile) GetAttr(out *fuse.Attr) fuse.Status {
	st := syscall.Stat_t{}
	err := syscall.Stat(f.jsonMetafile, &st)
	if err != nil {
		return fuse.EPERM
	}
	out.FromStat(&st)
	out.Size = f.jsonMetadata.ContentSize
	return fuse.OK
}

func getJsonMeta(localPath string) (JsonMeta, error){
	var jsm JsonMeta
	
	content, err := ioutil.ReadFile(localPath)
	if err != nil {
		return jsm, err
	}
	
	err = json.Unmarshal([]byte(content), &jsm)
	return jsm, err
}



func (self *HgmFs) GetAttr(fname string, ctx *fuse.Context) (*fuse.Attr, fuse.Status) {
	jsonPath := getLocalPath(fname)
	xdb("GetAttr->"+jsonPath)
	
	fi, err := os.Stat(jsonPath)
	if err != nil {
		return nil, fuse.ENOENT
	}
	if fi.IsDir() {
		return &fuse.Attr{Mode: fuse.S_IFDIR|0755}, fuse.OK
	} else {
		jsonMeta, _ := getJsonMeta(jsonPath) /* fixme: error handling */
		return &fuse.Attr{Mode: fuse.S_IFREG | 0644, Size: jsonMeta.ContentSize}, fuse.OK
	}
}


func (self *HgmFs) OpenDir(fname string, ctx *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	path := getLocalPath(fname)
	xdb("OpenDir->"+path)
	
	dirList, err := ioutil.ReadDir(path)

	if err == nil {
		fuseDirs := make([]fuse.DirEntry, 0, len(dirList))
		for _, fi := range dirList {
			fuseDirs = append(fuseDirs, fuse.DirEntry{Name: fi.Name(), Mode: fuse.S_IFDIR|0755}) /* fixme mode */
		}
		return fuseDirs, fuse.OK
	}
	return nil, fuse.ENOENT
}

func (self *HgmFs) Open(fname string, flags uint32, ctx *fuse.Context) (fuseFile nodefs.File, status fuse.Status) {
	xdb("Open->"+fname)
	return NewHgmFile(fname), fuse.OK
}

func getLocalPath(fusepath string) (string) {
	return fmt.Sprintf("./_aliases/%s", fusepath)
}

func MountFilesystem(dst string) {
	xdb("Mounting filesystem at "+dst)
	nfs := pathfs.NewPathNodeFs(&HgmFs{FileSystem: pathfs.NewDefaultFileSystem()}, nil)
	server, _, err := nodefs.MountFileSystem(dst, nfs, nil)
	if err != nil {
		log.Fatal(fmt.Sprintf("Mount failed: %v\n", err))
	}
	server.Serve()
}


func xdb(msg string) {
	fmt.Printf(msg+"\n")
}
