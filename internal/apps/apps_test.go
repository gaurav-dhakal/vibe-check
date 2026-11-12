package apps

import (
	"runtime"
	"strings"
	"testing"
)

func TestExtOf(t *testing.T) {
	cases := map[string]string{
		"/tmp/foo_1.0.deb":         ".deb",
		"/tmp/Foo-x86_64.AppImage": ".appimage",
		"a.tar.gz":                 ".tar.gz",
		"a.TAR.XZ":                 ".tar.xz",
		"plain":                    "",
	}
	for in, want := range cases {
		if got := extOf(in); got != want {
			t.Errorf("extOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSizeKB(t *testing.T) {
	cases := map[string]int64{
		"84.1MB": 86118, // int64(84.1*1024)
		"50 MB":  50 * 1024,
		"2 GiB":  2 * 1024 * 1024,
		"1234":   1234, // plain KB
		"1.5G":   1572864,
		"":       0,
		"abc":    0,
		"-8MB":   0,
		"8 quid": 0,
	}
	for in, want := range cases {
		if got := parseSizeKB(in); got != want {
			t.Errorf("parseSizeKB(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestStem(t *testing.T) {
	cases := map[string]string{
		"/x/foo-1.2.tar.gz": "foo-1.2",
		"Bar.AppImage":      "Bar",
		"a.deb":             "a",
		".tar.xz":           "app",
	}
	for in, want := range cases {
		if got := stem(in); got != want {
			t.Errorf("stem(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractZipSafeRejectsTraversal(t *testing.T) {
	// build a tiny zip containing "../evil" via archive/zip is verbose;
	// instead verify the guard helper directly on names.
	for _, bad := range []string{"../evil", "/abs/evil"} {
		if !isUnsafeEntry(bad) {
			t.Errorf("%q should be rejected", bad)
		}
	}
	for _, good := range []string{"app/bin", "readme.txt", "a/../b/c"} {
		if isUnsafeEntry(good) {
			t.Errorf("%q should be allowed", good)
		}
	}
}

func TestParseAppImageName(t *testing.T) {
	cases := []struct{ file, name, ver string }{
		{"Some-App-1.2.3-x86_64.AppImage", "Some-App", "1.2.3"},
		{"cursor.AppImage", "cursor", ""},
		{"VSCodium-1.85.2.AppImage", "VSCodium", "1.85.2"},
		{"Sublime.Merge.AppImage", "Sublime.Merge", ""},
		{"Foo_v2.0_i386.appimage", "Foo", "2.0"},
	}
	for _, c := range cases {
		name, ver := parseAppImageName(c.file)
		if name != c.name || ver != c.ver {
			t.Errorf("parseAppImageName(%q) = %q,%q want %q,%q", c.file, name, ver, c.name, c.ver)
		}
	}
}

// The dispatcher must never shell out for a manager this host does not have,
// and must refuse unknown ones instead of guessing at a command line.
func TestUninstallUnknownManager(t *testing.T) {
	if err := Uninstall(Package{Manager: "packagefinder", Name: "x"}, false); err == nil ||
		!strings.Contains(err.Error(), "packagefinder") {
		t.Errorf("unknown manager: got %v", err)
	}
}

func TestFilesUnknownManager(t *testing.T) {
	if _, err := Files(Package{Manager: "packagefinder", Name: "x"}); err == nil {
		t.Error("expected an error for an unlistable manager")
	}
}

func TestRemoveAppImageMissingFile(t *testing.T) {
	err := Uninstall(Package{Manager: "appimage", Name: "ghost", Path: "/nonexistent/ghost.AppImage"}, false)
	if err == nil || !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("expected the path in the error, got %v", err)
	}
}

func TestAppImageDirsAreAbsolute(t *testing.T) {
	if runtime.GOOS == "" {
		t.Skip()
	}
	for _, d := range appImageDirs() {
		if !strings.HasPrefix(d, "/") {
			t.Errorf("appImageDirs returned relative path %q", d)
		}
	}
}
