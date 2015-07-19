package hgmfs

import (
	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"encoding/json"
	"fmt"
	"golang.org/x/net/context"
	"io/ioutil"
	"log"
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
func getLocalPath(fusepath string) string {
	return fmt.Sprintf("./_aliases/%s", fusepath)
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
	return HgmDir{hgmFs: fs, localDir: getLocalPath("./")}, nil
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
	fmt.Printf("STAT(%s) finished\n", dir.localDir)
	return nil

}

/**
 * Stat()'s the current file
 */
func (file HgmFile) Attr(ctx context.Context, a *fuse.Attr) error {
	st := syscall.Stat_t{}
	err := syscall.Stat(file.localFile, &st)
	if err != nil {
		return fuse.ENOENT
	}
	attrFromStat(st, a)

	// This is a file, so we are delivering the filesize of the actual content
	jsonMeta, _ := getJsonMeta(file.localFile) // Filesize will be '0' on error, that's ok for us
	a.Size = jsonMeta.ContentSize

	fmt.Printf("fSTAT(%s) finished\n", file.localFile)
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

	fmt.Printf("LOOKUP(%s)\n", localDirent)
	if stat.IsDir() {
		return HgmDir{hgmFs: dir.hgmFs, localDir: localDirent + "/"}, nil
	}
	// else
	return HgmFile{hgmFs: dir.hgmFs, localFile: localDirent}, nil
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
