//go:build unix

package engine

import (
	"io/fs"
	"syscall"
)

// linkInfo fingerprints an inode so a size sweep can count a hard-linked file
// once, the way du does.
func linkInfo(info fs.FileInfo) (uint64, uint64) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Ino ^ uint64(st.Dev)<<32, uint64(st.Nlink)
	}
	return 0, 1
}

// canFingerprintLinks: POSIX stat(2) hands us ino+nlink, so a size sweep can
// match du exactly.
const canFingerprintLinks = true
