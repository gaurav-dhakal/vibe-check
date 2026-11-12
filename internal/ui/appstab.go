package ui

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gaurav-dhakal/vibe-check/internal/apps"
	"github.com/gaurav-dhakal/vibe-check/internal/config"
	"github.com/gaurav-dhakal/vibe-check/internal/engine"
)

// ---- filtered + sorted package list ----

func (m *model) filteredPkgs() []apps.Package {
	_, gen, _ := m.eng.AppsStatus()
	key := appsFilterKey{gen, strings.ToLower(m.filter), m.sortMode[TabApps]}
	if m.appsKey == key {
		return m.appsVal
	}
	src := m.eng.AppsPackages()
	// while a search is live the match ranking is the useful order; the sort
	// column takes over again the moment the filter is cleared
	out := fuzzyKeep(m.filter, src, func(p apps.Package) string {
		return strings.ToLower(p.Name + " " + p.Manager + " " + p.Desc)
	})
	if m.filter == "" {
		out = slices.Clone(src)
		switch m.sortMode[TabApps] {
		case 1:
			sort.Slice(out, func(i, j int) bool { return out[i].SizeKB > out[j].SizeKB })
		case 2:
			sort.Slice(out, func(i, j int) bool {
				if out[i].Manager != out[j].Manager {
					return out[i].Manager < out[j].Manager
				}
				return out[i].Name < out[j].Name
			})
		default:
			sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		}
	}
	m.appsKey, m.appsVal = key, out
	return out
}

// ---- view ----

func (m *model) viewApps(w, h int) string {
	rows := m.filteredPkgs()
	n := len(rows)
	loading, gen, err := m.eng.AppsStatus()

	badge := m.st.Faint.Render(fmt.Sprintf("%d packages · sort:%s",
		n, []string{"name", "size", "manager"}[m.sortMode[TabApps]]))
	switch {
	case loading:
		badge = m.st.Acc.Render(m.spinFrame() + " reading package managers…")
	case err != "":
		badge = m.st.badgePill("inventory failed", m.st.th.Bad) + " " + m.st.Faint.Render(err)
	case gen == 0 && n == 0:
		badge = m.st.Faint.Render("press r to load the inventory")
	default:
		if m.filter != "" {
			badge = m.st.Acc.Render("filter "+fmt.Sprintf("%q", m.filter)) +
				m.st.Faint.Render(fmt.Sprintf(" · %d of %d · esc clears", n, len(m.eng.AppsPackages())))
		} else if ms := m.eng.AppsManagers(); len(ms) > 0 {
			badge += m.st.Faint.Render(" · " + strings.Join(ms, "+") + " · / search")
		}
	}

	cols := []column{
		{"SIZE", 9, false, true},
		{"MANAGER", 9, false, false},
		{"VERSION", 16, false, false},
		{"NAME", 22, false, false},
		{"INFO", 0, true, false},
	}

	cells := func(i int) []cellSpec {
		p := rows[i]
		size := "—"
		if p.SizeKB > 0 {
			size = fmtSizeKB(p.SizeKB)
		}
		return []cellSpec{
			{size, m.st.Muted, true},
			{p.Manager, m.st.Acc2, false},
			{p.Version, m.st.Faint, false},
			{p.Name, m.st.Text, false},
			{p.Desc, m.st.Muted, false},
		}
	}
	return m.renderList(w, h, "installed apps", badge, cols, cells, n, func(sel bool) lipgloss.Style {
		if sel {
			return m.st.RowSel
		}
		return m.st.Row
	})
}

func fmtSizeKB(kb int64) string { return engine.Bytes(uint64(kb) * 1024) }

// ---- install from file ----

// startInstall opens the typed-path install modal. The desktop's own file
// chooser is reached from inside it with Ctrl+f; it is never the default,
// because vibe-check spends most of its life in a terminal over SSH, in a
// container, or on a bare VT where no dialog can be drawn.
func (m *model) startInstall() tea.Cmd {
	return m.openInstallModal()
}

// pickFileMsg carries the chooser's answer back in. The dialog is a blocking
// OS call, so it runs off the update loop and reports through a message.
type pickFileMsg struct{ path, err string }

func pickFileCmd() tea.Cmd {
	return func() tea.Msg {
		p, err := apps.PickFile()
		switch {
		case err == nil:
			return pickFileMsg{path: p}
		case errors.Is(err, apps.ErrPickCancelled):
			return pickFileMsg{}
		default:
			return pickFileMsg{err: err.Error()}
		}
	}
}

func (m *model) handlePickedFile(msg pickFileMsg) (tea.Model, tea.Cmd) {
	m.picking = false
	switch {
	case msg.err != "":
		m.setToast("no file dialog here ("+msg.err+") — type the path", "warn")
		return m, m.openInstallModal()
	case msg.path == "":
		m.setToast("install cancelled — nothing chosen", "info")
		return m, nil
	}
	m.eng.AppsInstall(msg.path)
	return m, nil
}

func (m *model) openInstallModal() tea.Cmd {
	m.installOn = true
	m.install.SetValue("")
	return m.install.Focus()
}

func installHelp() string {
	switch runtime.GOOS {
	case "linux":
		return "supports .deb · .rpm · .tar.gz · .tar.xz · .AppImage (added to the app tray)"
	case "darwin":
		return "supports .pkg · .dmg · .app · .zip · .tar.gz"
	case "windows":
		return "supports .msi · .exe · .zip · .tar.gz"
	}
	return ""
}

func (m *model) handleInstallKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.installOn = false
		m.install.Blur()
		return m, nil
	case "ctrl+f":
		if !apps.PickerAvailable() {
			m.setToast("no desktop file chooser on this host — type the path", "warn")
			return m, nil
		}
		m.installOn = false
		m.install.Blur()
		m.picking = true
		return m, pickFileCmd()
	case "enter":
		p := strings.TrimSpace(m.install.Value())
		m.installOn = false
		m.install.Blur()
		if p == "" {
			m.setToast("no path given — install cancelled", "warn")
			return m, nil
		}
		m.eng.AppsInstall(p)
		return m, nil
	}
	var c tea.Cmd
	m.install, c = m.install.Update(msg)
	return m, c
}

func (m *model) overlayInstall(base string) string {
	w := max(24, min(m.width-10, 72))
	m.install.Width = max(12, w-10)
	// Wrapped here and styled per line: the modal box must never be the one
	// doing the wrapping, or it splits the colour codes it is wrapping.
	var help []string
	for _, l := range strings.Split(wrapText(installHelp(), w-4), "\n") {
		help = append(help, m.st.Faint.Render(l))
	}
	inner := lipgloss.JoinVertical(lipgloss.Left,
		m.st.Acc2.Render("install app from file"),
		"",
		m.st.Muted.Render("path to the file · ~/ works"),
		m.install.View(),
		strings.Join(help, "\n"),
		"",
		m.st.Acc.Render("enter")+m.st.KeyHint.Render(" install · ")+
			m.st.Acc.Render("ctrl+f")+m.st.KeyHint.Render(" browse · ")+
			m.st.Acc.Render("esc")+m.st.KeyHint.Render(" cancel"),
	)
	return m.paintModal(base, w, lipgloss.Height(inner), inner)
}

// ---- dashboard boost & clean ----

// confirmBoost collects the three reclaim levers into one confirmed sweep:
// orphan RAM, oversized build junk, and the page cache.
func (m *model) confirmBoost() {
	var junk, junkBytes uint64
	floor := uint64(m.cfg.AutoCleanJunkMB) << 20
	var big []string
	for _, a := range m.snap.Artifacts {
		if a.Size >= floor {
			junk++
			junkBytes += a.Size
			big = append(big, displayPath(a.Path))
		}
	}
	orphans := m.snap.Sys.OrphanCount
	lines := []string{}
	if orphans > 0 {
		lines = append(lines, fmt.Sprintf("kill %d orphaned dev servers (~%s RAM)", orphans, engine.Bytes(m.snap.Sys.OrphanRSS)))
	}
	if junk > 0 {
		lines = append(lines, fmt.Sprintf("purge %d junk dirs ≥ %d MB (~%s disk)", junk, m.cfg.AutoCleanJunkMB, engine.Bytes(junkBytes)))
	}
	lines = append(lines, "drop the kernel page cache (asks for root)")
	detail := strings.Join(lines, "\n")
	if junk > 0 && len(big) > 0 {
		detail += "\n\n" + strings.Join(big[:min(len(big), 6)], "\n")
	}
	m.confirm = &confirmState{
		title:  "boost performance",
		detail: detail,
		onYes: func(mm *model) {
			mm.eng.BoostCaches()
			mm.killAllOrphans()
			for _, a := range mm.snap.Artifacts {
				if a.Size >= floor {
					mm.flashDir[a.Path] = time.Now()
					mm.eng.Purge(a.Path)
				}
			}
		},
	}
}

func (m *model) toggleAutoClean() {
	on := !m.eng.AutoClean()
	m.eng.SetAutoClean(on)
	if err := config.Set(m.cfg.Path, "auto_clean", fmt.Sprintf("%v", on)); err == nil {
		m.cfg.AutoClean = on
	}
	if on {
		m.setToast(fmt.Sprintf("auto-clean ON · purge junk ≥ %d MB every minute", m.cfg.AutoCleanJunkMB), "warn")
	} else {
		m.setToast("auto-clean OFF", "info")
	}
}

// ---- per-app file browser ----

func (m *model) openFilesModal() tea.Cmd {
	pkgs := m.filteredPkgs()
	if len(pkgs) == 0 {
		return nil
	}
	m.filesPkg = pkgs[m.selectedIndex()]
	m.filesOn = true
	m.filesSkip = 0
	m.eng.AppsFiles(m.filesPkg)
	m.setAppsPin(true)
	return nil
}

func (m *model) closeFiles() {
	m.filesOn = false
	if m.tab != TabApps {
		m.setAppsPin(false)
	}
}

func (m *model) handleFilesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	_, _, _, errStr := m.eng.AppsFilesResult()
	vis := m.filesVisible()
	switch msg.String() {
	case "esc", "q", "enter", "backspace":
		m.closeFiles()
		return m, nil
	case "j", "down":
		m.filesSkip++
		return m, nil
	case "k", "up":
		m.filesSkip = max(0, m.filesSkip-1)
		return m, nil
	case "ctrl+d", "pagedown":
		m.filesSkip += vis
		return m, nil
	case "ctrl+u", "pageup":
		m.filesSkip = max(0, m.filesSkip-vis)
		return m, nil
	case "g":
		m.filesSkip = 0
		return m, nil
	case "G":
		if fl := m.filesList(); fl != nil {
			m.filesSkip = max(0, len(fl.Paths)-vis)
		}
		return m, nil
	case "y", "Y":
		if errStr == "" {
			m.copyFilesListing()
		}
		return m, nil
	case "x", "X":
		m.filesOn = false
		m.act(msg.String() == "X")
		return m, nil
	}
	return m, nil
}

// filesList returns the live listing only when it belongs to the package the
// overlay was opened for, so a half-finished walk never paints another app's
// paths under this one's name.
func (m *model) filesList() *apps.FileList {
	fl, key, _, _ := m.eng.AppsFilesResult()
	if fl == nil || key != engine.AppsFileKey(m.filesPkg) {
		return nil
	}
	return fl
}

func filesHintCells(s *Styles) []string {
	return []string{
		s.Acc.Render("esc") + s.KeyHint.Render(" close"),
		s.Acc.Render("j/k") + s.KeyHint.Render(" scroll"),
		s.Acc.Render("y") + s.KeyHint.Render(" copy paths"),
		s.Acc.Render("x") + s.KeyHint.Render(" uninstall"),
		s.Acc.Render("X") + s.KeyHint.Render(" uninstall + files"),
	}
}

// filesVisible is how many path rows the modal can actually paint. The scroll
// math and the drawing both read it here, so a row can never be counted as
// visible and then fall off the bottom of the screen.
func (m *model) filesVisible() int {
	hints := layoutCells(filesHintCells(m.st), m.st.Faint.Render(" · "),
		max(24, min(m.width-6, 92))-4)
	// title, meta, totals, the "n more" row, blank, source path
	return m.modalRows(6 + len(hints))
}

func (m *model) copyFilesListing() {
	fl := m.filesList()
	if fl == nil {
		return
	}
	go func() { _, _ = engine.Copy(strings.Join(fl.Paths, "\n")) }()
	m.setToast(fmt.Sprintf("✔ %d paths copied", len(fl.Paths)), "ok")
}

func (m *model) overlayFiles(base string) string {
	w := max(24, min(m.width-6, 92))
	innerW := w - 6

	// The key row wraps before the list is measured, so the two can never
	// disagree about who owns the last line of the box.
	hints := layoutCells(filesHintCells(m.st), m.st.Faint.Render(" · "), w-4)

	var b strings.Builder
	b.WriteString(m.st.Acc2.Render(truncateSpace(m.filesPkg.Name+" — files on disk", w-4)) + "\n")
	b.WriteString(m.st.Muted.Render(fmt.Sprintf("%s · %s · %s staged",
		m.filesPkg.Manager, orDash(m.filesPkg.Version), sizeOrDash(m.filesPkg.SizeKB))) + "\n")

	fl, key, loading, errStr := m.eng.AppsFilesResult()
	if key != engine.AppsFileKey(m.filesPkg) {
		fl, errStr, loading = nil, "", true // a newer request owns the result now
	}
	vis := m.filesVisible()
	switch {
	case loading:
		b.WriteString("\n" + m.st.Acc.Render(m.spinFrame()+" scanning…") +
			m.st.Faint.Render("  (j/k scroll once done · esc close)") + "\n")
	case errStr != "":
		b.WriteString("\n" + m.st.Bad.Render(truncateSpace("✘ "+errStr, w-4)) + "\n")
	case fl != nil:
		b.WriteString(m.st.Faint.Render(fmt.Sprintf("%s · %s · %s%s",
			countLabel(fl.Files, "file"), countLabel(fl.Dirs, "folder"), engine.Bytes(fl.Bytes),
			map[bool]string{true: " · list capped", false: ""}[fl.Truncated])) + "\n")
		m.filesSkip = min(max(0, m.filesSkip), max(0, len(fl.Paths)-vis))
		for _, p := range ansiPaths(fl.Paths[m.filesSkip:min(len(fl.Paths), m.filesSkip+vis)], innerW, m.st) {
			b.WriteString(p + "\n")
		}
		if len(fl.Paths) > m.filesSkip+vis {
			b.WriteString(m.st.Faint.Render(fmt.Sprintf("  ↓ %d more (j/k to scroll)",
				len(fl.Paths)-m.filesSkip-vis)) + "\n")
		}
	}

	// npm and AppImage apps *are* a directory, so the listing already contains
	// this line; repeating it reads like a bug rather than a caption.
	if p := displayPath(m.filesPkg.Path); m.filesPkg.Path != "" &&
		(len(fl.Paths) != 1 || displayPath(fl.Paths[0]) != p) {
		b.WriteString("\n" + m.st.Faint.Render("  "+truncateSpace(p, w-6)) + "\n")
	} else {
		b.WriteString("\n")
	}
	for _, h := range hints {
		b.WriteString(h + "\n")
	}

	inner := strings.TrimRight(b.String(), "\n")
	return m.paintModal(base, w, lipgloss.Height(inner), inner)
}

// ansiPaths dims the directory part and keeps the basename bright, so a long
// /usr/share/... listing is skimmable.
func ansiPaths(paths []string, width int, s *Styles) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		dir, file := filepath.Split(p)
		line := s.Faint.Render(truncateSpace(displayPath(dir), width-len(file)-2)) +
			s.Text.Render(file)
		out = append(out, truncate("  "+line, width))
	}
	return out
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func sizeOrDash(kb int64) string {
	if kb <= 0 {
		return "size unknown"
	}
	return fmtSizeKB(kb)
}

func countLabel(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// row identity + raw row for the apps tab are wired through lists.go.
