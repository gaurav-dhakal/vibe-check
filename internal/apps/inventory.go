package apps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// loadManagers is per-OS (apt/rpm/pacman/snap/flatpak, brew, winget).
// AppImages and npm globals live outside any manager's view, so the shared
// inventory scans them on every platform.

func sortByName(pkgs []Package) {
	sort.Slice(pkgs, func(i, j int) bool {
		if pkgs[i].Name != pkgs[j].Name {
			return pkgs[i].Name < pkgs[j].Name
		}
		return pkgs[i].Manager < pkgs[j].Manager
	})
}

var appImageRe = regexp.MustCompile(`(?i)\.appimage$`)

// appImageDirs are the places people park AppImages: our own staging dir,
// the home folders that exist for exactly this, and the usual bin paths.
func appImageDirs() []string {
	home := homeShort()
	if home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, "Apps"),
		filepath.Join(home, "Applications"),
		filepath.Join(home, "Downloads"),
		filepath.Join(home, ".local", "bin"),
		"/usr/local/bin",
		"/opt",
	}
}

// loadAppImages lists loose .AppImage files as first-class apps, so they
// appear next to manager-installed packages and can be removed from here.
func loadAppImages() []Package {
	var out []Package
	seen := map[string]bool{}
	var scan func(dir string, depth int)
	scan = func(dir string, depth int) {
		ents, err := os.ReadDir(dir)
		if err != nil || depth > 1 {
			return
		}
		for _, e := range ents {
			if e.IsDir() {
				if depth < 1 {
					scan(filepath.Join(dir, e.Name()), depth+1)
				}
				continue
			}
			p := filepath.Join(dir, e.Name())
			if !appImageRe.MatchString(e.Name()) {
				continue
			}
			fi, err := e.Info()
			if err != nil || fi.Mode()&0o111 == 0 {
				continue // not executable: a download sitting there, not an app
			}
			if seen[p] {
				continue
			}
			seen[p] = true
			name, ver := parseAppImageName(e.Name())
			out = append(out, Package{
				Manager: "appimage", Name: name, Version: ver,
				SizeKB: fi.Size() / 1024, Path: p,
				Desc: strings.TrimPrefix(p, homeShort()+"/"),
			})
		}
	}
	for _, d := range appImageDirs() {
		scan(d, 0)
	}
	return out
}

var appImageVerRe = regexp.MustCompile(`[-_ ]v?(\d[0-9A-Za-z.]*(?:-\d+)?)`)

// parseAppImageName splits "Some-App-1.2.3-x86_64.AppImage" into name+version.
func parseAppImageName(file string) (string, string) {
	base := strings.TrimSuffix(strings.TrimSuffix(file, ".AppImage"), ".appimage")
	if loc := appImageVerRe.FindStringSubmatchIndex(base); loc != nil {
		return strings.Trim(base[:loc[0]], "-_ "), base[loc[2]:loc[3]]
	}
	return base, ""
}

// loadNPM lists global npm packages — the dev-machine apps managers miss.
func loadNPM() []Package {
	if !hasBin("npm") {
		return nil
	}
	s, err := runOut("npm", "ls", "-gp", "--depth=0")
	if err != nil && s == "" {
		return nil
	}
	var out []Package
	for _, line := range strings.Split(s, "\n") {
		p := strings.TrimSpace(line)
		if p == "" || !strings.HasPrefix(p, "/") { // npm prints warnings too
			continue
		}
		pj, err := os.ReadFile(filepath.Join(p, "package.json"))
		if err != nil {
			continue
		}
		var meta struct {
			Name        string `json:"name"`
			Version     string `json:"version"`
			Description string `json:"description"`
		}
		if json.Unmarshal(pj, &meta) != nil || meta.Name == "" {
			continue
		}
		if meta.Name == "npm" { // npm's own package dir ships in the global root
			continue
		}
		out = append(out, Package{
			Manager: "npm", Name: meta.Name, Version: meta.Version,
			SizeKB: dirSizeKB(p), Desc: clipLine(meta.Description), Path: p,
		})
	}
	return out
}

// dirSizeKB is a capped recursive stat pass — good enough for a table cell,
// never allowed to stall the inventory on a huge tree.
func dirSizeKB(dir string) int64 {
	var total int64
	var n int
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if n++; n > 20000 {
			return filepath.SkipAll
		}
		if fi, e := d.Info(); e == nil {
			total += fi.Size()
		}
		return nil
	})
	return total / 1024
}

// removeAppImage deletes the file plus any desktop entry pointing at it.
func removeAppImage(p Package, purge bool) error {
	if err := os.Remove(p.Path); err != nil {
		return err
	}
	if !purge {
		return nil // keep the tray entry so relaunching still works after a copy back
	}
	home := homeShort()
	ents, _ := os.ReadDir(filepath.Join(home, ".local", "share", "applications"))
	for _, e := range ents {
		data, err := os.ReadFile(filepath.Join(home, ".local", "share", "applications", e.Name()))
		if err == nil && strings.Contains(string(data), "Exec="+p.Path) {
			_ = os.Remove(filepath.Join(home, ".local", "share", "applications", e.Name()))
		}
	}
	return nil
}

func removeNPM(p Package, _ bool) error {
	if _, err := runOut("npm", "uninstall", "-g", p.Name); err != nil {
		// global root may be system-owned
		_, err2 := elevate("npm", "uninstall", "-g", p.Name)
		return err2
	}
	return nil
}

// files dispatches the "what did this app install" question per manager.
func files(p Package) (*FileList, error) {
	var paths []string
	switch p.Manager {
	case "appimage", "npm":
		paths = []string{p.Path}
	case "apt":
		s, err := runOut("dpkg", "-L", p.Name)
		if err != nil {
			return nil, err
		}
		paths = strings.Split(s, "\n")
	case "dnf", "yum", "rpm":
		s, err := runOut("rpm", "-ql", p.Name)
		if err != nil {
			return nil, err
		}
		paths = strings.Split(s, "\n")
	case "pacman":
		s, err := runOut("pacman", "-Ql", p.Name)
		if err != nil {
			return nil, err
		}
		for _, ln := range strings.Split(s, "\n") {
			if f := strings.Fields(ln); len(f) >= 2 {
				paths = append(paths, f[1])
			}
		}
	case "snap":
		paths = []string{"/snap/" + p.Name}
	case "flatpak":
		// The listing column is the app ID; runtimes and extensions live
		// under runtime/, apps under app/, and user data under .var/app.
		home := homeShort()
		inst := p.Name
		if i := strings.LastIndex(inst, "."); i > 0 {
			inst = inst[i+1:]
		}
		roots := []string{"/var/lib/flatpak", filepath.Join(home, ".local", "share", "flatpak")}
		for _, r := range roots {
			paths = append(paths, r+"/app/"+p.Name, r+"/runtime/"+p.Name)
		}
		paths = append(paths, filepath.Join(home, ".var", "app", inst))
	case "brew":
		s, err := runOut("brew", "list", "--verbose", p.Name)
		if err != nil {
			return nil, err
		}
		paths = strings.Split(s, "\n")
	case "brew-cask":
		// A cask's receipt lists no files; the Caskroom staging dir and the
		// app it dropped in /Applications are the honest answer.
		prefix, err := runOut("brew", "--prefix")
		if err != nil {
			return nil, err
		}
		paths = []string{
			strings.TrimSpace(prefix) + "/Caskroom/" + p.Name,
			"/Applications/" + p.Name + ".app",
			homeShort() + "/Applications/" + p.Name + ".app",
		}
	case "winget":
		return nil, errNoFiles
	default:
		return nil, errNoFiles
	}

	fl := &FileList{}
	// Only walk when the listing is the app's private tree (an AppImage, an
	// npm folder). Manager listings name shared directories — recursing into
	// /usr/share would bill the whole filesystem to one package, and take
	// 20 seconds doing it.
	walk := p.Manager == "appimage" || p.Manager == "npm" || p.Manager == "snap" ||
		p.Manager == "flatpak" || p.Manager == "brew-cask"
	for _, raw := range paths {
		pth := strings.TrimSpace(raw)
		if pth == "" || !strings.HasPrefix(pth, "/") {
			continue
		}
		fi, err := os.Stat(pth)
		if err != nil {
			continue // dpkg -L happily lists files another package removed
		}
		fl.Paths = append(fl.Paths, pth)
		if fi.IsDir() {
			fl.Dirs++
			if walk {
				n, bytes := walkStats(pth)
				fl.Files += n
				fl.Bytes += bytes
			}
		} else {
			fl.Files++
			fl.Bytes += uint64(fi.Size())
		}
	}
	sort.Strings(fl.Paths)
	if len(fl.Paths) > 400 {
		fl.Paths = fl.Paths[:400]
		fl.Truncated = true
	}
	if len(fl.Paths) == 0 {
		return nil, errNoFiles
	}
	return fl, nil
}

var errNoFiles = &CmdError{Name: "files", Msg: "no file listing available for this package"}

// walkStats counts regular files under root without following symlinks. It
// gives up on huge trees: the number is a table cell, not an audit.
func walkStats(root string) (int, uint64) {
	var n int
	var total uint64
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if fi, e := d.Info(); e == nil && fi.Mode().IsRegular() {
			n++
			total += uint64(fi.Size())
			if n > walkCap {
				return filepath.SkipAll
			}
		}
		return nil
	})
	return n, total
}

const walkCap = 60000
