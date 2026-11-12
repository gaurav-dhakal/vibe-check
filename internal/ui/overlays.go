package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gaurav-dhakal/vibe-check/internal/engine"
	"github.com/sahilm/fuzzy"
)

// ---- ANSI-aware screen splicing (overlays without clearing) ----

// ansiCut returns the substring of styled line s covering display
// columns [lo, hi). Escape sequences carry no width, so every one is replayed
// to keep SGR state correct, and a reset closes the slice so styles never leak
// into whatever is painted next.
func ansiCut(s string, lo, hi int) string {
	if lo < 0 {
		lo = 0
	}
	if hi <= lo {
		return ""
	}
	var b strings.Builder
	col := 0
	styled := false
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '\x1b' {
			j := i + 1
			switch {
			case j < len(rs) && rs[j] == '[':
				j++
				for j < len(rs) && !(rs[j] >= 0x40 && rs[j] <= 0x7e) {
					j++
				}
			case j < len(rs) && rs[j] == ']':
				j++
				for j < len(rs) && rs[j] != '\x07' && !(rs[j] == '\x1b' && j+1 < len(rs) && rs[j+1] == '\\') {
					j++
				}
			}
			if j >= len(rs) {
				break
			}
			seq := string(rs[i : j+1])
			b.WriteString(seq)
			if strings.HasPrefix(seq, "\x1b[") && strings.HasSuffix(seq, "m") {
				styled = true
			}
			i = j
			continue
		}
		w := runeWidth(r)
		if col >= lo && col < hi {
			b.WriteRune(r)
		}
		col += w
		if col >= hi {
			break
		}
	}
	if styled {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

func runeWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 0x7f:
		return 1
	case strings.ContainsRune("…▮▏▎▍▌▋▊▉─│╭╮╰╯▁▂▃▄▅▆▇█░▒▓▄▲▼⠁⠂⠄•·✔✘⚡❯▸", r):
		return 1
	case r >= 0x1100 && (r <= 0x115f || r >= 0x2e80 && r <= 0xa4cf):
		return 2
	default:
		return 1
	}
}

func paintLine(line string, x int, overlay string, w int) string {
	before := ansiCut(line, 0, x)
	after := ansiCut(line, x+w, 1<<30)
	return before + overlay + after
}

// ---- modal composition with 3D-ish shadow ----

func (m *model) modal(w, h int, inner string) string {
	// Fit every line here. lipgloss wraps by display column and its wrapper
	// happily splits an SGR sequence in half, which paints "38;2;203;166…" as
	// literal text on the row below.
	iw := max(4, w-4)
	lines := strings.Split(inner, "\n")
	for i, l := range lines {
		if ansiWidth(l) > iw {
			lines[i] = ansiCut(l, 0, iw)
		}
	}
	box := m.st.Modal.Width(w).Height(h).MaxHeight(h + 2)
	return box.Render(strings.Join(lines, "\n"))
}

func (m *model) overlayOn(base string, mx, my, mw, mh int, content string) string {
	lines := strings.Split(base, "\n")
	if mx < 0 {
		mx = 1
	}
	if my < 0 {
		my = 1
	}
	if mx+mw > m.width {
		mw = max(1, m.width-mx)
	}
	if my+mh > m.height {
		mh = max(1, m.height-my)
	}
	// ▓ drop shadow on the right edge, ▄ on the bottom
	for y := my; y < my+mh && y < len(lines); y++ {
		if x := mx + mw; x < m.width {
			lines[y] = paintLine(lines[y], x, m.st.Shadow.Render("▓"), 1)
		}
	}
	if y := my + mh; y < len(lines) {
		lines[y] = paintLine(lines[y], mx, m.st.Shadow.Render(strings.Repeat("▄", mw)), mw)
	}
	for i, cl := range strings.Split(content, "\n") {
		y := my + i
		if y >= len(lines) {
			break
		}
		lines[y] = paintLine(lines[y], mx, truncate(cl, mw), mw)
	}
	return strings.Join(lines, "\n")
}

// paintModal draws a box whose content is w columns wide and innerH rows
// tall, centred on the frame with its drop shadow.
//
// The geometry matters: a box rendered with Width(w) is w+2 columns on screen
// and Height(h) is h+2 rows, and every overlay used to tell the shadow layer
// w+4/h+2. The shadow therefore landed two cells off the box it was supposed
// to be touching, and taller-than-the-terminal content was quietly cropped —
// which is how "the help section gets hidden" happened on a short window.
func (m *model) paintModal(base string, w, contentRows int, inner string) string {
	w = max(8, min(w, m.width-4))
	contentRows = max(2, min(contentRows, m.height-8))
	// Guard, not the usual path: a box taller than the screen paints its own
	// bottom border and shadow over the key bar, so the lines that would fall
	// off are dropped here rather than by the terminal.
	if lines := strings.Split(inner, "\n"); len(lines) > contentRows {
		inner = strings.Join(lines[:contentRows], "\n")
	}
	mw, mh := w+2, contentRows+4
	x, y := m.centerModal(mw, mh)
	return m.overlayOn(base, x, y, mw, mh, m.modal(w, contentRows+2, inner))
}

// modalRows is how many content rows fit in a modal after the caller's
// reserved lines (title, totals, the hint row that must always be visible). The
// eight rows it keeps are the box's own borders and padding plus the key bar,
// which stays readable behind the drawer.
func (m *model) modalRows(reserved int) int { return max(2, m.height-8-reserved) }

func (m *model) centerModal(contentW, contentH int) (x, y int) {
	x = (m.width - contentW) / 2
	y = (m.height - contentH) / 2
	if x < 1 {
		x = 1
	}
	if y < 1 {
		y = 1
	}
	return x, y
}

// ---- help drawer ----

func (m *model) overlayHelp(base string) string {
	w := min(m.width-8, 100)
	twoCol := w >= 78
	if !twoCol {
		w = max(16, m.width-6)
	}

	lines := m.st.helpLines(w-4, twoCol)
	hint := m.st.Faint.Render("j/k scroll · esc close")
	// title…position…blank…hint: three rows besides the keymap itself
	rows := m.modalRows(3)
	maxSkip := max(0, len(lines)-rows)
	m.helpSkip = min(max(0, m.helpSkip), maxSkip)
	shown := lines[m.helpSkip:min(len(lines), m.helpSkip+rows)]

	inner := strings.Join(shown, "\n")
	if maxSkip > 0 {
		inner += "\n" + m.st.Faint.Render(fmt.Sprintf("  rows %d–%d of %d",
			m.helpSkip+1, m.helpSkip+len(shown), len(lines)))
	}
	inner = lipgloss.JoinVertical(lipgloss.Left, inner, "", hint)
	return m.paintModal(base, w, lipgloss.Height(inner), inner)
}

// handleHelpKey scrolls the drawer; the keymap is longer than most terminals
// are tall, and there was no way to see the rest of it.
func (m *model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "?", "f1", "q":
		m.helpOn = false
		m.helpSkip = 0
	case "j", "down", "ctrl+f":
		m.helpSkip++
	case "k", "up", "ctrl+b":
		m.helpSkip = max(0, m.helpSkip-1)
	case "g":
		m.helpSkip = 0
	case "G":
		m.helpSkip = 1 << 20 // clamped by the view
	}
	return m, nil
}

// helpLines lays the keymap out for w columns, in two columns when there is
// room and in one when there is not. The drawer used to force two columns and
// let the box wrapper cut the rows in half.
func (s *Styles) helpLines(w int, twoCol bool) []string {
	type grp struct {
		title string
		rows  [][2]string
	}
	// rows: key, description — single source of truth for the keymap
	groups := []grp{
		{"navigation", [][2]string{
			{"j / ↓", "move cursor down"},
			{"k / ↑", "move cursor up"},
			{"g / G", "jump to top / bottom"},
			{"ctrl+d / ctrl+u", "half page down / up"},
			{"h / l", "switch pane (dashboard) · tab (lists)"},
			{"1 – 6", "jump to tab"},
			{"f or space", "toggle fullscreen focus"},
			{"mouse", "click tabs/rows/panes · wheel scroll"},
		}},
		{"search & commands", [][2]string{
			{"/", "fuzzy filter current list"},
			{"esc", "clear filter · close overlay · unzoom"},
			{"ctrl+p", "command palette"},
			{"?", "this keymap drawer"},
		}},
		{"actions", [][2]string{
			{"y / Y", "copy row · copy row as JSON"},
			{"x / X", "kill · purge · uninstall · uninstall with files"},
			{"o", "orphan-only filter (processes)"},
			{"s", "cycle sort order"},
			{"r", "rescan disk · reload package list"},
			{"t", "cycle theme"},
			{"q / ctrl+c", "quit (terminal always restored)"},
		}},
		{"apps tab", [][2]string{
			{"x", "uninstall app only (keeps config)"},
			{"X", "uninstall and delete its files"},
			{"⏎", "list the files + folders the app owns"},
			{"i", "install from file (.deb/.rpm/.AppImage…)"},
			{"r", "reload the inventory"},
			{"s", "cycle sort: name · size · manager"},
			{"y", "copy the file listing (in the files modal)"},
		}},
		{"dashboard boost", [][2]string{
			{"c", "boost & clean (orphans + junk + caches)"},
			{"C", "toggle auto-clean purge"},
			{"b", "drop page cache (root)"},
		}},
		{"backend engines", [][2]string{
			{"orphans", "PPID==1 dev servers, RAM reclaimable"},
			{"ports", "/proc/net/tcp + synthetic HTTP HEAD"},
			{"docker", "raw unix socket, streaming stats"},
			{"disk", "low-priority crawler, concurrent purge"},
			{"apps", "apt · dnf · pacman · snap · flatpak · AppImage · npm · brew · winget"},
			{"fd", "/proc/sys/fs/file-nr + per-proc counts"},
		}},
	}

	colW := w
	if twoCol {
		colW = (w - 2) / 2
	}
	one := func(gs []grp, cw int) []string {
		out := make([]string, 0, 64)
		for _, g := range gs {
			out = append(out, s.HeadAccent.Render(truncate(g.title, cw)))
			for _, r := range g.rows {
				key := truncate(s.KeyCap.Render(padRight(r[0], 18)), 18)
				hint := s.KeyHint.Render(truncateSpace(r[1], cw-20))
				out = append(out, key+hint)
			}
			out = append(out, "")
		}
		return out
	}

	if !twoCol {
		return one(groups, colW)
	}
	left := one(groups[:3], colW)
	right := one(groups[3:], colW)
	n := max(len(left), len(right))
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out = append(out, truncate(l, colW)+" "+r)
	}
	return out
}

func truncateSpace(s string, w int) string {
	if w <= 0 || len(s) <= w {
		return s
	}
	return s[:w-1] + "…"
}

func padRight(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

// ---- command palette ----

type paletteItem struct {
	title string
	hint  string
	run   func(*model)
}

func (m *model) paletteCommands(q string) []paletteItem {
	all := []paletteItem{
		{"dashboard", "task manager · panes + sparklines", func(mm *model) { mm.setTab(TabDashboard) }},
		{"processes", "live table · kill orphans", func(mm *model) { mm.setTab(TabProcs) }},
		{"ports", "listeners + HTTP health", func(mm *model) { mm.setTab(TabPorts) }},
		{"docker", "container runtime socket", func(mm *model) { mm.setTab(TabDocker) }},
		{"disk artifacts", "node_modules / target / .next", func(mm *model) { mm.setTab(TabDisk) }},
		{"apps", "installed packages · uninstall", func(mm *model) { mm.setTab(TabApps) }},
		{"install app from file…", ".deb .rpm .AppImage .tar.gz .pkg .msi", func(mm *model) { mm.setTab(TabApps); mm.openInstallModal() }},
		{"browse selected app's files…", "what it put on disk + totals", func(mm *model) { mm.setTab(TabApps); mm.openFilesModal() }},
		{"boost performance", "kill orphans + purge junk + drop caches", func(mm *model) { mm.setTab(TabDashboard); mm.confirmBoost() }},
		{"toggle auto-clean", "unattended junk purge", func(mm *model) { mm.toggleAutoClean() }},
		{"reload package list", "re-read apt/rpm/pacman/snap/flatpak/npm/brew/winget", func(mm *model) { mm.setTab(TabApps); mm.eng.AppsReload() }},
		{"kill all orphaned dev servers", "reclaim forgotten RAM", func(mm *model) { mm.killAllOrphans() }},
		{"toggle orphan-only filter", "show only PPID==1 dev procs", func(mm *model) { mm.orphanOnly = !mm.orphanOnly }},
		{"rescan disk artifacts", "re-run low-priority crawler", func(mm *model) { mm.eng.Rescan(); mm.setToast("rescan queued…", "info") }},
		{"cycle theme", "catppuccin → tokyonight → gruvbox", func(mm *model) { mm.handleThemeCycle() }},
		{"toggle fullscreen focus", "zoom current pane/tab", func(mm *model) { mm.zoomed = !mm.zoomed }},
		{"copy snapshot JSON", "full telemetry snapshot → clipboard", func(mm *model) { mm.copyRow(true) }},
		{"show keymap", "every shortcut", func(mm *model) { mm.helpOn = true }},
		{"clear filter", "reset fuzzy search", func(mm *model) { mm.filter = ""; mm.search.SetValue("") }},
	}
	if q == "" {
		return all
	}
	strs := make([]string, len(all))
	for i, it := range all {
		strs[i] = it.title
	}
	matches := fuzzy.Find(strings.ToLower(q), strs)
	out := make([]paletteItem, 0, len(matches))
	for _, mm := range matches {
		out = append(out, all[mm.Index])
	}
	return out
}

func (m *model) killAllOrphans() {
	var killed int
	var totalRSS uint64
	for _, p := range m.snap.Procs {
		if p.Orphan {
			go m.eng.Kill(p.Pid, false)
			killed++
			totalRSS += p.RSS
		}
	}
	if killed == 0 {
		m.setToast("✔ no orphaned dev servers — RAM is clean", "ok")
		return
	}
	m.setToast(fmt.Sprintf("✔ killed %d orphaned servers · ~%s reclaiming", killed, engine.Bytes(totalRSS)), "ok")
}

func (m *model) handleThemeCycle() {
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
}

func (m *model) overlayPalette(base string) string {
	cmds := m.paletteCommands(m.palette.Value())
	if m.palSel > len(cmds)-1 {
		m.palSel = max(0, len(cmds)-1)
	}
	w := min(m.width-10, 68)
	rows := m.modalRows(6) // title, input, rule, "n more", blank, hint
	skip := 0
	if m.palSel >= rows {
		skip = m.palSel - rows + 1
	}
	list := ""
	for i, c := range cmds[skip:min(len(cmds), skip+rows)] {
		line := "  " + truncateSpace(c.title, 34)
		if lipgloss.Width(line) < w-24 {
			gap := (w - 24) - lipgloss.Width(line)
			if gap < 2 {
				gap = 2
			} else if gap > 24 {
				gap = 24
			}
			line += strings.Repeat(" ", gap) + c.hint
		}
		line = truncate(line, w-4)
		if skip+i == m.palSel {
			list += m.st.RowSel.Render("▸"+line[1:min(len(line), w-3)]) + "\n"
		} else {
			list += m.st.Row.Render(line) + "\n"
		}
	}
	if len(cmds) > skip+rows {
		list += m.st.Faint.Render(fmt.Sprintf("  ↓ %d more", len(cmds)-skip-rows)) + "\n"
	}
	inner := lipgloss.JoinVertical(lipgloss.Left,
		m.st.Acc2.Render("command palette"),
		m.palette.View(),
		m.st.Faint.Render(strings.Repeat("─", max(0, w-8))),
		strings.TrimRight(list, "\n"),
		m.st.Faint.Render("enter run · esc close · j/k select"),
	)
	return m.paintModal(base, w, lipgloss.Height(inner), inner)
}

// ---- confirm modal ----

func (m *model) overlayConfirm(base string) string {
	w := max(18, min(m.width-8, 60))
	// A boost sweep lists every directory it intends to delete, which is more
	// lines than a laptop screen is rows. The count that matters stays at the
	// top; the tail is summarised rather than cut off the bottom.
	lines := strings.Split(wrapText(m.confirm.detail, w-4), "\n")
	rows := m.modalRows(6) // title, 2 blanks, action row, "n more"
	more := 0
	if len(lines) > rows {
		more = len(lines) - rows
		lines = lines[:rows]
	}
	body := strings.Join(lines, "\n")
	if more > 0 {
		body += "\n" + m.st.Faint.Render(fmt.Sprintf("  … and %d more", more))
	}
	inner := lipgloss.JoinVertical(lipgloss.Left,
		m.st.Bad.Render(truncateSpace("⚠ "+m.confirm.title, w-4)),
		"",
		m.st.Text.Render(body),
		"",
		m.st.Acc.Render("y")+m.st.KeyHint.Render("confirm · ")+
			m.st.Acc.Render("n")+m.st.KeyHint.Render("cancel"),
	)
	return m.paintModal(base, w, lipgloss.Height(inner), inner)
}

func wrapText(s string, w int) string {
	if w <= 10 {
		w = 10
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		cur := ""
		for _, word := range strings.Fields(para) {
			if ansiWidth(cur)+runeLen(word)+1 > w && cur != "" {
				out = append(out, cur)
				cur = word
			} else if cur == "" {
				cur = word
			} else {
				cur += " " + word
			}
		}
		out = append(out, cur)
	}
	return strings.Join(out, "\n")
}

func runeLen(s string) int { return len([]rune(s)) }
