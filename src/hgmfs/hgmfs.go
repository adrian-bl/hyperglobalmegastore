package hgmfs

import (
	"encoding/json"
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"syscall"
	"time"
)

var hgmConfig *HgmConfig

/* fixme: duplicate code */
type JsonMeta struct {
	Location    [][]string
	Key         string
	Created     int64
	ContentSize uint64
	BlobSize    int64
}

/* Sturct for our pathfs based fuse filesystem */
type HgmFs struct {
	pathfs.FileSystem
}

/* Structure of an open file */
type hgmFile struct {
	sync.Mutex                   // Mutex to lock the struct
	fuseFilename string          // Filename as requested by fuse caller
	jsonMetafile string          // Path to local (matching) json metadata
	jsonMetadata JsonMeta        // Parsed JSON data
	offset       int64           // Current offset of 'resp'
	resp         *http.Response  // An HTTP connection, may be nil
	readQueue    map[int32]int64 // Array with queued read requests, used to minimize 'seeks-over-http'
}

type HgmConfig struct {
	mountpoint string
	proxyUrl string
}

func getLocalPath(fusepath string) string {
	return fmt.Sprintf("./_aliases/%s", fusepath)
}

// Returns TRUE if the offset specified by 'want'
// is the best match in our queue
func (f *hgmFile) nextInQueue(want int64) bool {

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

// Unmarshal JSON file at localPath
func getJsonMeta(localPath string) (JsonMeta, error) {
	var jsm JsonMeta
	content, err := ioutil.ReadFile(localPath)
	if err != nil {
		return jsm, err
	}
	err = json.Unmarshal([]byte(content), &jsm)
	return jsm, err
}

// Unimplemented dummy functions
func (f *hgmFile) Truncate(size uint64) fuse.Status {
	return fuse.EROFS
}
func (f *hgmFile) Allocate(off uint64, sz uint64, mode uint32) fuse.Status {
	return fuse.EROFS
}
func (f *hgmFile) Chmod(mode uint32) fuse.Status {
	return fuse.EROFS
}
func (f *hgmFile) Chown(guid uint32, gid uint32) fuse.Status {
	return fuse.EROFS
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
func (f *hgmFile) SetInode(n *nodefs.Inode) {
}
func (f *hgmFile) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	return fuse.EROFS
}
func (f *hgmFile) Write(content []byte, off int64) (uint32, fuse.Status) {
	return 0, fuse.EROFS
}
func (f *hgmFile) String() string {
	return fmt.Sprintf("FooBar!")
}
func (f *hgmFile) Utimens(a *time.Time, m *time.Time) fuse.Status {
	return fuse.EROFS
}

func (f *hgmFile) StatFS(name string) (*fuse.StatfsOut) {
	sFs := fuse.StatfsOut{Blocks:12345, Bfree: 12345, Bavail: 12345, Files: 0, Ffree: 0, Bsize: 4096, NameLen: 255, Frsize: 3, Padding: 0}
	fmt.Printf("> statfs %s\n", name)
	return &sFs
}

// Open VFS call: Attempts to parse
// the json-metadata and retuns a struct to hgmFile on success
func NewHgmFile(fname string) (nodefs.File, fuse.Status) {
	jmetafile := getLocalPath(fname)
	jmetadata, err := getJsonMeta(jmetafile)
	if err != nil {
		return nil, fuse.EIO
	}

	hgmf := hgmFile{fuseFilename: fname, jsonMetafile: jmetafile, jsonMetadata: jmetadata}
	hgmf.readQueue = make(map[int32]int64)
	return &hgmf, fuse.OK
}

// Open callback, returns a new HgmFile
func (self *HgmFs) Open(fname string, flags uint32, ctx *fuse.Context) (fuseFile nodefs.File, status fuse.Status) {
	return NewHgmFile(fname)
}

// Close VFS call
// Will terminate the http connection if we had one open
func (f *hgmFile) Release() {
	if f.resp != nil {
		f.resp.Body.Close()
	}
}

// VFS Read call: Reads len(dst) bytes at offset off
func (f *hgmFile) Read(dst []byte, off int64) (fuse.ReadResult, fuse.Status) {
	/* Generate a random id, aids printf() debugging, fixme: should be unique */
	rqid := rand.Int31()

	// Inject or request into the queue
	f.Lock()
	f.readQueue[rqid] = off
	f.Unlock()

	// This request will wait in the queue until....
	// -> The HTTP-Client offset is at the correct position, OR
	// -> We waited 3 rounds and are still the best match, OR
	// -> We waited 5 rounds and are the ONLY request, OR
	// -> We got stuck and break out after 10 wait-rounds
	retry := 0
	for loop := true; loop; {
		f.Lock()

		if (f.offset == off) || retry > 2 && f.nextInQueue(off) || (len(f.readQueue) == 1 && retry > 5) || (retry > 10) {
			loop = false
			defer f.Unlock()
		} else {
			f.Unlock()
			retry++
			time.Sleep(0.02 * 1e9)
		}
	}

	fmt.Printf("<%08X> retry=%d, want_off=%d\n", rqid, retry, off)

	// Our open HTTP connection is at the wrong offset.
	// Do a quick-forward if we can or drop it if we seek backwards or to a far pos
	if off != f.offset && f.resp != nil {
		mustSeek := off - f.offset
		wantBlobIdx := int64(off / f.jsonMetadata.BlobSize)
		haveBlobIdx := int64(f.offset / f.jsonMetadata.BlobSize)
		
		fmt.Printf("<%08X> wantIDX=%d (off=%d), gotIDX=%d (off=%d)\n", rqid, wantBlobIdx, off, haveBlobIdx, f.offset);
		
		if mustSeek > 0 && wantBlobIdx == haveBlobIdx {
			fmt.Printf("<%08X> Could do a quick seek by dropping %d bytes\n", rqid, mustSeek)

			// Throw away mustSeek bytes
			for mustSeek != 0 {
				tmpBuf := make([]byte, mustSeek)
				didRead, err := f.resp.Body.Read(tmpBuf)
				if err != nil {
					return nil, fuse.EIO
				}
				mustSeek -= int64(didRead)
				f.offset += int64(didRead)
			}
			fmt.Printf("<%08X> QuickFWD finished\n", rqid)
		} else {
			fmt.Printf("<%08X> !!!!!!!!!!! Closing existing http request: want=%d, got=%d\n", rqid, off, f.offset)
			f.resp.Body.Close()
			f.resp = nil
			f.offset = 0
		}
	}

	// No open http connection: Create a new request
	if f.resp == nil {
		esc := url.QueryEscape(f.fuseFilename)

		fmt.Printf("<%08X> Establishing a new connection, need to seek to %d, fname=%s\n", rqid, off, esc)

		req, err := http.NewRequest("GET", fmt.Sprintf("%s%s", hgmConfig.proxyUrl, esc), nil)
		if err != nil {
			return nil, fuse.EIO
		}

		req.Header.Add("Range", fmt.Sprintf("bytes=%d-", off))
		hclient := &http.Client{}
		resp, err := hclient.Do(req)
		if err != nil {
			return nil, fuse.EIO
		}

		if resp.StatusCode != 200 && resp.StatusCode != 206 {
			fmt.Printf("<%08X> FATAL: Wrong status code: %d (file=%s)\n", rqid, resp.StatusCode, f.fuseFilename)
			resp.Body.Close()
			return nil, fuse.EIO
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

		/* tmpBuf may be larger than didRead: we do not care because
		 * -> it will never overshoot dst (calculated by canRead)
		 * -> we are clamping it later to bytesRead, so we are not
		 *    going to return junk to the caller
		 */
		copy(dst[bytesRead:], tmpBuf)

		bytesRead += didRead
	}

	f.offset += int64(bytesRead)
	delete(f.readQueue, rqid)
	rr := fuse.ReadResultData(dst[0:bytesRead])
	return rr, fuse.OK
}

// Returns the filesystem attributes for given file using
// the data (inode, permissions, etc) from the json file
func (f *hgmFile) GetAttr(attr *fuse.Attr) fuse.Status {
	st := syscall.Stat_t{}
	err := syscall.Stat(f.jsonMetafile, &st)
	if err != nil {
		return fuse.EIO /* open file vanished?! */
	}
	attr.FromStat(&st)
	attr.Size = f.jsonMetadata.ContentSize
	return fuse.OK
}

// Returns the attributes for a non-open file (including directories)
func (self *HgmFs) GetAttr(fname string, ctx *fuse.Context) (*fuse.Attr, fuse.Status) {
	jsonPath := getLocalPath(fname)
	st := syscall.Stat_t{}
	err := syscall.Stat(jsonPath, &st)
	if err != nil {
		return nil, fuse.ENOENT
	}

	attr := &fuse.Attr{}
	attr.FromStat(&st)

	if (attr.Mode & fuse.S_IFDIR) == 0 {
		// This is a file: need to parse json in order to get the filesize
		jsonMeta, _ := getJsonMeta(jsonPath) /* fsize will be 0 on error, that's ok with us */
		attr.Size = jsonMeta.ContentSize
	}

	return attr, fuse.OK
}

// Read all directory entries for fname
func (self *HgmFs) OpenDir(fname string, ctx *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	path := getLocalPath(fname)
	dirList, err := ioutil.ReadDir(path)

	if err == nil {
		fuseDirs := make([]fuse.DirEntry, 0, len(dirList))
		for _, fi := range dirList {
			dirent := fuse.DirEntry{Name: fi.Name(), Mode: 0}
			ea, ee := self.GetAttr(fmt.Sprintf("%s/%s", fname, dirent.Name), ctx)
			if ee == fuse.OK {
				dirent.Mode = ea.Mode
			}
			fuseDirs = append(fuseDirs, dirent)
		}
		return fuseDirs, fuse.OK
	}
	return nil, fuse.EIO
}

func MountFilesystem(mountpoint string, proxy string) {
	nfs := pathfs.NewPathNodeFs(&HgmFs{FileSystem: pathfs.NewDefaultFileSystem()}, nil)
	server, _, err := nodefs.MountRoot(mountpoint, nfs.Root(), nil)
	if err != nil {
		log.Fatal(fmt.Sprintf("Mount failed: %v\n", err))
	}
	fmt.Printf("Filesystem mounted at '%s', using '%s' as upstream source\n", mountpoint, proxy)

	hgmConfig = new (HgmConfig)

	hgmConfig.mountpoint = mountpoint
	hgmConfig.proxyUrl = proxy

	server.Serve()
}
