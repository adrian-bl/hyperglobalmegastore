package hgmfs

import (
	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"encoding/json"
	"fmt"
	"golang.org/x/net/context"
	"io"
	"io/ioutil"
	"libhgms/ssc"
	"log"
	"net/http"
	"net/url"
	"os"
	"syscall"
	"time"
)

type HgmFs struct {
	mountPoint string
	proxyUrl   string
}

type HgmDir struct {
	hgmFs    HgmFs
	localDir string
}

type HgmFile struct {
	hgmFs     HgmFs
	localFile string
	blobSize  int64
	resp      *http.Response // An HTTP connection, may be nil
	offset    int64          // Current offset of 'resp'
}

/* fixme: duplicate code */
type JsonMeta struct {
	Location    [][]string
	Key         string
	Created     int64
	ContentSize uint64
	BlobSize    int64
}

var lruBlockSize = uint64(4096)
var lruMaxItems = uint64(8192) // how many lruBlockSize sized items we are storing
var lruCache *ssc.Cache

// Some handy shared statistics
var hgmStats = struct {
	lruEvicted int64
	bytesHit   int64
	bytesMiss  int64
}{}

/**
 * Returns the root-node point
 */
func getMetaRoot() string {
	return fmt.Sprintf("./_aliases/")
}

func dropMetaRoot(fusestring string) string {
	return fusestring[len(getMetaRoot()):]
}

/**
 * Converts syscall-stat to fuse-stat
 */
func attrFromStat(st syscall.Stat_t, a *fuse.Attr) {
	a.Inode = st.Ino
	a.Size = uint64(st.Size)
	a.Blocks = uint64(st.Blocks)
	a.Atime = time.Unix(st.Atim.Sec, st.Atim.Nsec)
	a.Mtime = time.Unix(st.Mtim.Sec, st.Mtim.Nsec)
	a.Ctime = time.Unix(st.Ctim.Sec, st.Ctim.Nsec)
	a.Mode = os.FileMode(st.Mode)
	a.Nlink = uint32(st.Nlink)
	a.Uid = st.Uid
	a.Gid = st.Gid
	a.Rdev = uint32(st.Rdev)
	a.BlockSize = uint32(st.Blksize)
}

/**
 * Returns the decoded json-meta information
 */
func getJsonMeta(localPath string) (JsonMeta, error) {
	var jsm JsonMeta
	content, err := ioutil.ReadFile(localPath)
	if err != nil {
		return jsm, err
	}
	err = json.Unmarshal([]byte(content), &jsm)
	return jsm, err
}

/**
 * Initialized the mount process, called by hgmcmd
 */
func MountFilesystem(mountpoint string, proxy string) {
	c, err := fuse.Mount(
		mountpoint,
		fuse.FSName(fmt.Sprintf("hgmsfs(%s)", proxy)),
		fuse.Subtype("hgmfs"),
		fuse.LocalVolume(),
		fuse.VolumeName("hgms-volume"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	if lruCache == nil {
		lruCache, err = ssc.New("./ssc.db", lruBlockSize, lruMaxItems)
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Printf("Serving FS at '%s' (lru_cache=%.2fMB)\n", mountpoint, float64(lruBlockSize*lruMaxItems/1024/1024))

	err = fs.Serve(c, HgmFs{mountPoint: mountpoint, proxyUrl: proxy})

	if err != nil {
		log.Fatal(err)
	}

	// check if the mount process has an error to report
	<-c.Ready
	if err := c.MountError; err != nil {
		log.Fatal(err)
	}
}

/**
 * Returns the filesystem root node (a directory)
 */
func (fs HgmFs) Root() (fs.Node, error) {
	return &HgmDir{hgmFs: fs, localDir: getMetaRoot()}, nil
}

/**
 * Stat()'s the current directory
 */
func (dir HgmDir) Attr(ctx context.Context, a *fuse.Attr) error {
	st := syscall.Stat_t{}
	err := syscall.Stat(dir.localDir, &st)
	if err != nil {
		return fuse.ENOENT
	}

	attrFromStat(st, a)
	a.Mode = os.ModeDir | a.Mode
	return nil
}

/**
 * Stat()'s the current file
 */
func (file *HgmFile) Attr(ctx context.Context, a *fuse.Attr) error {
	st := syscall.Stat_t{}
	err := syscall.Stat(file.localFile, &st)
	if err != nil {
		return fuse.ENOENT
	}
	attrFromStat(st, a)

	// This is a file, so we are delivering the filesize of the actual content
	jsonMeta, _ := getJsonMeta(file.localFile) // Filesize will be '0' on error, that's ok for us
	a.Size = jsonMeta.ContentSize
	return nil
}

/**
 * Performs a lookup-op and returns a file or dir-handle, depending on the file type
 */
func (dir HgmDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	localDirent := dir.localDir + name // dirs are ending with a slash, so we can just append
	stat, err := os.Stat(localDirent)
	if err != nil {
		return nil, fuse.ENOENT
	}

	if stat.IsDir() {
		return HgmDir{hgmFs: dir.hgmFs, localDir: localDirent + "/"}, nil
	}

	// Probably a file:
	hgmFile := &HgmFile{hgmFs: dir.hgmFs, localFile: localDirent, blobSize: 0}
	jsonMeta, jsonErr := getJsonMeta(localDirent)
	if jsonErr == nil {
		// we could parse the json, so we know the blobsize
		// a file with a blobsize of 0 would be considered to be empty
		hgmFile.blobSize = jsonMeta.BlobSize
	}

	return hgmFile, nil
}

/**
 * Set flags during file open()
 */
func (file *HgmFile) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	if !req.Flags.IsReadOnly() {
		return nil, fuse.Errno(syscall.EACCES)
	}
	//	resp.Flags |= fuse.OpenDirectIO
	resp.Flags |= fuse.OpenKeepCache // we are readonly: allow the OS to cache our result
	return file, nil
}

/**
 * Dummy-out common write requests
 */
func (dir *HgmDir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	return nil, nil, fuse.Errno(syscall.EROFS)
}
func (dir *HgmDir) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	return fuse.Errno(syscall.EROFS)
}
func (dir *HgmDir) Rename(ctx context.Context, req *fuse.RenameRequest, newDir fs.Node) error {
	return fuse.Errno(syscall.EROFS)
}

/**
 * Returns a complete directory list
 */

func (dir HgmDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	sysDirList, err := ioutil.ReadDir(dir.localDir)
	if err != nil {
		return nil, fuse.EIO
	}

	fuseDirList := make([]fuse.Dirent, 0)
	for _, fi := range sysDirList {
		fuseType := fuse.DT_File
		if fi.IsDir() {
			fuseType = fuse.DT_Dir
		}
		dirent := fuse.Dirent{Inode: 0, Name: fi.Name(), Type: fuseType}
		fuseDirList = append(fuseDirList, dirent)
	}
	return fuseDirList, nil
}

func (file *HgmFile) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	file.resetHandle()
	return nil
}

func (file *HgmFile) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	rqid := req.Handle
	off := req.Offset

	// quickly abort on pseudo-empty files
	if file.blobSize == 0 {
		return nil
	}

	cacheData, cacheOk := lruCache.Get(file.lruKey(off))
	if cacheOk {
		resp.Data = cacheData
		if len(resp.Data) > req.Size {
			// chop off if we got too much data
			resp.Data = resp.Data[:req.Size]
		}

		hgmStats.bytesHit += int64(len(resp.Data))
		return nil
	}

	// Our open HTTP connection is at the wrong offset.
	// Do a quick-forward if we can or drop it if we seek backwards or to a far pos
	if off != file.offset && file.resp != nil {
		mustSeek := off - file.offset
		wantBlobIdx := int64(off / file.blobSize)         // Blob we want to start reading from
		haveBlobIdx := int64(file.offset / file.blobSize) // Blob of currently open connection
		keepConn := false

		if mustSeek > 0 && wantBlobIdx == haveBlobIdx {
			err := file.readBody(mustSeek, nil)
			if err == nil {
				keepConn = true
				fmt.Printf("<%08X> skipped %d bytes via fast-forward, now at: %d\n", rqid, mustSeek, file.offset)
			}
		}

		// Close and reset the connection if we failed to do a fast forward
		// This may even happen if everything looked fine: The Go Net-GC might have
		// killed the http connection
		if keepConn == false {
			file.resetHandle()
			fmt.Printf("<%08X> connection was reset (mm=%d, wb=%d, hb=%d)\n", rqid, mustSeek, wantBlobIdx, haveBlobIdx)
		}
	}

	// No open http connection: Create a new request
	if file.resp == nil {
		linkURL := &url.URL{Path: file.localFile}
		linkName := linkURL.String()
		fmt.Printf("<%08X> Establishing a new connection, need to seek to %d, fname=%s\n", rqid, off, linkName)

		req, err := http.NewRequest("GET", fmt.Sprintf("%s%s", file.hgmFs.proxyUrl, dropMetaRoot(linkName)), nil)
		if err != nil {
			return fuse.EIO
		}

		req.Header.Add("Range", fmt.Sprintf("bytes=%d-", off))
		tr := &http.Transport{ResponseHeaderTimeout: 15 * time.Second, Proxy: http.ProxyFromEnvironment}
		hclient := &http.Client{Transport: tr}
		resp, err := hclient.Do(req)
		if err != nil {
			return fuse.EIO
		}

		file.offset = off
		file.resp = resp
		if resp.StatusCode != 200 && resp.StatusCode != 206 {
			fmt.Printf("<%08X> FATAL: Wrong status code: %d (file=%s)\n", rqid, resp.StatusCode, file.localFile)
			file.resetHandle()
			return fuse.EIO
		} else if resp.StatusCode == 200 && off != 0 {
			fmt.Printf("<%08x> Server was unable to fulfill request for offset %d -> reading up to destination\n", rqid, off)
			file.offset = 0 // we are at the beginning
			err = file.readBody(off, nil)
			if err != nil {
				file.resetHandle()
				return fuse.EIO
			}
		}
	}

	resp.Data = make([]byte, 0, req.Size)
	file.readBody(int64(req.Size), &resp.Data)

	return nil
}

// Discards count bytes from the filehandle connected
// to the HgmFile descriptor
// Will put a copy of the read data into copySink if non nil
// The code will not expand/make copySink!
func (file *HgmFile) readBody(count int64, copySink *[]byte) (err error) {

	for count != 0 {
		// Creates a sink which we are going to use as our read buffer
		// note that this is re-allocated on each loop as we may pass
		// this reference to lruCache and must avoid overwriting it afterwards
		byteSink := make([]byte, lruBlockSize)
		if int64(len(byteSink)) > count {
			// shrink buffer size if we got less to read than allocated
			byteSink = byteSink[:count]
		}

		nr := 0
		for nr != len(byteSink) {
			rb, re := file.resp.Body.Read(byteSink[nr:])
			nr += rb
			if re != nil {
				// may be a partial read with EOF on error
				err = re
				break
			}
		}

		if copySink != nil {
			*copySink = append(*copySink, byteSink[:nr]...)
			if file.offset%int64(lruBlockSize) == 0 && nr > 0 && (err == nil || err == io.EOF) {
				// Cache whatever we got from a lruBlockSize boundary
				// this will always be <= lruBlockSize
				evicted := lruCache.Add(file.lruKey(file.offset), byteSink[:nr])
				if evicted {
					hgmStats.lruEvicted++
				}
				hgmStats.bytesMiss += int64(nr)
			}
		}

		file.offset += int64(nr)
		count -= int64(nr)

		if err != nil {
			break
		}

	}
	return err
}

// Returns the cache key used for our in-memory LRU cache
func (file HgmFile) lruKey(offset int64) string {
	return fmt.Sprintf("%d/%s", offset, file.localFile)
}

func (file *HgmFile) resetHandle() {
	if file.resp != nil {
		file.resp.Body.Close()
		file.resp = nil
	}
	file.offset = 0
}
