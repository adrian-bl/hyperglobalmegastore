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
	Size      int64
	Blocks    int64
	Atime     uint64
	Mtime     uint64
	Ctime     uint64
	Mode      uint32
	Nlink     uint64
	Uid       uint32
	Gid       uint32
	Rdev      uint64
	BlockSize int64
}

/**
 * Calls readdir on a local path, returns an array of HgmStatDirent
 */
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

/**
 * Stats a local file
 */
func LocalStat(path string) (*HgmStatAttr, error) {
	stat := syscall.Stat_t{}
	err := syscall.Stat(path, &stat)

	if err != nil {
		return nil, err
	}

	a := &HgmStatAttr{
		Inode:     stat.Ino,
		Size:      stat.Size,
		Blocks:    stat.Blocks,
		Atime:     0,
		Mtime:     0,
		Ctime:     0,
		Mode:      stat.Mode,
		Nlink:     stat.Nlink,
		Uid:       stat.Uid,
		Gid:       stat.Gid,
		Rdev:      stat.Rdev, // ??
		BlockSize: stat.Blksize,
	}

	return a, nil
}

/**
 * Converts errors returned by stattool to an HTTP status
 */
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
