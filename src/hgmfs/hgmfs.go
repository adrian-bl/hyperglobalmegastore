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
	"libhgms/stattool"
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
	connected bool
	offset    int64 // Current offset of 'resp'
}

var lruBlockSize = uint32(4096)
var lruMaxItems = uint32(32768) // how many lruBlockSize sized items we are storing
var lruCache *ssc.Cache

// Some handy shared statistics
var hgmStats = struct {
	lruEvicted int64
	bytesHit   int64
	bytesMiss  int64
}{}

/**
 * Initialized the mount process, called by hgmcmd
 */
func MountFilesystem(mountpoint string, proxy string) {

	// The proxy URL should end with a slash, add it if the user forgot about this
	if proxy[len(proxy)-1] != '/' {
		proxy += "/"
	}

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
	return &HgmDir{hgmFs: fs, localDir: "/"}, nil
}

/**
 * Stat()'s the current directory
 */
func (dir HgmDir) Attr(ctx context.Context, a *fuse.Attr) error {
	resp, err := http.Get(dir.getStatEndpoint(dir.localDir, false))
	if err != nil {
		return fuse.EIO
	}

	fuseErr := stattool.HttpStatusToFuseErr(resp.StatusCode)
	if fuseErr == nil {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err == nil {
			attr := stattool.HgmStatAttr{}
			err = json.Unmarshal(bodyBytes, &attr)
			if err == nil {
				stattool.AttrFromHgmStat(attr, a)
			}
		}
		if err != nil {
			fuseErr = fuse.EIO
		}
	}

	return fuseErr
}

/**
 * Stat()'s the current file
 */
func (file *HgmFile) Attr(ctx context.Context, a *fuse.Attr) error {
	// The directory stat implementation also works for files
	d := HgmDir{hgmFs: file.hgmFs, localDir: file.localFile}
	err := d.Attr(ctx, a)
	return err
}

/**
 * Performs a lookup-op and returns a file or dir-handle, depending on the file type
 */
func (dir HgmDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	localDirent := dir.localDir + name // dirs are ending with a slash -> just append the name
	a := fuse.Attr{}
	d := HgmDir{hgmFs: dir.hgmFs, localDir: localDirent}
	err := d.Attr(ctx, &a)

	if err != nil {
		return nil, err
	}

	if (a.Mode & os.ModeType) == os.ModeDir {
		return HgmDir{hgmFs: dir.hgmFs, localDir: localDirent + "/"}, nil
	}

	return &HgmFile{hgmFs: dir.hgmFs, localFile: localDirent, blobSize: int64(a.Size)}, nil
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
	resp, err := http.Get(dir.getStatEndpoint(dir.localDir, true))
	if err != nil {
		return nil, fuse.EIO
	}

	fuseDirList := make([]fuse.Dirent, 0)
	fuseErr := stattool.HttpStatusToFuseErr(resp.StatusCode)

	if fuseErr == nil {
		hgmDirList := []stattool.HgmStatDirent{}
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err == nil {
			err = json.Unmarshal(bodyBytes, &hgmDirList)
			if err == nil {
				for _, v := range hgmDirList {
					fuseType := fuse.DT_File
					if v.IsDir == true {
						fuseType = fuse.DT_Dir
					}
					fuseDirList = append(fuseDirList, fuse.Dirent{Inode: 0, Name: v.Name, Type: fuseType})
				}
			}
		}
		if err != nil {
			fuseErr = fuse.EIO
		}
	}

	return fuseDirList, fuseErr
}

// Returns URL to query the stat service
func (dir HgmDir) getStatEndpoint(path string, readdir bool) string {
	pathUrl := url.URL{Path: path}
	endpoint := fmt.Sprintf("%s%s%s", dir.hgmFs.proxyUrl, stattool.StatSvcEndpoint, pathUrl.String())
	if readdir == true {
		endpoint += "?op=readdir"
	}
	fmt.Printf("GET %s\n", endpoint)
	return endpoint
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
	if off != file.offset && file.connected == true {
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
	if file.connected == false {
		linkURL := &url.URL{Path: file.localFile}
		linkName := linkURL.String()
		fmt.Printf("<%08X> Establishing a new connection, need to seek to %d, fname=%s\n", rqid, off, linkName)

		// skip first char in filename as this would be the fs root (/)
		req, err := http.NewRequest("GET", fmt.Sprintf("%s%s", file.hgmFs.proxyUrl, linkName[1:]), nil)
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

		// got our connection: set it up
		file.offset = off
		file.resp = resp
		file.connected = true

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
	err := file.readBody(int64(req.Size), &resp.Data)

	if err != nil && err != io.EOF {
		err = fuse.EIO
	} else {
		err = nil // clear EOF
	}

	return err
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
	file.connected = false
	file.offset = 0
}
