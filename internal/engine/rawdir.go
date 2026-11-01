package engine

import "os"

// dirBuf holds the one read buffer a crawler goroutine reuses for every
// directory it lists.
type dirBuf struct{ buf []byte }

// dentry is a directory entry handed to a scanDir visitor, resolved relative
// to the directory being listed. Name aliases the shared buffer, so it is only
// valid for the duration of the visit call.
type dentry struct {
	dir     *os.File
	Name    []byte
	isDir   bool
	size    int64
	linkKey uint64 // dev+ino fingerprint, 0 when the filesystem hides it
	nlink   uint64 // hard links to that inode
}
