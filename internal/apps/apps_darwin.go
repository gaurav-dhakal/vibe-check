//go:build darwin

package apps

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var managerBins = func() []string { return []string{"brew"} }

// brewInfo is the subset of `brew info --json=v2 --installed` we need.
type brewInfo struct {
	Formulae []struct {
		Name     string `json:"name"`
		Versions struct {
			Installed string `json:"installed"`
		} `json:"versions"`
		Desc string `json:"desc"`
	} `json:"formulae"`
	Casks []struct {
		Name     string `json:"name"`
		Versions struct {
			Installed string `json:"installed"`
		} `json:"versions"`
		Desc string `json:"desc"`
	} `json:"casks"`
}

func loadManagers() ([]Package, error) {
	if !hasBin("brew") {
		return nil, fmt.Errorf("homebrew not found — install it or use the installer files")
	}
	s, err := runOut("brew", "info", "--json=v2", "--installed")
	if err != nil {
		return nil, fmt.Errorf("brew: %w", err)
	}
	var bi brewInfo
	if err := json.Unmarshal([]byte(s), &bi); err != nil {
		return nil, fmt.Errorf("brew json: %w", err)
	}
	out := make([]Package, 0, len(bi.Formulae)+len(bi.Casks))
	for _, f := range bi.Formulae {
		out = append(out, Package{Manager: "brew", Name: f.Name, Version: f.Versions.Installed, Desc: f.Desc})
	}
	for _, c := range bi.Casks {
		out = append(out, Package{Manager: "brew-cask", Name: c.Name, Version: c.Versions.Installed, Desc: c.Desc})
	}
	return out, nil
}

// uninstallOS removes a brew package; purge=true also deletes its config and
// leaves no versioned cellar directory behind.
func uninstallOS(p Package, purge bool) error {
	args := []string{"uninstall"}
	if purge {
		args = append(args, "--purge")
	}
	if p.Manager == "brew-cask" {
		args = append(args, "--cask")
		if purge {
			args = append(args, "--zap")
		}
	}
	_, err := runOut("brew", append(args, p.Name)...)
	return err
}

func installFile(path string) (string, error) {
	path = expand(path)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("file not found: %s", path)
	}
	switch extOf(path) {
	case ".pkg":
		_, err := elevate("installer", "-pkg", path, "-target", "Drive")
		return describe("pkg installed system-wide", err)
	case ".dmg":
		return installDMG(path)
	case ".app":
		return installAppDir(path)
	case ".zip":
		tmp := filepath.Join(os.TempDir(), "vibe-check-install-"+stem(path))
		if err := extractZipSafe(path, tmp); err != nil {
			return "", err
		}
		defer os.RemoveAll(tmp)
		for _, cand := range []string{filepath.Join(tmp, stem(path)+".app"), findAppBundle(tmp)} {
			if cand != "" {
				return installAppDir(cand)
			}
		}
		return "", fmt.Errorf("no .app bundle inside the zip")
	case ".tar.gz", ".tar.xz", ".tar.bz2", ".tar":
		return extractArchive(path)
	}
	return "", fmt.Errorf("unsupported file type (%s) — try .pkg, .dmg, .app, .zip or .tar.gz", extOf(path))
}

// installDMG mounts the image, copies the first .app (or runs the first .pkg)
// into /Applications, then detaches.
func installDMG(path string) (string, error) {
	mnt := filepath.Join(os.TempDir(), "vibe-check-dmg-"+stem(path))
	if err := os.MkdirAll(mnt, 0o755); err != nil {
		return "", err
	}
	defer os.Remove(mnt)
	if _, err := runOut("hdiutil", "attach", "-nobrowse", "-quiet", "-mountpoint", mnt, path); err != nil {
		return "", fmt.Errorf("hdiutil attach: %w", err)
	}
	defer exec.Command("hdiutil", "detach", "-quiet", mnt).Run()
	if pkg := firstWithExt(mnt, ".pkg"); pkg != "" {
		_, err := elevate("installer", "-pkg", pkg, "-target", "Drive")
		return describe("pkg from dmg installed", err)
	}
	app := findAppBundle(mnt)
	if app == "" {
		return "", fmt.Errorf("no .app or .pkg inside the dmg")
	}
	return installAppDir(app)
}

func installAppDir(app string) (string, error) {
	dest := filepath.Join("/Applications", filepath.Base(app))
	_, err := elevate("cp", "-R", app, dest)
	if err != nil {
		// fall back to the user-local Applications folder
		home, _ := os.UserHomeDir()
		dir := filepath.Join(home, "Applications")
		os.MkdirAll(dir, 0o755)
		dest = filepath.Join(dir, filepath.Base(app))
		_, err = runOut("cp", "-R", app, dest)
		if err != nil {
			return "", err
		}
	}
	return "installed → " + strings.TrimPrefix(dest, homeShort()) + "/", nil
}

func findAppBundle(root string) string {
	var found string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && strings.HasSuffix(p, ".app") {
			found = p
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func firstWithExt(root, ext string) string {
	ents, _ := os.ReadDir(root)
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ext) {
			return filepath.Join(root, e.Name())
		}
	}
	return ""
}

// dropPageCache runs purge(8), macOS's only manual cache flush. It insists on
// root, so it goes through the same elevation as an uninstall.
func dropPageCache() error {
	if !hasBin("purge") {
		return fmt.Errorf("purge(8) not found")
	}
	_, err := elevate("/usr/sbin/purge")
	return err
}

// pickFile asks Finder for the path, through the one scripting bridge that is
// always installed. Choosing Cancel makes AppleScript exit with -128.
func pickFile() (string, error) {
	s, _, err := runStatus("osascript", "-e",
		`POSIX path of (choose file with prompt "vibe-check · install from file")`)
	switch {
	case err == nil:
		return s, nil
	case strings.Contains(s, "-128") || strings.Contains(s, "User canceled"):
		return "", ErrPickCancelled
	default:
		return "", &CmdError{Name: "osascript", Msg: clipLine(lastNonEmpty(s))}
	}
}

func pickerAvailable() bool { return hasBin("osascript") }

// statePaths: brew writes a JSON install receipt per formula, and every app
// lands in one of four known folders. Their mtimes move on install and on
// uninstall, which is enough to notice a change without reading them.
func statePaths() []string {
	h := homeShort()
	out := []string{
		"/opt/homebrew/Cellar", "/opt/homebrew/Caskroom",
		"/usr/local/Cellar", "/usr/local/Caskroom",
		"/Applications", "/usr/local/lib/node_modules",
	}
	if h != "" {
		out = append(out, h+"/Applications", h+"/.npm-global/lib/node_modules",
			h+"/.nvm/versions/node", h+"/Apps")
	}
	return out
}
