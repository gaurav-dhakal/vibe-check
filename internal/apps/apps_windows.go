//go:build windows

package apps

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var managerBins = func() []string { return []string{"winget"} }

// winget prints a fixed-width table (Name, Id, Version, Source); columns are
// separated by runs of 2+ spaces.
var twoPlus = regexp.MustCompile(`\s{2,}`)

func loadManagers() ([]Package, error) {
	if !hasBin("winget") {
		return nil, fmt.Errorf("winget not found — install App Installer from the Store")
	}
	s, err := runOut("winget", "list", "--accept-source-agreements")
	if err != nil {
		return nil, fmt.Errorf("winget: %w", err)
	}
	var out []Package
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "Name ") {
			continue
		}
		f := twoPlus.Split(ln, -1)
		if len(f) < 4 || f[1] == "Id" {
			continue
		}
		out = append(out, Package{
			Manager: "winget", Name: f[1], Version: f[2],
			Desc: f[0] + " (" + f[3] + ")",
		})
	}
	return out, nil
}

// uninstallOS hands the removal to winget. purge=true asks for a silent
// uninstall so the app cannot leave an interactively-owned trash behind;
// winget has no file-level purge, so the app's data is left in place.
func uninstallOS(p Package, purge bool) error {
	args := []string{"uninstall", "--id", p.Name, "-e",
		"--accept-source-agreements", "--accept-package-agreements"}
	if purge {
		args = append(args, "--silent")
	}
	_, err := runOut("winget", args...)
	return err
}

func installFile(path string) (string, error) {
	path = expand(path)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("file not found: %s", path)
	}
	switch extOf(path) {
	case ".msi":
		// winget installs local manifests and surfaces the UAC prompt itself.
		_, err := runOut("winget", "install", "--manifest", path,
			"--accept-source-agreements", "--accept-package-agreements")
		if err == nil {
			return "msi installed", nil
		}
		_, err2 := elevate("msiexec", "/i", path, "/qn")
		if err2 != nil {
			return "", fmt.Errorf("%v / %v", err, err2)
		}
		return "msi installed (msiexec)", nil
	case ".exe":
		_, err := runOut("cmd", "/c", "start", "", path)
		return describe("installer launched — finish it in the window", err)
	case ".zip":
		dst := filepath.Join(os.TempDir(), "vibe-check-install-"+stem(path))
		if err := extractZipSafe(path, dst); err != nil {
			return "", err
		}
		return "extracted → " + dst, nil
	case ".tar.gz", ".tar.xz", ".tar.bz2", ".tar":
		return extractArchive(path)
	}
	return "", fmt.Errorf("unsupported file type (%s) — try .msi, .exe or .zip", extOf(path))
}

// dropPageCache has no Windows equivalent worth offering: the cache manager
// trims its own working set, and the documented manual flush is a kernel
// debugger command, not something a monitor should be reaching for.
func dropPageCache() error {
	return fmt.Errorf("Windows reclaims its cache automatically — there is nothing to flush here")
}

// pickFile shows the Win32 open dialog. Go cannot reach it without cgo, but
// the shell that ships with every Windows install can, and that dialog is the
// one users already know.
func pickFile() (string, error) {
	const script = `Add-Type -AssemblyName System.Windows.Forms;` +
		`$o = New-Object System.Windows.Forms.OpenFileDialog;` +
		`$o.Title = 'vibe-check - install from file';` +
		`$o.CheckFileExists = $true;` +
		`if ($o.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {` +
		`[Console]::Out.Write($o.FileName) }`
	bin := "powershell"
	args := []string{"-NoProfile", "-Command", script}
	if !hasBin(bin) {
		bin = "pwsh"
		args = []string{"-NoProfile", "-UseWindowsForms", "-Command", script}
	}
	if !hasBin(bin) {
		return "", &CmdError{Name: "picker", Msg: "no PowerShell available to open the file dialog"}
	}
	s, _, err := runStatus(bin, args...)
	switch {
	case err != nil:
		return "", &CmdError{Name: bin, Msg: clipLine(lastNonEmpty(s))}
	case strings.TrimSpace(s) == "":
		return "", ErrPickCancelled // the dialog closed without OK
	}
	return s, nil
}

func pickerAvailable() bool { return hasBin("powershell") || hasBin("pwsh") }

// statePaths: winget keeps its own state under LOCALAPPDATA and every machine
// install touches Program Files. Registry reads are the accurate source but
// cost a shell-out, so the watch compares folder mtimes instead and the exact
// list still comes from `apps list` on demand.
func statePaths() []string {
	out := []string{
		os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WinGet"),
		filepath.Join(os.Getenv("ProgramData"), "Package Cache"),
		filepath.Join(os.Getenv("APPDATA"), "npm"),
	}
	if h := homeShort(); h != "" {
		out = append(out, filepath.Join(h, "Apps"), filepath.Join(h, "scoop/apps"))
	}
	return out
}
