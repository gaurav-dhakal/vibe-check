package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gaurav-dhakal/vibe-check/internal/engine"
)

func (m *model) renderHeader() string {
	st := m.st
	brand := st.BrandDot.Render("●") + " " + st.Brand.Render("vibe-check")
	host := st.HeaderDim.Render(m.snap.Sys.Hostname) + st.Faint.Render(" · ") +
		st.HeaderText.Render(m.snap.Sys.OS+"/"+fmt.Sprintf("%d", m.snap.Sys.NumCPU)+"c")
	up := st.HeaderDim.Render("up " + engine.Dur(m.snap.Sys.Uptime))

	cpuPct := m.snap.Sys.CPUPercent
	cpuSty := st.Good
	if cpuPct > 80 {
		cpuSty = st.Bad
	} else if cpuPct > 50 {
		cpuSty = st.Warn
	}
	cpu := cpuSty.Render(fmt.Sprintf("▮ %.0f%%", cpuPct)) + st.Faint.Render(" cpu")

	memPct := m.snap.Sys.MemPercent
	memSty := st.Good
	if memPct > float64(m.cfg.RAMAlertPct) {
		memSty = st.Bad
	} else if memPct > 60 {
		memSty = st.Warn
	}
	mem := memSty.Render(fmt.Sprintf("▮ %.0f%%", memPct)) + st.Faint.Render(" mem")

	sep := st.Faint.Render("  ·  ")
	var extras []string
	if m.snap.Sys.OrphanCount > 0 {
		extras = append(extras, st.Acc2.Render(fmt.Sprintf("⚠ %d orphans", m.snap.Sys.OrphanCount))+
			st.Faint.Render(" "+engine.Bytes(m.snap.Sys.OrphanRSS)))
	}
	extras = append(extras, up)
	// Degradation order for a narrow window: the least useful readout first,
	// then the hostname, then the memory gauge. The brand and the CPU number
	// are what always survive.
	left := brand + st.Faint.Render("  ") + host
	cells := append([]string{cpu, mem}, extras...)
	right := strings.Join(cells, sep)
	for ansiWidth(left)+ansiWidth(right)+1 > m.width && len(cells) > 2 {
		cells = cells[:len(cells)-1]
		right = strings.Join(cells, sep)
	}
	if ansiWidth(left)+ansiWidth(right)+1 > m.width {
		left = brand
	}
	if ansiWidth(left)+ansiWidth(right)+1 > m.width {
		right = cpu
	}
	if ansiWidth(left)+ansiWidth(right)+1 > m.width {
		left, right = ansiCut(brand, 0, m.width), ""
	}
	gap := max(1, m.width-ansiWidth(left)-ansiWidth(right))
	return fitBar(left+strings.Repeat(" ", gap)+right, m.width)
}

// ansiWidth measures display columns of a styled string, skipping escapes.
//
// A CSI's introducer '[' is itself inside the 0x40–0x7e range that ends a
// sequence, so the naive "first such byte closes it" version stopped after the
// ESC and then counted "1;38;2;137;179;250m" as twenty columns of text. Every
// truecolor hint looked far wider than it was, which is how the key bar and
// the header ended up wrapping and shifting after one cell.
func ansiWidth(s string) int {
	col := 0
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '\x1b' && i+1 < len(rs) {
			switch rs[i+1] {
			case '[':
				i += 2
				for i < len(rs) && (rs[i] < 0x40 || rs[i] > 0x7e) {
					i++
				}
			case ']':
				i += 2
				for i < len(rs) && rs[i] != '\x07' && rs[i] != '\x1b' {
					i++
				}
				if i < len(rs) && rs[i] == '\x1b' {
					i++ // the ST terminator "\x1b\"
				}
			default:
				i++
			}
			continue
		}
		col += runeWidth(r)
	}
	return col
}

// tabShortNames are the labels used when the full ones do not fit. A 40-column
// terminal gets "dash procs ports dock disk apps" instead of a strip that
// drags the whole frame wider than the window.
var tabShortNames = [TabCount]string{"dash", "procs", "ports", "dock", "disk", "apps"}

type tabLayout struct{ short, counts, tight bool }

func (m *model) tabCounts() [TabCount]string {
	var c [TabCount]string
	if n := m.snap.Sys.OrphanCount; n > 0 {
		c[TabProcs] = fmt.Sprintf("%d", n)
	}
	c[TabPorts] = fmt.Sprintf("%d", len(m.snap.Ports))
	if n := len(m.snap.Containers); n > 0 {
		run := 0
		for _, ct := range m.snap.Containers {
			if ct.State == "running" {
				run++
			}
		}
		c[TabDocker] = fmt.Sprintf("%d/%d", run, n)
	}
	var total uint64
	for _, a := range m.snap.Artifacts {
		total += a.Size
	}
	if total > 0 {
		c[TabDisk] = engine.Bytes(total)
	}
	if n := len(m.eng.AppsPackages()); n > 0 {
		c[TabApps] = fmt.Sprintf("%d", n)
	}
	return c
}

// tabsBlock draws the tab strip in the widest layout that still fits, with its
// click rects. An empty strip means nothing fit: giving up the row buys the
// list a row, and at three dozen columns "process…" says less than the keys
// 1–6 already documented in the help drawer.
func (m *model) tabsBlock() (string, []rect) {
	cnt := m.tabCounts()
	for _, lay := range []tabLayout{{counts: true}, {counts: false}, {short: true}, {short: true, tight: true}} {
		bar, rects, w := m.tabsLine(lay, cnt)
		if w <= m.width {
			m.tabH = 1
			return bar, rects
		}
	}
	m.tabH = 0
	return "", nil
}

func (m *model) tabsLine(lay tabLayout, cnt [TabCount]string) (string, []rect, int) {
	st := m.st
	var b strings.Builder
	var rects []rect
	b.WriteString("  ")
	x, total := 2, 2
	for i, name := range tabNames {
		label := name
		if lay.short {
			label = tabShortNames[i]
		}
		if lay.counts && cnt[i] != "" {
			label += " " + cnt[i]
		}
		var cell string
		switch {
		case lay.tight && Tab(i) == m.tab:
			cell = st.KeyCap.Render(label)
		case lay.tight:
			cell = st.Muted.Render(label)
		case Tab(i) == m.tab:
			cell = st.TabActive.Render(label)
		default:
			cell = st.TabInactive.Render(label)
		}
		cw := ansiWidth(cell)
		rects = append(rects, rect{x: x, y: m.headerH(), w: cw, h: 1})
		b.WriteString(cell)
		b.WriteString(" ")
		x, total = x+cw+1, total+cw+1
	}
	return b.String(), rects, total
}

// footerBlock renders the key bar and reports how many rows it needs.
//
// Hints wrap onto a second row instead of running off the end of the line.
// That was the bug people hit on a 60-column terminal or a maximised-half
// window: the bar was one string, truncated, and "x uninstall" was the first
// thing to disappear — so the action looked like it did not exist.
func (m *model) footerBlock() (string, int) {
	st := m.st
	sep := st.Faint.Render(" · ")

	cells := m.keyCells(m.searchOn)
	rows := layoutCells(cells, sep, m.width-2)

	toast := m.toastCell()
	status := m.statusCells(st.Faint.Render("  ·  "))

	// A toast is the answer to the key you just pressed, so it never gets cut:
	// if it cannot ride at the end of the hints, it opens a row of its own.
	if toast != "" && len(rows) < footerRowsMax &&
		ansiWidth(rows[len(rows)-1])+ansiWidth(toast)+4 > m.width-2 {
		rows = append(rows, "")
	}

	last := len(rows) - 1
	// Status is flush right on the final row. Least significant first in the
	// slice, so a tight bar loses the scan clock, then the theme name; the
	// hints are the part worth reading, and they are never cut mid-word.
	hint := rows[last]
	for len(status) > 0 &&
		ansiWidth(hint)+ansiWidth(toast)+2+ansiWidth(strings.Join(status, sep))+2 > m.width-2 {
		status = status[1:]
	}
	tail := strings.Join(status, sep)
	if toast != "" {
		hint += "  " + toast
	}
	gap := max(1, m.width-2-ansiWidth(hint)-ansiWidth(tail))
	rows[last] = hint + strings.Repeat(" ", gap) + tail

	for i := range rows {
		rows[i] = st.Footer.Render(fitBar(rows[i], m.width))
	}
	return strings.Join(rows, "\n"), len(rows)
}

// footerRowsMax caps the bar at two lines: a third would eat the list it is
// documenting, and the tail of a long keymap is the least useful of it.
const footerRowsMax = 2

func (m *model) statusCells(sep string) []string {
	st := m.st
	var parts []string
	if m.busy() {
		parts = append(parts, st.Muted.Render(m.spinFrame()+" scanning"))
	} else if !m.snap.ScanState.LastDoneAt.IsZero() {
		parts = append(parts, st.Faint.Render(fmt.Sprintf("scan %ds ago",
			int(time.Since(m.snap.ScanState.LastDoneAt).Seconds()))))
	}
	// The full description names the variant and the colour mode; on a narrow
	// bar the plain key carries the same idea in a third of the columns.
	name := m.th.Describe()
	if m.width < 72 {
		name = m.key
	}
	parts = append(parts, st.Faint.Render(name))
	return parts
}

// fitBar pads a bar to the full width so its background has no gaps.
func fitBar(s string, w int) string {
	if n := w - ansiWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return ansiCut(s, 0, max(4, w))
}

// layoutCells greedily packs hint cells into rows of at most w columns, never
// using more than footerRowsMax of them.
func layoutCells(cells []string, sep string, w int) []string {
	sw := ansiWidth(sep)
	var rows []string
	cur, curW := "", 0
	for _, c := range cells {
		cw := ansiWidth(c)
		want := cw
		if cur != "" {
			want += sw
		}
		if cur != "" && curW+want > w {
			if len(rows) == footerRowsMax-1 {
				// No further row: mark the cut so the keys that fell off are
				// visibly missing rather than silently gone.
				return append(rows, cur+"…")
			}
			rows = append(rows, cur)
			cur, curW = "", 0
			want = cw
		}
		if cur != "" {
			cur += sep
			curW += sw
		}
		cur += c
		curW += cw
	}
	if cur != "" {
		rows = append(rows, cur)
	}
	if len(rows) == 0 {
		rows = []string{""}
	}
	return rows
}

func (m *model) toastCell() string {
	if m.toast.text == "" {
		return ""
	}
	st := m.st
	sty := st.Info
	switch m.toast.level {
	case "ok":
		sty = st.Good
	case "warn":
		sty = st.Warn
	case "error":
		sty = st.Bad
	}
	return sty.Render(truncate(" "+m.toast.text+" ", min(m.width-10, 64)))
}

// keyCells is the bar's content, most actionable first — the tail is what a
// narrow terminal drops. With typing on, the filter slot shows the input
// instead of the key that opens it.
func (m *model) keyCells(typing bool) []string {
	st := m.st
	kc := func(k, v string) string { return st.KeyCap.Render(k) + " " + st.KeyHint.Render(v) }
	var parts []string
	switch m.tab {
	case TabDashboard:
		parts = append(parts, kc("h/l", "panes"), kc("f", "zoom"), kc("x", "kill orphans"), kc("c", "boost"), kc("C", "auto-clean"))
	case TabProcs:
		parts = append(parts, kc("j/k", "select"), kc("x", "kill"), kc("X", "force"), kc("o", "orphans"), kc("s", "sort"))
	case TabPorts:
		parts = append(parts, kc("j/k", "select"), kc("y", "copy"))
	case TabDocker:
		parts = append(parts, kc("x", "stop/start"), kc("y", "copy id"))
	case TabDisk:
		parts = append(parts, kc("x", "purge"), kc("y", "copy path"), kc("r", "rescan"))
	case TabApps:
		parts = append(parts, kc("x", "uninstall"), kc("X", "+files"), kc("⏎", "files"),
			kc("i", "install file"), kc("r", "reload"), kc("s", "sort"))
	}
	filter := kc("/", "filter")
	if typing {
		filter = st.Acc.Render("/") + " " + m.search.View()
	}
	return append(parts, filter, kc("⌃P", "commands"), kc("?", "help"), kc("q", "quit"))
}

// ---- mouse ----

func (m *model) handleMouse(msg tea.MouseMsg) {
	if m.filesOn || m.installOn || m.paletteOn || m.searchOn || m.confirm != nil {
		return // an overlay owns the screen; row clicks would be guesses
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.move(-3)
	case tea.MouseButtonWheelDown:
		m.move(3)
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return
		}
		y := msg.Y
		// tab bar hit
		if y == m.headerH() {
			for i, r := range m.geo.tabBtns {
				if msg.X >= r.x && msg.X < r.x+r.w {
					m.setTab(Tab(i))
					return
				}
			}
		}
		if m.tab == TabDashboard {
			for i, r := range m.geo.panes {
				if msg.X >= r.x && msg.X < r.x+r.w && y >= r.y && y < r.y+r.h {
					if m.paneSel == i && m.zoomed {
						m.zoomed = false
					} else if m.paneSel == i {
						m.zoomed = true
					} else {
						m.paneSel = i
					}
					return
				}
			}
			return
		}
		// list rows
		if m.geo.rowTop > 0 && y >= m.geo.rowTop && y < m.geo.rowTop+m.geo.listRows {
			idx := m.off[m.tab] + (y - m.geo.rowTop)
			n := m.listLen()
			if idx < n {
				m.sel[m.tab] = idx
				m.scrollToSel()
			}
		}
	}
}
