package engine

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestScanDirParity pins the raw getdents walk against os.ReadDir: same names,
// same directory flags. The optimized path has to agree with the standard one
// entry for entry, or the scan reports the wrong reclaimable bytes.
func TestScanDirParity(t *testing.T) {
	root := t.TempDir()
	buildTree(t, root)

	var gotNames []string
	var gotDirs []string
	var buf dirBuf
	err := scanDir(root, &buf, func(d *dentry) bool {
		n := string(d.Name)
		gotNames = append(gotNames, n)
		if d.isDir {
			gotDirs = append(gotDirs, n)
		}
		return true
	})
	if err != nil {
		t.Fatalf("scanDir: %v", err)
	}

	ents, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var wantNames, wantDirs []string
	for _, e := range ents {
		wantNames = append(wantNames, e.Name())
		if e.IsDir() {
			wantDirs = append(wantDirs, e.Name())
		}
	}
	sort.Strings(gotNames)
	sort.Strings(wantNames)
	sort.Strings(gotDirs)
	sort.Strings(wantDirs)
	if diff(gotNames, wantNames) {
		t.Errorf("names differ:\n got %v\nwant %v", gotNames, wantNames)
	}
	if diff(gotDirs, wantDirs) {
		t.Errorf("dirs differ:\n got %v\nwant %v", gotDirs, wantDirs)
	}
}

// TestScanDirMultiChunk covers a directory large enough to need several
// getdents reads: record offsets are relative to the chunk, and getting that
// wrong only shows up past the first 64 KiB.
func TestScanDirMultiChunk(t *testing.T) {
	root := t.TempDir()
	want := 0
	for i := 0; i < 900; i++ {
		name := strings.Repeat("x", 120) + "_" + strconv.Itoa(i) + ".js"
		if err := os.WriteFile(filepath.Join(root, name), []byte("z"), 0o644); err != nil {
			t.Fatal(err)
		}
		want++
	}

	var n int
	var buf dirBuf
	if err := scanDir(root, &buf, func(d *dentry) bool {
		n++
		if len(d.Name) == 0 || !strings.Contains(string(d.Name), "_") {
			t.Fatalf("bad entry name %q", d.Name)
		}
		return true
	}); err != nil {
		t.Fatalf("scanDir: %v", err)
	}
	if n != want {
		t.Errorf("scanDir saw %d entries, want %d", n, want)
	}
}

// TestMeasureDirWalkDir does the same for the size sweep: every regular entry
// counted once, symlinked directories never descended into.
func TestMeasureDirWalkDir(t *testing.T) {
	root := t.TempDir()
	buildTree(t, root)

	var wantSize uint64
	var wantFiles int
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == root {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return nil
		}
		wantSize += uint64(info.Size())
		wantFiles++
		return nil
	})
	if walkErr != nil {
		t.Fatalf("WalkDir: %v", walkErr)
	}

	gotSize, gotFiles := measureDir(context.Background(), root, nil)
	if gotSize != wantSize || gotFiles != wantFiles {
		t.Errorf("measureDir = %d bytes/%d files, want %d/%d",
			gotSize, gotFiles, wantSize, wantFiles)
	}
}

// TestPacerDutyCycle checks the limiter really throttles: 200 turns of ~0 cost
// must take far longer than the work itself, and must stop on cancel.
func TestPacerDutyCycle(t *testing.T) {
	p := newPacer(0.5)
	ctx := context.Background()
	for i := 0; i < 200; i++ {
		if !p.charge(ctx, 0) {
			t.Fatal("charge aborted with live context")
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if p.charge(canceled, 0) {
		t.Error("charge returned true after cancel")
	}
}

// TestMeasureDirHardlinks pins the reclaimable figure to what a purge really
// frees: one 4 KiB file linked under two names counts once, matching du.
func TestMeasureDirHardlinks(t *testing.T) {
	root := t.TempDir()
	body := strings.Repeat("z", 4096)
	if err := os.WriteFile(filepath.Join(root, "a"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(root, "a"), filepath.Join(root, "b")); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}
	if !canFingerprintLinks {
		t.Skip("platform exposes no inode fingerprint, so links cannot be deduped here")
	}
	size, files := measureDir(context.Background(), root, nil)
	if want := uint64(len(body)); size != want || files != 1 {
		t.Errorf("measureDir = %d bytes/%d files, want %d/1", size, files, want)
	}
}

// buildTree makes a directory with the shapes that break naive crawls: names
// with spaces and unicode, empty dirs, deep nesting, symlinks to a dir, a file
// and nowhere.
func buildTree(t *testing.T, root string) {
	t.Helper()
	mk := func(rel string, mode os.FileMode, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	mk("plain.txt", 0o644, "1234567890")
	mk("with space and ünïcode.txt", 0o644, "abcd")
	mk("node_modules/pkg-a/index.js", 0o644, "abcdef")
	mk("node_modules/pkg-a/sub/deep.go", 0o644, "xyz")
	mk("src/.hidden", 0o644, "q")
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{
		"link-to-dir":    "node_modules",
		"link-to-file":   "plain.txt",
		"link-broken":    "does-not-exist",
		"loop-a":         "loop-b",
		"loop-b":         "loop-a",
		"abs-self":       root,
		"root-symlink":   "..",
		"weird name.lnk": "with space and ünïcode.txt",
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
}

func diff(a, b []string) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}
