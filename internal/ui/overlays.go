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
	box := m.st.Modal.Width(w).Height(h).MaxHeight(h + 1)
	return box.Render(inner)
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
	w := min(m.width-6, 100)
	if w < 40 {
		w = max(20, m.width-4)
	}
	inner := m.st.helpBody(w - 6)
	mw := w + 4
	mh := lipgloss.Height(inner) + 2
	x, y := m.centerModal(mw, mh)
	return m.overlayOn(base, x, y, mw, mh, m.modal(w, mh-2, inner))
}

func (s *Styles) helpBody(w int) string {
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
			{"1 – 5", "jump to tab"},
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
			{"x / X", "kill (TERM/KILL) · purge · stop/start"},
			{"o", "orphan-only filter (processes)"},
			{"s", "cycle sort order"},
			{"r", "rescan disk artifacts"},
			{"t", "cycle theme"},
			{"q / ctrl+c", "quit (terminal always restored)"},
		}},
		{"backend engines", [][2]string{
			{"orphans", "PPID==1 dev servers, RAM reclaimable"},
			{"ports", "/proc/net/tcp + synthetic HTTP HEAD"},
			{"docker", "raw unix socket, streaming stats"},
			{"disk", "low-priority crawler, concurrent purge"},
			{"fd", "/proc/sys/fs/file-nr + per-proc counts"},
		}},
	}
	var b strings.Builder
	total := w - 2
	// two columns
	colW := total/2 - 2
	var left, right []string
	for _, g := range groups {
		var cg strings.Builder
		cg.WriteString(s.HeadAccent.Render(g.title) + "\n")
		for _, r := range g.rows {
			key := s.KeyCap.Render(padRight(r[0], 18))
			cg.WriteString(key + s.KeyHint.Render(truncateSpace(r[1], colW-20)) + "\n")
		}
		if len(left) < len(right)+1 && len(left) < 2 {
			left = append(left, cg.String())
		} else {
			right = append(right, cg.String())
		}
	}
	lcol := lipgloss.NewStyle().Width(colW).Render(strings.Join(left, "\n"))
	rcol := lipgloss.NewStyle().Width(colW).Render(strings.Join(right, "\n"))
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, lcol, rcol))
	return b.String()
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
		{"dashboard", "system overview · gauges + sparklines", func(mm *model) { mm.tab = TabDashboard }},
		{"processes", "live table · kill orphans", func(mm *model) { mm.tab = TabProcs }},
		{"ports", "listeners + HTTP health", func(mm *model) { mm.tab = TabPorts }},
		{"docker", "container runtime socket", func(mm *model) { mm.tab = TabDocker }},
		{"disk artifacts", "node_modules / target / .next", func(mm *model) { mm.tab = TabDisk }},
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
	list := ""
	for i, c := range cmds {
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
		if i == m.palSel {
			list += m.st.RowSel.Render("▸"+line[1:min(len(line), w-3)]) + "\n"
		} else {
			list += m.st.Row.Render(line) + "\n"
		}
	}
	inner := lipgloss.JoinVertical(lipgloss.Left,
		m.st.Acc2.Render("command palette")+"\n",
		m.palette.View(),
		m.st.Faint.Render(strings.Repeat("─", max(0, w-8)))+"\n",
		list,
		m.st.Faint.Render("enter run · esc close · j/k select"),
	)
	mh := lipgloss.Height(inner) + 2
	mw := w + 4
	x, y := m.centerModal(mw, mh)
	return m.overlayOn(base, x, y, mw, mh, m.modal(w, mh-2, inner))
}

// ---- confirm modal ----

func (m *model) overlayConfirm(base string) string {
	w := min(m.width-8, 56)
	inner := lipgloss.JoinVertical(lipgloss.Left,
		m.st.Bad.Render("⚠ "+m.confirm.title),
		"",
		m.st.Text.Render(wrapText(m.confirm.detail, w-8)),
		"",
		m.st.Acc.Render("y"), m.st.KeyHint.Render("confirm ")+
			m.st.Acc.Render(" n"), m.st.KeyHint.Render("cancel"),
	)
	mh := lipgloss.Height(inner) + 2
	mw := w + 4
	x, y := m.centerModal(mw, mh)
	return m.overlayOn(base, x, y, mw, mh, m.modal(w, mh-2, inner))
}

func wrapText(s string, w int) string {
	if w <= 10 {
		w = 10
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		cur := ""
		for _, word := range strings.Fields(para) {
			if len(cur)+len(word)+1 > w && cur != "" {
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
