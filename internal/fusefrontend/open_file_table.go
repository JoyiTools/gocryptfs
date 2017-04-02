package fusefrontend

import (
	"sync"
	"sync/atomic"
	"syscall"
)

// DevIno uniquely identifies a backing file through device number and
// inode number.
type DevIno struct {
	dev uint64
	ino uint64
}

// DevInoFromStat fills a new DevIno with the passed Stat_t info
func DevInoFromStat(st *syscall.Stat_t) DevIno {
	// Explicit cast to uint64 to prevent build problems on 32-bit platforms
	return DevIno{
		dev: uint64(st.Dev),
		ino: uint64(st.Ino),
	}
}

// openFileTable - usage:
// 1) register
// 2) lock ... unlock ...
// 3) unregister
type openFileTable struct {
	// opCount counts writeLock.Lock() calls. As every operation that modifies a file should
	// call it, this effectively serves as a write-operation counter.
	// The variable is accessed without holding any locks so atomic operations
	// must be used. It must be the first element of the struct to guarantee
	// 64-bit alignment.
	opCount uint64
	// Protects map access and refCount writes to entries
	sync.Mutex
	// Actual table entries
	entries map[DevIno]*oftEntry
}

// opCountMutex is a Mutex that increments the target of its opCount pointer for
// each Lock() operation
type opCountMutex struct {
	sync.Mutex
	opCount *uint64
}

func (ocm *opCountMutex) Lock() {
	ocm.Mutex.Lock()
	atomic.AddUint64(ocm.opCount, 1)
}

// oftEntry - Open File Table Entry
type oftEntry struct {
	// refCount = Reference count (a file may be open multiple times)
	// Protected by the openFileTable mutex.
	refCount int
	// Write lock for this inode
	writeLock *opCountMutex
	// ID is the file ID in the file header.
	ID     []byte
	IDLock sync.RWMutex
}

// register creates an entry for "di", or incrementes the reference count
// if the entry already exists.
func (oft *openFileTable) register(di DevIno) *oftEntry {
	oft.Lock()
	defer oft.Unlock()

	r := oft.entries[di]
	if r == nil {
		o := opCountMutex{opCount: &oft.opCount}
		r = &oftEntry{writeLock: &o}
		oft.entries[di] = r
	}
	r.refCount++
	return r
}

// unregister decrements the reference count for "di" and deletes the entry if
// the reference count has reached 0.
func (oft *openFileTable) unregister(di DevIno) {
	oft.Lock()
	defer oft.Unlock()

	r := oft.entries[di]
	r.refCount--
	if r.refCount == 0 {
		delete(oft.entries, di)
	}
}
