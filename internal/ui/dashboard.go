package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gaurav-dhakal/vibe-check/internal/engine"
)

func (m *model) viewDashboard(w, h int) string {
	if m.zoomed {
		m.geo.panes = nil
		return m.dashPane(m.paneSel, w, h, true)
	}
	cw1 := w * 3 / 5
	cw2 := w - cw1
	rh1 := h * 3 / 5
	rh2 := h - rh1
	top := lipgloss.JoinHorizontal(lipgloss.Top,
		m.dashPane(0, cw1, rh1, m.paneSel == 0),
		m.dashPane(1, cw2, rh1, m.paneSel == 1))
	bot := lipgloss.JoinHorizontal(lipgloss.Top,
		m.dashPane(2, cw1, rh2, m.paneSel == 2),
		m.dashPane(3, cw2, rh2, m.paneSel == 3))
	m.geo.panes = []rect{
		{x: 0, y: m.geo.contentTop, w: cw1, h: rh1},
		{x: cw1, y: m.geo.contentTop, w: cw2, h: rh1},
		{x: 0, y: m.geo.contentTop + rh1, w: cw1, h: rh2},
		{x: cw1, y: m.geo.contentTop + rh1, w: cw2, h: rh2},
	}
	return lipgloss.JoinVertical(lipgloss.Left, top, bot)
}

func (m *model) dashPane(i, w, h int, active bool) string {
	switch i {
	case 1:
		return m.paneMem(w, h, active)
	case 2:
		return m.paneIO(w, h, active)
	case 3:
		return m.paneRuntime(w, h, active)
	}
	return m.paneCPU(w, h, active)
}

// spark01 renders values already on a fixed 0-100 scale.
func spark01(vals []float64, width int, sty lipgloss.Style) string {
	if len(vals) == 0 {
		return sty.Render(strings.Repeat("▁", min(width, 10)))
	}
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}
	out := make([]rune, 0, len(vals))
	for _, v := range vals {
		idx := int(v / 100 * 7)
		if idx < 0 {
			idx = 0
		}
		if idx > 7 {
			idx = 7
		}
		out = append(out, []rune(sparkChars)[idx])
	}
	s := sty.Render(string(out))
	return s + strings.Repeat(" ", max(0, width-len(out)))
}

func (m *model) pctStyle(pct float64) lipgloss.Style {
	switch {
	case pct > 85:
		return m.st.Bad
	case pct > 60:
		return m.st.Warn
	default:
		return m.st.Good
	}
}

func (m *model) paneCPU(w, h int, active bool) string {
	s := m.st
	sys := m.snap.Sys
	iw := max(w-4, 8)

	pct := fmt.Sprintf("%5.1f%%", sys.CPUPercent)
	head := m.pctStyle(sys.CPUPercent).Render(pct) + "  " +
		gaugeBar(sys.CPUPercent, min(iw-lipgloss.Width(pct)-3, 30), s)
	if sys.CPUModel != "" {
		head += "\n" + s.Faint.Render(truncate(sys.CPUModel, iw))
	}
	head += "\n" + s.Muted.Render(fmt.Sprintf("load %.2f %.2f %.2f", sys.Load1, sys.Load5, sys.Load15))

	body := head
	rows := h - 6 - lipgloss.Height(head)
	n := len(sys.CorePercents)
	barW := min(max(iw-12, 6), 22)
	shown := min(n, max(rows, 1))
	for i := 0; i < shown; i++ {
		lbl := s.Faint.Render(fmt.Sprintf("c%-2d", i))
		v := sys.CorePercents[i]
		body += "\n" + lbl + " " + gaugeBar(v, barW, s) +
			s.Muted.Render(rightPad(fmt.Sprintf("%4.0f", v), 5))
	}
	hist := m.eng.History().CPU.Values()
	body += "\n" + spark01(hist, min(iw, max(10, h)), s.Acc2)

	badge := ""
	if active && m.zoomed {
		badge = s.Acc2.Render(" f unzoom ")
	} else if n-shown > 0 {
		badge = s.Faint.Render(fmt.Sprintf("+%d cores", n-shown))
	}
	return s.pane("cpu", badge, w, h, active, body)
}

func (m *model) paneMem(w, h int, active bool) string {
	s := m.st
	sys := m.snap.Sys
	iw := max(w-4, 8)

	pct := fmt.Sprintf("%5.1f%%", sys.MemPercent)
	head := m.pctStyle(sys.MemPercent).Render(pct) + "  " +
		gaugeBar(sys.MemPercent, min(iw-lipgloss.Width(pct)-3, 30), s)
	body := head + "\n" +
		s.Muted.Render(fmt.Sprintf("used %s / %s · avail %s",
			engine.Bytes(sys.MemUsed), engine.Bytes(sys.MemTotal), engine.Bytes(sys.MemAvail)))
	if sys.SwapTotal > 0 {
		swPct := float64(sys.SwapUsed) / float64(sys.SwapTotal) * 100
		body += "\n" + s.Faint.Render("swap") + " " +
			gaugeBar(swPct, min(iw-14, 20), s) +
			s.Muted.Render(fmt.Sprintf(" %s/%s", engine.Bytes(sys.SwapUsed), engine.Bytes(sys.SwapTotal)))
	}
	hist := m.eng.History().Mem.Values()
	body += "\n" + spark01(hist, min(iw, max(10, h)), s.Acc)

	badge := ""
	if sys.MemPercent > float64(m.cfg.RAMAlertPct) {
		badge = s.badgePill(" RAM "+fmt.Sprintf("%.0f%%", sys.MemPercent), s.th.Warn)
	}
	if active && m.zoomed && badge == "" {
		badge = s.Acc2.Render(" f unzoom ")
	}
	return s.pane("memory", badge, w, h, active, body)
}

func (m *model) paneIO(w, h int, active bool) string {
	s := m.st
	sys := m.snap.Sys
	iw := max(w-4, 8)

	ioRate := func(label string, bps uint64, hist []float64, sty lipgloss.Style) string {
		bar := spark01(hist, max(6, iw-14), sty)
		return s.Faint.Render(label) + " " +
			s.Muted.Render(rightPad(engine.Bps(bps), 12)) + " " + bar
	}
	hist := m.eng.History()
	body := ioRate("disk read ", sys.DiskReadBps, hist.DiskR.Values(), s.Acc)
	body += "\n" + ioRate("disk write", sys.DiskWriteBps, hist.DiskW.Values(), s.Acc2)
	body += "\n" + s.Faint.Render(strings.Repeat("·", min(iw, 16)))
	body += "\n" + ioRate("net down  ", sys.NetRecvBps, hist.NetR.Values(), s.Good)
	body += "\n" + ioRate("net up    ", sys.NetSentBps, hist.NetW.Values(), s.Info)
	if sys.FDMax > 0 {
		fdLine := s.Faint.Render("fd ") +
			s.Muted.Render(fmt.Sprintf("%d", sys.FDAlloc))
		if sys.FDMax < 1<<30 {
			fdPct := float64(sys.FDAlloc) / float64(sys.FDMax) * 100
			fdLine += " " + gaugeBar(fdPct, max(8, iw-24), s) +
				s.Muted.Render(fmt.Sprintf(" /%d · %d avg/proc", sys.FDMax, sys.FDPerProc))
		} else {
			fdLine += s.Muted.Render(fmt.Sprintf(" · unlimited cap · %d avg/proc", sys.FDPerProc))
		}
		body += "\n" + fdLine
	}
	badge := ""
	if active && m.zoomed {
		badge = s.Acc2.Render(" f unzoom ")
	}
	return s.pane("i/o · fd", badge, w, h, active, body)
}

func (m *model) paneRuntime(w, h int, active bool) string {
	s := m.st
	sys := m.snap.Sys
	iw := max(w-4, 12)

	run, total := 0, len(m.snap.Containers)
	for _, c := range m.snap.Containers {
		if c.State == "running" {
			run++
		}
	}
	var b strings.Builder
	if m.snap.DockerErr != "" {
		b.WriteString(s.Faint.Render("docker ") + s.Bad.Render(truncate("socket: "+m.snap.DockerErr, iw-8)) + "\n")
	} else if total == 0 {
		b.WriteString(s.Faint.Render("docker ") + s.Muted.Render("0 containers") + "\n")
	} else {
		b.WriteString(s.Good.Render(fmt.Sprintf("● %d running", run)) +
			s.Faint.Render(fmt.Sprintf(" / %d total", total)) + "\n")
		for i, c := range m.snap.Containers {
			if i >= max(0, h-10) || i >= 8 {
				b.WriteString(s.Faint.Render(fmt.Sprintf("  +%d more…", total-i)) + "\n")
				break
			}
			dot := s.Bad.Render("○")
			if c.State == "running" {
				dot = s.Good.Render("●")
			}
			b.WriteString(dot + " " + s.Muted.Render(truncate(c.Name, iw-4)) + "\n")
		}
	}
	b.WriteString(s.Faint.Render("ports ") +
		s.Muted.Render(fmt.Sprintf("%d listeners", len(m.snap.Ports))))
	if n := sys.OrphanCount; n > 0 {
		b.WriteString("\n" + s.Warn.Render(fmt.Sprintf("⚠ %d orphans · %s", n, engine.Bytes(sys.OrphanRSS))) +
			s.Faint.Render("  x kill all"))
	} else {
		b.WriteString("\n" + s.Good.Render("✔ no orphaned dev servers"))
	}
	var artTotal uint64
	for _, a := range m.snap.Artifacts {
		artTotal += a.Size
	}
	scanBadge := s.Muted.Render(fmt.Sprintf("%d dirs · %s", len(m.snap.Artifacts), engine.Bytes(artTotal)))
	if m.snap.ScanState.Scanning {
		scanBadge = s.Acc.Render(m.spinFrame() + " scanning…")
	}
	b.WriteString("\n" + s.Faint.Render("disk  ") + scanBadge)
	if m.filter != "" {
		b.WriteString("\n" + s.Acc2.Render("/ "+m.filter))
	}

	badge := ""
	if active && m.zoomed {
		badge = s.Acc2.Render(" f unzoom ")
	}
	return s.pane("runtime", badge, w, h, active, b.String())
}
