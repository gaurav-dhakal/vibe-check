package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	var rightParts []string
	if m.snap.Sys.OrphanCount > 0 {
		rightParts = append(rightParts, st.Acc2.Render(fmt.Sprintf("⚠ %d orphans", m.snap.Sys.OrphanCount))+
			st.Faint.Render(" "+engine.Bytes(m.snap.Sys.OrphanRSS)))
	}
	rightParts = append(rightParts, cpu, mem, up)
	right := strings.Join(rightParts, st.Faint.Render("  ·  "))

	left := strings.Join([]string{brand, host}, st.Faint.Render("  "))
	gap := m.width - ansiWidth(left) - ansiWidth(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// ansiWidth measures display columns of a styled string, skipping escapes.
func ansiWidth(s string) int {
	col, inEsc := 0, false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if r >= 0x40 && r <= 0x7e {
				inEsc = false
			}
			continue
		}
		col += runeWidth(r)
	}
	return col
}

func (m *model) renderTabs() string {
	st := m.st
	m.geo.tabBtns = nil
	var b strings.Builder
	b.WriteString("  ")
	x := 2
	for i, name := range tabNames {
		tab := Tab(i)
		count := ""
		switch tab {
		case TabProcs:
			if n := m.snap.Sys.OrphanCount; n > 0 {
				count = fmt.Sprintf(" %d", n)
			}
		case TabPorts:
			count = fmt.Sprintf(" %d", len(m.snap.Ports))
		case TabDocker:
			run := 0
			for _, c := range m.snap.Containers {
				if c.State == "running" {
					run++
				}
			}
			if n := len(m.snap.Containers); n > 0 {
				count = fmt.Sprintf(" %d/%d", run, n)
			}
		case TabDisk:
			var total uint64
			for _, a := range m.snap.Artifacts {
				total += a.Size
			}
			if total > 0 {
				count = " " + engine.Bytes(total)
			}
		}
		label := name + count
		w := lipgloss.Width(label) + 2
		if tab == m.tab {
			b.WriteString(st.TabActive.Render(label))
		} else {
			b.WriteString(st.TabInactive.Render(label))
		}
		m.geo.tabBtns = append(m.geo.tabBtns, rect{x: x, y: m.headerH(), w: w, h: 1})
		x += w
		b.WriteString(" ")
		x++
	}
	return b.String()
}

func (m *model) renderFooter() string {
	st := m.st

	left := m.contextKeys()
	if m.searchOn {
		left = st.Acc.Render("/") + st.Text.Render(" ") + m.search.View()
	}

	var rightParts []string
	if m.busy() {
		rightParts = append(rightParts, st.Muted.Render(m.spinFrame()+" scanning"))
	} else if !m.snap.ScanState.LastDoneAt.IsZero() {
		rightParts = append(rightParts, st.Faint.Render(fmt.Sprintf("scan %ds ago",
			int(time.Since(m.snap.ScanState.LastDoneAt).Seconds()))))
	}
	rightParts = append(rightParts, st.Faint.Render(m.th.Describe()))
	right := strings.Join(rightParts, st.Faint.Render("  ·  "))

	if m.toast.text != "" {
		sty := st.Info
		switch m.toast.level {
		case "ok":
			sty = st.Good
		case "warn":
			sty = st.Warn
		case "error":
			sty = st.Bad
		}
		toast := sty.Render(truncate(" "+m.toast.text+" ", min(m.width-8, 70)))
		gap := m.width - ansiWidth(left) - ansiWidth(toast) - ansiWidth(right) - 4
		if gap < 1 {
			left = truncate(left, max(4, m.width-ansiWidth(toast)-4))
			gap = 1
		}
		return st.Footer.Render(left + strings.Repeat(" ", gap) + toast + " " + right)
	}

	gap := m.width - ansiWidth(left) - ansiWidth(right)
	if gap < 1 {
		left = truncate(left, max(4, m.width-ansiWidth(right)-1))
		gap = 1
	}
	return st.Footer.Render(left + strings.Repeat(" ", gap) + right)
}

func (m *model) contextKeys() string {
	st := m.st
	kc := func(k, v string) string { return st.KeyCap.Render(k) + " " + st.KeyHint.Render(v) }
	parts := []string{}
	switch m.tab {
	case TabDashboard:
		parts = append(parts, kc("h/l", "panes"), kc("f", "zoom"))
	case TabProcs:
		parts = append(parts, kc("j/k", "select"), kc("x", "kill"), kc("X", "force"), kc("o", "orphans"), kc("s", "sort"))
	case TabPorts:
		parts = append(parts, kc("j/k", "select"), kc("y", "copy"))
	case TabDocker:
		parts = append(parts, kc("x", "stop/start"), kc("y", "copy id"))
	case TabDisk:
		parts = append(parts, kc("x", "purge"), kc("y", "copy path"), kc("r", "rescan"))
	}
	parts = append(parts, kc("/", "filter"), kc("⌃P", "commands"), kc("?", "help"), kc("q", "quit"))
	return strings.Join(parts, st.Faint.Render(" · "))
}

// ---- mouse ----

func (m *model) handleMouse(msg tea.MouseMsg) {
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
					m.tab = Tab(i)
					m.zoomed = false
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
