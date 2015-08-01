package ssc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc64"
	"math/rand"
	"os"
	"sync"
)

var ErrCorruptedDb = errors.New("Database is corrupted")

var SUPER_MAGIC = [4]byte{'!', 's', 's', 'c'}
var SUPER_VERSION = uint8(1)

const SUPERBLOCK_SIZE = uint64(4096)

type Superblock struct {
	Magic      [4]byte    // magic-4-byte sequence
	Version    uint8      // revision of database layout
	ChunkSize  uint64     // size in bytes of a single chunk
	ChunkCount uint64     // how many chunks this database is storing
	Padding    [4075]byte // align to 4K
}

const METAENTRY_SIZE = 8 * 3

type MetaEntry struct {
	Key      uint64 // crc64 key
	Len      uint64 // lenght of this slice (might be < chunksize!)
	Checksum uint64 // calculated on-disk checksum
}

type chunkmapEntry struct {
	chunk uint64 // chunk index we are pointing to
	dirty bool   // true if the on-disk data needs to be validated
}

type Cache struct {
	chunkSize  uint64                   // size in bytes of a single chunk
	chunkCount uint64                   // amount of chunks we are storing
	metaMap    []MetaEntry              // mapping of chunk -> MetaEntry
	chunkMap   map[uint64]chunkmapEntry // mapping of hash -> chunk
	nextChunk  uint64                   // next chunk to use for writing
	hasher     hash.Hash64              // hasher implementation
	fh         *os.File                 // filehandle pointing to our database
	mutex      *sync.RWMutex            // cache-wide lock for io and slice operations
}

// Returns an initialized Cache handle with sane defaults
func New(dbpath string, chunksize uint64, chunkcount uint64) (*Cache, error) {
	c := &Cache{
		chunkSize:  chunksize,
		chunkCount: chunkcount,
		metaMap:    make([]MetaEntry, chunkcount),
		chunkMap:   make(map[uint64]chunkmapEntry, chunkcount),
		nextChunk:  uint64(rand.Int63n(int64(chunkcount))), // start at a random index
		hasher:     crc64.New(crc64.MakeTable(crc64.ECMA)),
		mutex:      &sync.RWMutex{},
	}
	err := c.openDbFile(dbpath)
	return c, err
}

// Closes an open cache handle (that is: closing the filehandle pointing to the database)
func (c *Cache) Close() {
	c.fh.Close()
}

// Adds a new key to the cache
// returns false if the data was not added to the cache
func (c *Cache) Add(key string, value []byte) bool {
	kh := c.hash64([]byte(key))

	c.mutex.Lock()
	_, addFail := c.chunkMap[kh]

	if addFail == false {
		// cache miss: we can write to the index of nextChunk
		chunk := c.nextChunk
		// this index will be changed -> update in-memory struct
		oldMeta := c.metaMap[chunk]
		newMeta := MetaEntry{Key: kh, Len: uint64(len(value)), Checksum: c.hash64(value)}

		c.replaceMeta(oldMeta.Key, newMeta, false)

		c.seekToData(chunk)
		binary.Write(c.fh, binary.LittleEndian, value)

		c.nextChunk++
		if c.nextChunk >= c.chunkCount {
			c.nextChunk = 0 // overflowed -> next chunk shall be 0
		}
	}
	c.mutex.Unlock()
	return !addFail
}

// Performs a lookup of 'key' in the cache
// 'ok' will be true if the data could be found in the cache
func (c *Cache) Get(key string) (data []byte, ok bool) {
	kh := c.hash64([]byte(key))

	c.mutex.RLock()
	chunkEntry, ok := c.chunkMap[kh]

	if ok {
		memMeta := c.metaMap[chunkEntry.chunk]
		data = make([]byte, memMeta.Len)

		c.seekToData(chunkEntry.chunk)
		data = make([]byte, memMeta.Len)
		binary.Read(c.fh, binary.LittleEndian, data)

		if chunkEntry.dirty == true { // old data: verify on-disk checksum
			calcChecksum := c.hash64(data)

			if calcChecksum == memMeta.Checksum {
				chunkEntry.dirty = false // passed -> do not re-verify
				c.chunkMap[kh] = chunkEntry
			} else {
				fmt.Printf("Corrupted block detected: %d - Checksum %X != %X\n", chunkEntry.chunk, calcChecksum, memMeta.Checksum)
				fakeKey := c.findFreeKey(0)
				c.replaceMeta(kh, MetaEntry{Key: fakeKey}, true)
				data = make([]byte, 0)
				ok = false
			}
		}
	}

	c.mutex.RUnlock()
	return
}

func (c *Cache) replaceMeta(oldKey uint64, newMeta MetaEntry, dirty bool) {
	chunk := c.chunkMap[oldKey].chunk
	delete(c.chunkMap, oldKey)
	c.chunkMap[newMeta.Key] = chunkmapEntry{chunk: chunk, dirty: dirty}
	c.metaMap[chunk] = newMeta

	c.seekToMeta(chunk)
	binary.Write(c.fh, binary.LittleEndian, newMeta)

	if len(c.chunkMap) != len(c.metaMap) || uint64(len(c.metaMap)) != c.chunkCount {
		panic("Corrupted mapping!")
	}
}

// Returns a CRC64 sum of b
func (c *Cache) hash64(b []byte) uint64 {
	c.hasher.Reset()
	c.hasher.Write(b)
	return c.hasher.Sum64()
}

// Seeks to given chunk-position in the metadata part
func (c *Cache) seekToMeta(chunk uint64) error {
	_, err := c.fh.Seek(int64(SUPERBLOCK_SIZE*1+METAENTRY_SIZE*chunk), 0)
	return err
}

// Seeks to given chunk-position in the data part
func (c *Cache) seekToData(chunk uint64) error {
	_, err := c.fh.Seek(int64(SUPERBLOCK_SIZE*1+METAENTRY_SIZE*c.chunkCount+chunk*c.chunkSize), 0)
	return err
}

// Opens the ssc database file
// A new file will be created if the specified path does not exist
// The file will be initialized if the given file is 0 bytes (or did not exist)
func (c *Cache) openDbFile(dbpath string) error {
	fh, err := os.OpenFile(dbpath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}

	stat, err := fh.Stat()
	if err != nil {
		return err
	}

	expectedDbSize := int64(SUPERBLOCK_SIZE*1 + METAENTRY_SIZE*c.chunkCount + c.chunkCount*c.chunkSize)
	if stat.Size() == 0 {
		// File was just created or empty -> we are adding a pristine superblock to it
		sb := &Superblock{Magic: SUPER_MAGIC, Version: SUPER_VERSION, ChunkSize: c.chunkSize, ChunkCount: c.chunkCount}
		binary.Write(fh, binary.LittleEndian, sb)
		fh.Truncate(expectedDbSize)
	} else if stat.Size() == expectedDbSize {
		// Size looks sane, try to read superblock
		sb := Superblock{}
		binary.Read(fh, binary.LittleEndian, &sb)
		if sb.Magic != SUPER_MAGIC || sb.Version != SUPER_VERSION || sb.ChunkSize != c.chunkSize || sb.ChunkCount != c.chunkCount {
			fh.Close()
			return ErrCorruptedDb
		}
	} else {
		// File existed but size was wrong: return an error
		fh.Close()
		return ErrCorruptedDb
	}

	c.fh = fh

	// Map metadata into memory
	for chunk := uint64(0); chunk < c.chunkCount; chunk++ {
		c.seekToMeta(chunk)
		mEnt := MetaEntry{}
		binary.Read(c.fh, binary.LittleEndian, &mEnt)

		mEnt.Key = c.findFreeKey(mEnt.Key) // assigns a random key if this key already exists (causing a checksum fail)
		c.chunkMap[mEnt.Key] = chunkmapEntry{chunk: chunk, dirty: true}
		c.metaMap[chunk] = mEnt
	}

	return nil
}

// Returns a free key in chunkMap, tries to
// return 'hint'
func (c *Cache) findFreeKey(hint uint64) uint64 {
	for {
		if _, exists := c.chunkMap[hint]; exists == false {
			break
		}
		hint = uint64(rand.Int63())
	}
	return hint
}
