// Package apps inventories installed packages, removes them, and installs
// apps from local files using whatever the host OS already provides. Every
// listing is one shell-out to a native manager; nothing is cached twice.
package apps

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Package is one installed app as reported by its manager.
// SizeKB is the manager's best guess; 0 means unknown.
type Package struct {
	Manager string `json:"manager"` // apt | dnf | rpm | pacman | snap | flatpak | appimage | brew | winget | npm
	Name    string `json:"name"`
	Version string `json:"version"`
	SizeKB  int64  `json:"size_kb"`
	Desc    string `json:"desc"`
	Path    string `json:"path,omitempty"` // AppImage/npm: the file/dir itself
}

// Load returns every installed app the system can report: native package
// managers, npm globals, and loose AppImage files.
func Load() ([]Package, error) {
	sys, sysErr := loadManagers()
	out := append(sys, loadAppImages()...)
	out = append(out, loadNPM()...)
	sortByName(out)
	if len(out) == 0 && sysErr != nil {
		return nil, sysErr
	}
	return out, nil
}

// Uninstall removes one package through its manager, elevating if needed.
// purge=true also deletes the app's config/data where the manager supports it.
func Uninstall(p Package, purge bool) error { return uninstall(p, purge) }

// uninstall dispatches on where the app came from: AppImages are files we
// can delete outright, npm globals have their own CLI, everything else goes
// to the host's package manager.
func uninstall(p Package, purge bool) error {
	switch p.Manager {
	case "appimage":
		return removeAppImage(p, purge)
	case "npm":
		return removeNPM(p, purge)
	}
	return uninstallOS(p, purge)
}

// FileList describes the files and folders an installed app owns.
type FileList struct {
	Paths     []string `json:"paths"` // top-level entries, existing only
	Files     int      `json:"files"` // recursive regular-file count
	Dirs      int      `json:"dirs"`
	Bytes     uint64   `json:"bytes"`
	Truncated bool     `json:"truncated"`
}

// Files reports what an installed app put on disk.
func Files(p Package) (*FileList, error) { return files(p) }

// Install takes a local installer/archive path and returns a human
// description of what was done.
func Install(path string) (string, error) { return installFile(path) }

// DropPageCache flushes and releases the kernel's page cache so free-RAM
// numbers reflect what is actually resident. Root only, where it exists.
func DropPageCache() error { return dropPageCache() }

// StateStamp is a cheap fingerprint of the places an install actually touches:
// package databases, desktop folders, the AppImage staging dir. It changes when
// someone installs with a command vibe-check never ran, which is how the apps
// list can stay current without a shell-out every second. A handful of stats,
// safe on a timer.
func StateStamp() int64 {
	var h int64
	for _, p := range statePaths() {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		h = h*31 + fi.ModTime().Unix() + fi.Size()
	}
	return h
}

// statePaths is per-OS, declared in apps_<os>.go.

func Available() []string {
	var out []string
	for _, c := range managerBins() {
		if hasBin(c) {
			out = append(out, c)
		}
	}
	return out
}

// managerBins is per-OS, declared in apps_<os>.go.

func hasBin(name string) bool { _, err := exec.LookPath(name); return err == nil }

// expand resolves "~/x" and relative paths so managers receive absolute ones.
func expand(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if h := homeShort(); h != "" {
			path = filepath.Join(h, strings.TrimPrefix(path, "~"))
		}
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func runOut(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		if s == "" {
			s = err.Error()
		}
		return "", &CmdError{Name: name, Msg: clipLine(lastNonEmpty(s))}
	}
	return s, nil
}

// runStatus is runOut for callers that have to tell a user's Cancel apart from
// a broken tool, which is only visible in the exit code.
func runStatus(name string, args ...string) (string, int, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		code = -1
	}
	return strings.TrimSpace(string(out)), code, err
}

// ErrPickCancelled means the chooser opened and the user closed it without
// picking. That is a decision, not a fault, and the UI says so plainly.
var ErrPickCancelled = errors.New("no file chosen")

// PickFile opens the desktop's own file chooser and returns what was picked,
// so "install from file" needs no path from memory and no flags to type.
func PickFile() (string, error) {
	p, err := pickFile()
	if err != nil {
		return "", err
	}
	if p = strings.TrimSpace(p); p == "" {
		return "", ErrPickCancelled
	}
	return p, nil
}

// PickerAvailable reports whether a chooser could actually appear here.
// vibe-check spends most of its life in a terminal reached over SSH, in a
// container, or on a bare VT, where no desktop exists to draw one: there the
// install flow keeps the typed-path dialog instead of waiting on a window
// nobody will ever see.
func PickerAvailable() bool { return pickerAvailable() }

type CmdError struct{ Name, Msg string }

func (e *CmdError) Error() string { return e.Name + ": " + e.Msg }

func lastNonEmpty(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return s
}

func clipLine(s string) string {
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

// elev returns a privilege wrapper for commands that need admin: none when
// already root, pkexec (graphical prompt) when available, else sudo -n so a
// missing password fails fast instead of hanging on an invisible prompt.
func elev() []string {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return nil
	}
	if hasBin("pkexec") {
		return []string{"pkexec"}
	}
	return []string{"sudo", "-n"}
}

// canElevate reports whether a privileged command has anywhere to go. Without
// polkit or sudo the alternative is a confusing "exec: sudo: not found".
func canElevate() bool {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return true
	}
	return hasBin("pkexec") || hasBin("sudo")
}

// elevate runs name+args, prefixed with the privilege wrapper.
func elevate(name string, args ...string) (string, error) {
	if !canElevate() {
		return "", &CmdError{Name: name, Msg: "administrator access unavailable (no sudo/pkexec)"}
	}
	full := append(elev(), name)
	full = append(full, args...)
	return runOut(full[0], full[1:]...)
}

// extOf returns the install dispatch key for a path: .deb, .rpm, .appimage,
// .tar.gz, .zip, … (lowercase, compound archive suffixes kept whole).
// describe turns an (ok message, error) pair into the function result.
func describe(ok string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	return ok, nil
}

func extOf(path string) string {
	l := strings.ToLower(filepath.Base(path))
	for _, comp := range []string{".tar.gz", ".tar.xz", ".tar.bz2", ".tar.zst"} {
		if strings.HasSuffix(l, comp) {
			return comp
		}
	}
	if e := filepath.Ext(l); e != "" {
		return e
	}
	return ""
}

// appsDir is where vibe-check stages portable installs (AppImages, extracted
// archives). ~/Apps exists as a convention; create it on demand.
func appsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(home, "Apps")
	return d, os.MkdirAll(d, 0o755)
}

// stem strips archive suffixes for directory naming: foo-1.2.tar.xz → foo-1.2.
func stem(path string) string {
	b := filepath.Base(path)
	for _, e := range []string{".tar.gz", ".tar.xz", ".tar.bz2", ".tar.zst", ".AppImage", ".appimage", ".zip", ".deb", ".rpm", ".7z"} {
		b = strings.TrimSuffix(b, e)
	}
	b = strings.TrimSuffix(b, ".tar")
	if b == "" {
		b = "app"
	}
	return b
}

// extractArchive unpacks tar-family (and zip on hosts with a native tool)
// archives into appsDir()/stem and describes the first executable found.
func extractArchive(path string) (string, error) {
	dst, err := appsDir()
	if err != nil {
		return "", err
	}
	dst = filepath.Join(dst, stem(path))
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "windows":
		if extOf(path) == ".zip" {
			if err := extractZipSafe(path, dst); err != nil {
				return "", err
			}
			break
		}
		_, err = runOut("tar", "-xf", path, "-C", dst)
	default:
		_, err = runOut("tar", "-xf", path, "-C", dst)
	}
	if err != nil {
		return "", err
	}
	bin := findExecutable(dst)
	where := strings.TrimPrefix(dst, homeShort()) + "/"
	if bin != "" {
		return "extracted → " + where + " · start with " +
			strings.TrimPrefix(bin, homeShort()) + "/", nil
	}
	return "extracted → " + where, nil
}

// extractZipSafe unzips with archive/zip, rejecting any entry that resolves
// outside dst (zip-slip).
// isUnsafeEntry rejects the zip-slip classics: absolute paths and any entry
// that starts above the destination. Mid-path ".." is fine — the prefix
// check after Join catches anything it would escape with.
func isUnsafeEntry(name string) bool {
	return filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../")
}

func extractZipSafe(src, dst string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	cleanDst := filepath.Clean(dst) + string(filepath.Separator)
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if isUnsafeEntry(name) {
			continue
		}
		target := filepath.Join(filepath.Clean(dst), filepath.FromSlash(name))
		if !strings.HasPrefix(target+string(filepath.Separator), cleanDst) {
			continue
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(w, rc)
		rc.Close()
		w.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func homeShort() string {
	h, _ := os.UserHomeDir()
	return h
}

// findExecutable returns the first world-executable file: a bin/ or usr/bin
// entry if present, else any executable near the top of the tree.
func findExecutable(root string) string {
	for _, sub := range []string{filepath.Join(root, "bin"), filepath.Join(root, "usr", "bin")} {
		if e := firstExecIn(sub); e != "" {
			return e
		}
	}
	return firstExecIn(root)
}

func firstExecIn(dir string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		if fi, err := e.Info(); err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0o111 != 0 {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// parseSizeKB reads manager size strings like "84.1MB", "50 MB", "1234" (KB).
func parseSizeKB(s string) int64 {
	s = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, " ", "")))
	s = strings.TrimRight(s, "b")
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "gi"), strings.HasSuffix(s, "g"):
		mult = 1024 * 1024
	case strings.HasSuffix(s, "mi"), strings.HasSuffix(s, "m"):
		mult = 1024
	case strings.HasSuffix(s, "ki"), strings.HasSuffix(s, "k"):
		mult = 1
	case s != "" && (s[len(s)-1] < '0' || s[len(s)-1] > '9'):
		return 0 // unknown unit suffix: refuse to guess
	}
	s = strings.TrimRight(s, "kmigtbi")
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f < 0 {
		return 0
	}
	return int64(f * float64(mult))
}
