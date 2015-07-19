package hgmfs

import (
	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"encoding/json"
	"fmt"
	"golang.org/x/net/context"
	"io"
	"io/ioutil"
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
	hgmFs      HgmFs
	localFile  string
	blobSize   int64
	resp       *http.Response // An HTTP connection, may be nil
	offset     int64          // Current offset of 'resp'
}

/* fixme: duplicate code */
type JsonMeta struct {
	Location    [][]string
	Key         string
	Created     int64
	ContentSize uint64
	BlobSize    int64
}

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
	return HgmDir{hgmFs: fs, localDir: getMetaRoot()}, nil
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
	jsonMeta, jsonErr := getJsonMeta(localDirent)
	if jsonErr == nil {
		return &HgmFile{hgmFs: dir.hgmFs, localFile: localDirent, blobSize: jsonMeta.BlobSize}, nil
	}

	// JSON was bad: return io error
	return nil, fuse.EIO
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
	if file.resp != nil {
		file.resp.Body.Close()
	}
	fmt.Printf("<%08X> Closed due to RELEASE()\n", req.Handle)
	return nil
}

func (file *HgmFile) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	rqid := req.Handle
	off := req.Offset

	// Our open HTTP connection is at the wrong offset.
	// Do a quick-forward if we can or drop it if we seek backwards or to a far pos
	if off != file.offset && file.resp != nil {
		mustSeek := off - file.offset
		wantBlobIdx := int64(off / file.blobSize)         // Blob we want to start reading from
		haveBlobIdx := int64(file.offset / file.blobSize) // Blob of currently open connection
		keepConn := false

		if mustSeek > 0 && wantBlobIdx == haveBlobIdx {
			err := discardBody(mustSeek, file.resp.Body)
			if err == nil {
				file.offset += mustSeek
				keepConn = true
				fmt.Printf("<%08X> skipped %d bytes via fast-forward\n", rqid, mustSeek)
			}
		}

		// Close and reset the connection if we failed to do a fast forward
		// This may even happen if everything looked fine: The Go Net-GC might have
		// killed the http connection
		if keepConn == false {
			file.resp.Body.Close()
			file.resp = nil
			file.offset = 0
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

		if resp.StatusCode != 200 && resp.StatusCode != 206 {
			fmt.Printf("<%08X> FATAL: Wrong status code: %d (file=%s)\n", rqid, resp.StatusCode, file.localFile)
			resp.Body.Close()
			return fuse.EIO
		} else if resp.StatusCode == 200 && off != 0 {
			fmt.Printf("<%08x> Server was unable to fulfill request for offset %d -> reading up to destination\n", rqid, off)
			err = discardBody(off, resp.Body)
			if err != nil {
				return fuse.EIO
			}
		}
		file.offset = off
		file.resp = resp
	}

	bytesRead := 0
	mustRead := req.Size
	resp.Data = make([]byte, mustRead)

	for bytesRead != mustRead {
		canRead := mustRead - bytesRead
		tmpBuf := make([]byte, canRead)
		didRead, err := file.resp.Body.Read(tmpBuf)
		if err != nil && didRead == 0 {
			break
		}
		copy(resp.Data[bytesRead:], tmpBuf[:didRead])
		bytesRead += didRead
	}

	file.offset += int64(bytesRead)

	return nil
}


// Skips X bytes from an io.ReadCloser (fixme: is there a library function?!)
func discardBody(toSkip int64, reader io.ReadCloser) error {
	maxBufSize := int64(1024 * 1024) // Keep at most 1MB in memory

	for toSkip != 0 {
		tmpSize := toSkip
		if tmpSize > maxBufSize {
			tmpSize = maxBufSize
		}
		tmpBuf := make([]byte, tmpSize)
		didSkip, err := reader.Read(tmpBuf)
		if err != nil {
			return err
		}
		toSkip -= int64(didSkip)
	}
	return nil
}
