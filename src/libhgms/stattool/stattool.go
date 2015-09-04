/*
 * Copyright (C) 2015 Adrian Ulrich <adrian@blinkenlights.ch>
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

package stattool

import (
	"bazil.org/fuse"
	"encoding/json"
	"io/ioutil"
	"os"
	"syscall"
)

type HgmStatDirent struct {
	Name  string `json:"Name"`
	IsDir bool   `json:"IsDir"`
}

type HgmStatAttr struct {
	Inode     uint64
	Size      uint64
	Blocks    uint64
	Atime     uint64
	Mtime     uint64
	Ctime     uint64
	Mode      uint32
	Nlink     uint64
	Uid       uint32
	Gid       uint32
	BlockSize uint64
	IsDir     bool
}

type JsonMeta struct {
	Location    [][]string
	Key         string
	Created     int64
	ContentSize uint64
	BlobSize    int64
}

// Calls readdir on a local path, returns an array of HgmStatDirent entries
func LocalReadDir(path string) ([]HgmStatDirent, error) {
	sysDirList, err := ioutil.ReadDir(path)

	if err != nil {
		e, ok := err.(*os.PathError)
		if ok == false {
			panic(err)
		}
		return nil, e.Err
	}

	dirList := make([]HgmStatDirent, 0)
	for _, fi := range sysDirList {
		dirent := HgmStatDirent{Name: fi.Name(), IsDir: fi.IsDir()}
		dirList = append(dirList, dirent)
	}
	return dirList, nil
}

// Stats a local (json) file
func LocalStat(path string) (*HgmStatAttr, error) {
	stat := syscall.Stat_t{}
	err := syscall.Stat(path, &stat)

	if err != nil {
		return nil, err
	}

	// Drops all non-permission flags
	modePerm := (stat.Mode & uint32(os.ModePerm))

	isDir := false
	if (stat.Mode & syscall.S_IFMT) == syscall.S_IFDIR {
		isDir = true
	}

	a := &HgmStatAttr{
		Inode:     stat.Ino,
		Size:      uint64(stat.Size),
		Blocks:    uint64(stat.Blocks),
		Atime:     0,
		Mtime:     0,
		Ctime:     0,
		Mode:      modePerm,
		IsDir:     isDir,
		Nlink:     stat.Nlink,
		Uid:       stat.Uid,
		Gid:       stat.Gid,
		BlockSize: uint64(stat.Blksize),
	}

	// Get size of content if this is a file (eg: we got json info)
	if isDir == false {
		a.Size = 0   // invalidate size
		a.Blocks = 0 // zero size uses zero blocks

		jContent, jErr := ioutil.ReadFile(path)
		if jErr == nil {
			jStruct := JsonMeta{}
			jErr = json.Unmarshal([]byte(jContent), &jStruct)
			if jErr == nil {
				a.Size = jStruct.ContentSize
				a.Blocks = 1 + (a.Size / a.BlockSize) // not using math.Ceil() for this: Blocks is foobar anyway
			}
		}
		// else: size will be zero
	}

	return a, nil
}

// Converts an HgmStatAttr struct to a fuse.Attr struct
func AttrFromHgmStat(hgm HgmStatAttr, a *fuse.Attr) {
	a.Inode = hgm.Inode
	a.Size = uint64(hgm.Size)
	a.Blocks = uint64(hgm.Blocks)
	//	a.Atime
	//	a.Mtime
	//	a.Ctime
	a.Mode = os.FileMode(hgm.Mode)
	a.Nlink = uint32(hgm.Nlink)
	a.Uid = hgm.Uid
	a.Gid = hgm.Gid
	a.Rdev = 0
	a.BlockSize = uint32(hgm.BlockSize)

	if hgm.IsDir == true {
		a.Mode |= os.ModeDir
	}

}

// Translates errors returned by stattool into an HTTP status
func SysErrToHttpStatus(syserr error) int {
	switch syserr {
	case nil:
		return 200
	case syscall.EPERM:
		return 403
	case syscall.ENOENT:
		return 404
	case syscall.EACCES:
		return 405
	}
	return 500
}

// Translates HTTP codes returned by SysErrToHttpStatus into a fuse error
func HttpStatusToFuseErr(status int) error {
	switch status {
	case 200:
		return nil
	case 403:
		return fuse.EPERM
	case 404:
		return fuse.ENOENT
	case 405:
		return fuse.EPERM // fuse has no EACCES ?
	}
	return fuse.EIO
}
