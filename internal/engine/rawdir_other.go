//go:build !linux

package engine

import (
	"os"
	"path/filepath"
)

// dirBuf is unused off Linux; os.ReadDir does its own buffering.
func (b *dirBuf) get() []byte { return nil }

func scanDir(dir string, b *dirBuf, visit func(d *dentry) bool) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	ents, err := f.ReadDir(-1)
	if err != nil {
		return err
	}
	d := dentry{dir: f}
	for _, ent := range ents {
		d.Name, d.isDir = []byte(ent.Name()), ent.IsDir()
		if !visit(&d) {
			return nil
		}
	}
	return nil
}

func (d *dentry) stat() bool {
	info, err := os.Lstat(filepath.Join(d.dir.Name(), string(d.Name)))
	if err != nil {
		return false
	}
	d.size = info.Size()
	d.linkKey, d.nlink = linkInfo(info)
	return true
}
