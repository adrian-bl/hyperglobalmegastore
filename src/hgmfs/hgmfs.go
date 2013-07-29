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
	lock sync.Mutex
	fuseFilename string
	jsonMetafile string
	jsonMetadata JsonMeta
	offset int64
	resp *http.Response
}



func NewHgmFile(fname string) nodefs.File {
	fmt.Printf(">> newhgm file: %s\n", fname)
	jmetafile := getLocalPath(fname)
	jmetadata, err := getJsonMeta(jmetafile)
	if err != nil {
		return nil
	}
	
	hgmf := hgmFile{fuseFilename: fname, jsonMetafile: jmetafile, jsonMetadata: jmetadata}
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
	fmt.Printf("[close] fname=%s, resp=%s\n", f.fuseFilename, f.resp)
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
func (f *hgmFile) Read(dst []byte, off int64) (fuse.ReadResult, fuse.Status) {
	fmt.Printf("READ fname=%s, offset=%d, myoff=%d\n", f.fuseFilename, off, f.offset)
	for i:=0;i<5;i++ {
		if off != f.offset {
			fmt.Printf("want %d, have %d -> wait\n", off, f.offset)
			time.Sleep(0.02*1e9)
		} else {
			break
		}
	}
	
	f.lock.Lock()
	defer f.lock.Unlock()
	
	if off != f.offset && f.resp != nil {
		fmt.Printf("Closing existing http request to fulfill request for offset %d\n", off)
		f.offset = 0
		f.resp.Body.Close()
		f.resp = nil
	}
	
	if f.resp == nil {
		fmt.Printf("--> first request for %s, starting http request at offset %d\n", f.fuseFilename, off)
		
		req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:8080/%s", f.fuseFilename), nil)
		if err != nil {
			return nil, fuse.EPERM
		}
		req.Header.Add("Range", fmt.Sprintf("bytes=%d-", off))
		
		clnt := &http.Client{}
		resp, err := clnt.Do(req)
		if err != nil {
			return nil, fuse.EPERM
		}
		
		if resp.StatusCode != 200 && resp.StatusCode != 206 {
			fmt.Printf("!! wrong code: %d\n", resp.StatusCode)
			f.resp = nil
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
		if err != nil && didRead == 0{
			break
		}
		
		for i:=0;i<didRead;i++ {
			dst[bytesRead+i] = tmpBuf[i]
		}
		
		bytesRead += didRead
	}
	
	f.offset += int64(bytesRead)
	rr := fuse.ReadResultData(dst[0:bytesRead])
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
