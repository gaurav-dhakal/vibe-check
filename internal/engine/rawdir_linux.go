//go:build linux

package engine

import (
	"bytes"
	"encoding/binary"
	"os"

	"golang.org/x/sys/unix"
)

// getdents64 record: ino(8) off(8) reclen(2) type(1) name(remainder, NUL pad).
const (
	direntHdr = 19 // bytes before the name
	direntMin = 24 // shortest legal record: header + "." + padding
)

func (b *dirBuf) get() []byte {
	if len(b.buf) == 0 {
		b.buf = make([]byte, 64*1024)
	}
	return b.buf
}

// scanDir lists dir through one reusable getdents64 buffer and hands each entry
// to visit. Unlike os.ReadDir it allocates nothing per entry, which is the
// difference between a whole-home scan costing 2% of a core and pinning one:
// the tree holds millions of entries, and every DirEntry is a heap object the
// GC later has to find.
//
// d.Name aliases the shared buffer, so it is only valid during the call; copy
// it when a path has to outlive the directory.
func scanDir(dir string, b *dirBuf, visit func(d *dentry) bool) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := b.get()
	fd := int(f.Fd())
	d := dentry{dir: f}
	for {
		n, err := unix.Getdents(fd, buf)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return err
		}
		if n == 0 {
			return nil
		}
		for off := 0; off+direntMin <= n; {
			reclen := int(binary.LittleEndian.Uint16(buf[off+16 : off+18]))
			if reclen < direntMin || off+reclen > n {
				break // short or truncated tail: end this chunk
			}
			ino := binary.LittleEndian.Uint64(buf[off : off+8])
			typ := buf[off+18]
			name := zap(buf[off+direntHdr : off+reclen])
			off += reclen
			if ino == 0 || len(name) == 0 || (name[0] == '.' && isSelfOrParent(name)) {
				continue
			}
			if typ == unix.DT_UNKNOWN {
				// Filesystem gave no type (some network/overlay mounts): the
				// fd-relative fallback keeps the walk correct, just slower.
				typ = unix.DT_REG
				var st unix.Stat_t
				if e := unix.Fstatat(fd, string(name), &st, unix.AT_SYMLINK_NOFOLLOW); e == nil &&
					st.Mode&unix.S_IFMT == unix.S_IFDIR {
					typ = unix.DT_DIR
				}
			}
			d.Name, d.isDir = name, typ == unix.DT_DIR
			if !visit(&d) {
				return nil
			}
		}
	}
}

// stat resolves the entry fd-relatively, so a size sweep costs one statx and no
// path reconstruction.
func (d *dentry) stat() bool {
	var st unix.Stat_t
	if err := unix.Fstatat(int(d.dir.Fd()), string(d.Name), &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return false
	}
	d.size = st.Size
	d.nlink = uint64(st.Nlink)
	d.linkKey = st.Ino ^ uint64(st.Dev)<<32
	return true
}

// zap trims the NUL padding a dirent name carries.
func zap(b []byte) []byte {
	for i, c := range b {
		if c == 0 {
			return b[:i]
		}
	}
	return b
}

func isSelfOrParent(name []byte) bool {
	return bytes.Equal(name, dotBytes) || bytes.Equal(name, dotdotBytes)
}

var (
	dotBytes    = []byte(".")
	dotdotBytes = []byte("..")
)
