//go:build linux

package apps

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var managerBins = func() []string { return []string{"apt", "dpkg-query", "rpm", "pacman", "snap", "flatpak"} }

func loadManagers() ([]Package, error) {
	var out []Package
	var errs []string
	for _, f := range []func(*[]Package, *[]string){loadApt, loadRPM, loadPacman, loadSnap, loadFlatpak} {
		f(&out, &errs)
	}
	if len(out) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("no package manager responded: %s", strings.Join(errs, "; "))
	}
	return out, nil
}

func loadApt(pkgs *[]Package, errs *[]string) {
	if !hasBin("dpkg-query") {
		return
	}
	s, err := runOut("dpkg-query", "-W",
		"-f=${Package}\t${Version}\t${Installed-Size}\t${Section}\t${Status}\n")
	if err != nil {
		*errs = append(*errs, err.Error())
		return
	}
	for _, ln := range strings.Split(s, "\n") {
		f := strings.SplitN(ln, "\t", 5)
		if len(f) < 5 || f[4] != "install ok installed" {
			continue
		}
		*pkgs = append(*pkgs, Package{
			Manager: "apt", Name: f[0], Version: f[1],
			SizeKB: atoi64(f[2]), Desc: f[3],
		})
	}
}

func loadRPM(pkgs *[]Package, errs *[]string) {
	if hasBin("dpkg-query") || !hasBin("rpm") { // rpm exists on some apt hosts too
		return
	}
	mgr := "rpm"
	if hasBin("dnf") {
		mgr = "dnf"
	} else if hasBin("yum") {
		mgr = "yum"
	}
	s, err := runOut("rpm", "-qa", "--queryformat", "%{NAME}\t%{VERSION}-%{RELEASE}\t%{SIZE}\t%{SUMMARY}\n")
	if err != nil {
		*errs = append(*errs, err.Error())
		return
	}
	for _, ln := range strings.Split(s, "\n") {
		f := strings.SplitN(ln, "\t", 4)
		if len(f) < 3 {
			continue
		}
		*pkgs = append(*pkgs, Package{
			Manager: mgr, Name: f[0], Version: f[1],
			SizeKB: atoi64(f[2]), Desc: strings.Join(f[3:], ""),
		})
	}
}

func loadPacman(pkgs *[]Package, errs *[]string) {
	if !hasBin("pacman") {
		return
	}
	if hasBin("expac") { // one shot with size + description
		s, err := runOut("expac", "-S", "%n\t%v\t%k\t%p")
		if err == nil {
			for _, ln := range strings.Split(s, "\n") {
				f := strings.SplitN(ln, "\t", 4)
				if len(f) < 2 {
					continue
				}
				*pkgs = append(*pkgs, Package{Manager: "pacman", Name: f[0], Version: f[1],
					SizeKB: atoi64(f[2]), Desc: strings.Join(f[3:], " ")})
			}
			return
		}
	}
	s, err := runOut("pacman", "-Q")
	if err != nil {
		*errs = append(*errs, err.Error())
		return
	}
	for _, ln := range strings.Split(s, "\n") {
		if f := strings.Fields(ln); len(f) >= 2 {
			*pkgs = append(*pkgs, Package{Manager: "pacman", Name: f[0], Version: f[1]})
		}
	}
}

func loadSnap(pkgs *[]Package, errs *[]string) {
	if !hasBin("snap") {
		return
	}
	s, err := runOut("snap", "list")
	if err != nil {
		*errs = append(*errs, err.Error())
		return
	}
	for _, ln := range dropHeader(s) {
		f := strings.Fields(ln)
		if len(f) < 6 {
			continue
		}
		// Name Version Rev Publisher Notes Size — size may carry a space ("50 MB")
		*pkgs = append(*pkgs, Package{
			Manager: "snap", Name: f[0], Version: f[1],
			SizeKB: parseSizeKB(strings.Join(f[len(f)-2:], "")),
			Desc:   f[3],
		})
	}
}

func loadFlatpak(pkgs *[]Package, errs *[]string) {
	if !hasBin("flatpak") {
		return
	}
	s, err := runOut("flatpak", "list", "--columns=application,version,size,description")
	if err != nil {
		*errs = append(*errs, err.Error())
		return
	}
	// --columns prints tab-separated rows with no header.
	for _, ln := range strings.Split(strings.TrimSpace(s), "\n") {
		f := strings.Split(ln, "\t")
		if len(f) < 2 {
			continue
		}
		p := Package{Manager: "flatpak", Name: f[0], Version: f[1]}
		if len(f) > 2 {
			p.SizeKB = parseSizeKB(f[2])
		}
		if len(f) > 3 {
			p.Desc = f[3]
		}
		*pkgs = append(*pkgs, p)
	}
}

func dropHeader(s string) []string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 0 {
		return lines[1:]
	}
	return lines
}

func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

// uninstallOS removes a manager-owned package. purge=true also drops configs
// where the manager separates the two (apt); the others already take their
// own dependencies with them.
func uninstallOS(p Package, purge bool) error {
	switch p.Manager {
	case "apt":
		args := []string{"remove", "-y"}
		if purge {
			args = []string{"purge", "-y"}
		}
		_, err := elevate("apt-get", append(args, p.Name)...)
		return err
	case "dnf":
		_, err := elevate("dnf", "remove", "-y", p.Name)
		return err
	case "yum":
		_, err := elevate("yum", "remove", "-y", p.Name)
		return err
	case "rpm":
		_, err := elevate("rpm", "-e", p.Name)
		return err
	case "pacman":
		args := []string{"-Rns", "--noconfirm"}
		if purge {
			args = []string{"-Rnsu", "--noconfirm"}
		}
		_, err := elevate("pacman", append(args, p.Name)...)
		return err
	case "snap":
		args := []string{"remove"}
		if purge {
			args = []string{"remove", "--purge"}
		}
		_, err := elevate("snap", append(args, p.Name)...)
		return err
	case "flatpak":
		args := []string{"uninstall", "-y", "--noninteractive"}
		if purge {
			args = append(args, "--delete-data")
		}
		_, err := runOut("flatpak", append(args, p.Name)...)
		return err
	}
	return fmt.Errorf("no uninstall path for manager %q", p.Manager)
}

func installFile(path string) (string, error) {
	path = expand(path)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("file not found: %s", path)
	}
	switch extOf(path) {
	case ".deb":
		if hasBin("apt-get") { // resolves dependencies, unlike plain dpkg
			_, err := elevate("apt-get", "install", "-y", path)
			return describe("deb package installed", err)
		}
		_, err := elevate("dpkg", "-i", path)
		return describe("deb package installed (dpkg: fix deps with apt -f install)", err)
	case ".rpm":
		for _, m := range []string{"dnf", "yum"} {
			if hasBin(m) {
				_, err := elevate(m, "install", "-y", path)
				return describe("rpm package installed", err)
			}
		}
		_, err := elevate("rpm", "-Uvh", path)
		return describe("rpm package installed", err)
	case ".appimage":
		return installAppImage(path)
	case ".tar.gz", ".tar.xz", ".tar.bz2", ".tar", ".zip":
		return extractArchive(path)
	}
	return "", fmt.Errorf("unsupported file type (%s) — try .deb, .rpm, .tar.gz, .tar.xz or .AppImage", extOf(path))
}

// installAppImage stages the AppImage under ~/Apps and registers a
// .desktop entry so it shows up in the desktop environment's app tray.
func installAppImage(path string) (string, error) {
	dir, err := appsDir()
	if err != nil {
		return "", err
	}
	dest := filepath.Join(dir, filepath.Base(path))
	if abs, _ := filepath.Abs(path); abs == dest { // already staged
		if err := os.Chmod(dest, 0o755); err != nil {
			return "", err
		}
	} else {
		if err := exec.Command("cp", path, dest).Run(); err != nil {
			return "", fmt.Errorf("copy AppImage: %w", err)
		}
		if err := os.Chmod(dest, 0o755); err != nil {
			return "", err
		}
	}
	name := stem(dest)
	desk := filepath.Join(homeShort(), ".local", "share", "applications",
		"vibe-check-"+slug(name)+".desktop")
	if err := os.MkdirAll(filepath.Dir(desk), 0o755); err != nil {
		return "", err
	}
	content := "[Desktop Entry]\nType=Application\nName=" + name +
		"\nComment=vibe-check AppImage install\nExec=" + dest +
		"\nIcon=application-x-executable\nTerminal=false\nCategories=Utility;\n"
	if err := os.WriteFile(desk, []byte(content), 0o644); err != nil {
		return "", err
	}
	if hasBin("update-desktop-database") {
		_, _ = runOut("update-desktop-database", filepath.Dir(desk))
	}
	return "AppImage → " + strings.TrimPrefix(dest, homeShort()) + "/ · added to app tray", nil
}

func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteByte(byte(r))
		case r == '-' || r == '_':
			b.WriteByte(byte(r))
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// dropPageCache empties the kernel's page cache so the free-RAM gauge shows
// what is really needed, not what is merely borrowed. The redirect has to run
// as root, hence the shell; elevation is pkexec/sudo -n, so it asks once or
// fails fast rather than hanging behind the TUI.
func dropPageCache() error {
	if os.Geteuid() == 0 {
		if err := exec.Command("sync").Run(); err != nil {
			return fmt.Errorf("sync: %w", err)
		}
		return os.WriteFile("/proc/sys/vm/drop_caches", []byte("3"), 0)
	}
	_, err := elevate("sh", "-c", "sync; echo 3 > /proc/sys/vm/drop_caches")
	return err
}

// statePaths are the files a package operation actually rewrites, plus the
// folders loose apps land in. Directories count because their mtime moves when
// an entry is added, which is the only cheap signal apt, rpm and pacman give.
func statePaths() []string {
	h := homeShort()
	var out []string
	out = append(out,
		"/var/lib/dpkg/status", "/var/lib/dpkg/info",
		"/var/lib/rpm", "/var/lib/pacman/sync", "/var/log/pacman.log",
		"/var/lib/snapd/state.json", "/var/lib/flatpak",
		"/usr/local/lib/node_modules", "/usr/lib/node_modules")
	if h != "" {
		out = append(out, h+"/.local/share/flatpak", h+"/Apps",
			h+"/.npm-global/lib/node_modules", h+"/.nvm/versions/node")
	}
	return out
}
