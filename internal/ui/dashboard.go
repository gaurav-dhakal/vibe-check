package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gaurav-dhakal/vibe-check/internal/engine"
)

// ringVals reads one history ring into the model scratch buffer under the
// engine lock, so the 60fps loop allocates no history slices at all.
func (m *model) ringVals(get func(*engine.History) *engine.Ring) []float64 {
	m.histScr = m.eng.RingValues(get, m.histScr)
	return m.histScr
}

// dashCols is how many panes fit side by side: six panes of task-manager
// numbers need about 16 columns each to read, so a narrow terminal gets two
// columns, then one, instead of six boxes whose contents wrapped.
func dashCols(w int) int {
	switch {
	case w >= 96:
		return 3
	case w >= 44:
		return 2
	}
	return 1
}

func (m *model) viewDashboard(w, h int) string {
	if m.zoomed {
		m.geo.panes = nil
		return m.dashPane(m.paneSel%dashPanes, w, h, true)
	}
	nc := dashCols(w)
	nr := (dashPanes + nc - 1) / nc

	cw := make([]int, nc)
	for c := range cw {
		cw[c] = w / nc
	}
	cw[nc-1] = w - (nc-1)*(w/nc)
	rh := make([]int, nr)
	for r := range rh {
		rh[r] = h / nr
	}
	rh[nr-1] = h - (nr-1)*(h/nr)

	rows := make([]string, nr)
	panes := make([]rect, 0, dashPanes)
	for r := 0; r < nr; r++ {
		cells := make([]string, 0, nc)
		x := 0
		for c := 0; c < nc; c++ {
			i := r*nc + c
			if i >= dashPanes {
				break // only when the pane count stops dividing the columns
			}
			cells = append(cells, m.dashPane(i, cw[c], rh[r], m.paneSel%dashPanes == i))
			panes = append(panes, rect{x, m.geo.contentTop + r*rh[r], cw[c], rh[r]})
			x += cw[c]
		}
		rows[r] = lipgloss.JoinHorizontal(lipgloss.Top, cells...)
	}
	m.geo.panes = panes
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m *model) dashPane(i, w, h int, active bool) string {
	switch i {
	case 1:
		return m.paneMem(w, h, active)
	case 2:
		return m.paneStorage(w, h, active)
	case 3:
		return m.paneIO(w, h, active)
	case 4:
		if len(m.snap.GPUs) > 0 {
			return m.paneGPU(w, h, active)
		}
		return m.paneRuntime(w, h, active)
	case 5:
		return m.paneBoost(w, h, active)
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

// paneCPU is the task-manager centerpiece: total gauge, identity line,
// then one live sparkline per core.
func (m *model) paneCPU(w, h int, active bool) string {
	s := m.st
	sys := m.snap.Sys
	iw := max(w-4, 8)

	pct := fmt.Sprintf("%5.1f%%", sys.CPUPercent)
	head := m.pctStyle(sys.CPUPercent).Render(pct) + "  " +
		gaugeBar(sys.CPUPercent, min(iw-lipgloss.Width(pct)-3, 20), s)
	if sys.CPUModel != "" {
		head += "\n" + s.Faint.Render(truncate(sys.CPUModel, iw))
	}
	cores := len(sys.CorePercents)
	ident := fmt.Sprintf("%d cores", cores)
	if mhz := maxMHz(sys.CoreMHz); mhz > 0 {
		ident += fmt.Sprintf(" · %.2f GHz", mhz/1000) // gopsutil reports MHz
	}
	head += "\n" + s.Muted.Render(fmt.Sprintf("%s · load %.2f %.2f %.2f",
		ident, sys.Load1, sys.Load5, sys.Load15))

	body := head
	rows := h - 6 - lipgloss.Height(head)
	shown := min(cores, max(rows, 0))
	barW := min(max(iw-14, 6), 28)
	for i := 0; i < shown; i++ {
		v := sys.CorePercents[i]
		vals := m.ringVals(func(hh *engine.History) *engine.Ring {
			if i < len(hh.CPUCores) {
				return &hh.CPUCores[i]
			}
			return nil
		})
		body += "\n" + s.Faint.Render(fmt.Sprintf("c%-2d", i)) + " " +
			spark01(vals, barW, m.pctStyle(v)) +
			s.Muted.Render(rightPad(fmt.Sprintf("%4.0f", v), 6))
	}
	hist := m.ringVals(func(hh *engine.History) *engine.Ring { return &hh.CPU })
	body += "\n" + spark01(hist, iw, s.Acc2)

	badge := ""
	if active && m.zoomed {
		badge = s.Acc2.Render(" f unzoom ")
	} else if cores-shown > 0 {
		badge = s.Faint.Render(fmt.Sprintf("+%d cores", cores-shown))
	}
	return s.pane("cpu · per core", badge, w, h, active, body)
}

func maxMHz(v []float64) float64 {
	var max float64
	for _, x := range v {
		if x > max {
			max = x
		}
	}
	return max
}

func (m *model) paneMem(w, h int, active bool) string {
	s := m.st
	sys := m.snap.Sys
	iw := max(w-4, 8)

	pct := fmt.Sprintf("%5.1f%%", sys.MemPercent)
	head := m.pctStyle(sys.MemPercent).Render(pct) + "  " +
		gaugeBar(sys.MemPercent, min(iw-lipgloss.Width(pct)-3, 20), s)
	body := head + "\n" +
		s.Muted.Render(fmt.Sprintf("used %s / %s · avail %s",
			engine.Bytes(sys.MemUsed), engine.Bytes(sys.MemTotal), engine.Bytes(sys.MemAvail)))
	if sys.SwapTotal > 0 {
		swPct := float64(sys.SwapUsed) / float64(sys.SwapTotal) * 100
		body += "\n" + s.Faint.Render("swap") + " " +
			gaugeBar(swPct, min(iw-14, 16), s) +
			s.Muted.Render(fmt.Sprintf(" %s/%s", engine.Bytes(sys.SwapUsed), engine.Bytes(sys.SwapTotal)))
	}
	hist := m.ringVals(func(hh *engine.History) *engine.Ring { return &hh.Mem })
	body += "\n" + spark01(hist, iw, s.Acc)

	badge := ""
	if sys.MemPercent > float64(m.cfg.RAMAlertPct) {
		badge = s.badgePill(" RAM "+fmt.Sprintf("%.0f%%", sys.MemPercent), s.th.Warn)
	}
	if active && m.zoomed && badge == "" {
		badge = s.Acc2.Render(" f unzoom ")
	}
	return s.pane("memory", badge, w, h, active, body)
}

// paneStorage charts every real partition: usage gauge, per-mount numbers
// and the total free space across all mounts.
func (m *model) paneStorage(w, h int, active bool) string {
	s := m.st
	sys := m.snap.Sys
	iw := max(w-4, 8)

	var body strings.Builder
	if len(sys.Disks) == 0 {
		body.WriteString(s.Muted.Render("no real filesystems detected"))
	}
	barW := min(max(iw-34, 4), 16)
	shown := 0
	limit := max(1, h-7)
	var freeTotal, sizeTotal uint64
	for _, d := range sys.Disks {
		if shown >= limit {
			body.WriteString("\n" + s.Faint.Render(fmt.Sprintf("+%d more…", len(sys.Disks)-shown)))
			break
		}
		free := uint64(0)
		if d.Total > d.Used {
			free = d.Total - d.Used
		}
		freeTotal += free
		sizeTotal += d.Total
		body.WriteString("\n" + s.Muted.Render(rightPad(truncate(d.Mount, 10), 10)) + " " +
			gaugeBar(d.Percent, barW, s) + " " +
			m.pctStyle(d.Percent).Render(fmt.Sprintf("%3.0f%%", d.Percent)) +
			s.Faint.Render(" "+engine.Bytes(free)+" free") +
			s.Muted.Render(rightPad(engine.Bytes(d.Total), 9)))
		shown++
	}
	if sizeTotal > 0 {
		body.WriteString("\n" + s.Faint.Render(strings.Repeat("·", min(iw, 16))) + "\n")
		body.WriteString(s.Good.Render(engine.Bytes(freeTotal)) +
			s.Muted.Render(fmt.Sprintf(" free of %s", engine.Bytes(sizeTotal))))
	}
	hist := m.ringVals(func(hh *engine.History) *engine.Ring { return &hh.Disk })
	body.WriteString("\n" + spark01(hist, iw, s.Acc))

	badge := ""
	if active && m.zoomed {
		badge = s.Acc2.Render(" f unzoom ")
	}
	return s.pane("disk · usage", badge, w, h, active, body.String())
}

func (m *model) paneIO(w, h int, active bool) string {
	s := m.st
	sys := m.snap.Sys
	iw := max(w-4, 8)

	// byte-rate rings span orders of magnitude, so they need the
	// auto-scaling sparkline, not the fixed 0-100 gauge.
	ioRate := func(label string, bps uint64, ring func(*engine.History) *engine.Ring, sty lipgloss.Style) string {
		bar := sparkline(m.ringVals(ring), max(6, iw-14), sty)
		return s.Faint.Render(label) + " " +
			s.Muted.Render(rightPad(engine.Bps(bps), 12)) + " " + bar
	}
	body := ioRate("read  ", sys.DiskReadBps, func(hh *engine.History) *engine.Ring { return &hh.DiskR }, s.Acc)
	body += "\n" + ioRate("write ", sys.DiskWriteBps, func(hh *engine.History) *engine.Ring { return &hh.DiskW }, s.Acc2)
	body += "\n" + s.Faint.Render(strings.Repeat("·", min(iw, 16)))
	body += "\n" + ioRate("down  ", sys.NetRecvBps, func(hh *engine.History) *engine.Ring { return &hh.NetR }, s.Good)
	body += "\n" + ioRate("up    ", sys.NetSentBps, func(hh *engine.History) *engine.Ring { return &hh.NetW }, s.Info)
	if sys.FDMax > 0 {
		fdLine := s.Faint.Render("fd ") +
			s.Muted.Render(fmt.Sprintf("%d", sys.FDAlloc))
		if sys.FDMax < 1<<30 {
			fdPct := float64(sys.FDAlloc) / float64(sys.FDMax) * 100
			fdLine += " " + gaugeBar(fdPct, max(6, iw-24), s) +
				s.Muted.Render(fmt.Sprintf(" /%d", sys.FDMax))
		} else {
			fdLine += s.Muted.Render(" · unlimited cap")
		}
		body += "\n" + fdLine
	}
	badge := ""
	if active && m.zoomed {
		badge = s.Acc2.Render(" f unzoom ")
	}
	return s.pane("i/o · net", badge, w, h, active, body)
}

func (m *model) paneRuntime(w, h int, active bool) string {
	s := m.st
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
			if i >= max(0, h-8) || i >= 6 {
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
	var artTotal uint64
	for _, a := range m.snap.Artifacts {
		artTotal += a.Size
	}
	scanBadge := s.Muted.Render(fmt.Sprintf("%d dirs · %s", len(m.snap.Artifacts), engine.Bytes(artTotal)))
	if m.snap.ScanState.Scanning {
		scanBadge = s.Acc.Render(m.spinFrame() + " scanning…")
	}
	b.WriteString("\n" + s.Faint.Render("junk  ") + scanBadge)

	badge := ""
	if active && m.zoomed {
		badge = s.Acc2.Render(" f unzoom ")
	}
	return s.pane("runtime", badge, w, h, active, b.String())
}

func (m *model) paneGPU(w, h int, active bool) string {
	s := m.st
	iw := max(w-4, 12)

	var b strings.Builder
	for i, g := range m.snap.GPUs {
		if i >= 2 {
			b.WriteString(s.Faint.Render(fmt.Sprintf("+%d more…", len(m.snap.GPUs)-i)) + "\n")
			break
		}
		if i > 0 {
			b.WriteString(s.Faint.Render(strings.Repeat("·", min(iw, 16))) + "\n")
		}
		pct := fmt.Sprintf("%5.1f%%", g.UtilPercent)
		b.WriteString(m.pctStyle(g.UtilPercent).Render(pct) + "  " +
			gaugeBar(g.UtilPercent, min(iw-lipgloss.Width(pct)-3, 20), s) + "\n")
		b.WriteString(s.Faint.Render(truncate(g.Name, iw)) + "\n")
		if g.MemTotal > 0 {
			mp := float64(g.MemUsed) / float64(g.MemTotal) * 100
			b.WriteString(s.Muted.Render(fmt.Sprintf("vram %s / %s",
				engine.Bytes(g.MemUsed), engine.Bytes(g.MemTotal))) + "\n")
			b.WriteString(gaugeBar(mp, min(iw-4, 20), s) + "\n")
		}
		if g.HasTemp {
			b.WriteString(s.Muted.Render(fmt.Sprintf("temp %.0f°C", g.TempC)) + "\n")
		}
	}

	badge := ""
	if active && m.zoomed {
		badge = s.Acc2.Render(" f unzoom ")
	}
	return s.pane("gpu", badge, w, h, active, b.String())
}

// paneBoost is the productivity cockpit: every reclaim lever with live
// numbers and the key to pull it.
func (m *model) paneBoost(w, h int, active bool) string {
	s := m.st
	floor := uint64(m.cfg.AutoCleanJunkMB) << 20

	var junk, junkBytes uint64
	for _, a := range m.snap.Artifacts {
		if a.Size >= floor {
			junk++
			junkBytes += a.Size
		}
	}

	kc := func(k, v string) string { return s.Acc.Render(k) + s.KeyHint.Render(" "+v) }
	var b strings.Builder
	if n := m.snap.Sys.OrphanCount; n > 0 {
		b.WriteString(s.Warn.Render(fmt.Sprintf("⚠ %d orphans", n)) +
			s.Muted.Render(" "+engine.Bytes(m.snap.Sys.OrphanRSS)+" RAM") + "\n")
		b.WriteString(kc("x", "kill them all") + "\n")
	} else {
		b.WriteString(s.Good.Render("✔ no orphaned dev servers") + "\n")
	}
	if junk > 0 {
		b.WriteString(s.Warn.Render(fmt.Sprintf("⚠ %d junk dirs", junk)) +
			s.Muted.Render(fmt.Sprintf(" ≥%dMB · %s", m.cfg.AutoCleanJunkMB, engine.Bytes(junkBytes))) + "\n")
	} else {
		b.WriteString(s.Muted.Render(fmt.Sprintf("build junk above %dMB: none", m.cfg.AutoCleanJunkMB)) + "\n")
	}
	b.WriteString(kc("c", "boost & clean sweep") + "\n")
	if m.eng.AutoClean() {
		b.WriteString(s.badgePill("auto-clean ON", s.th.Good) + "\n")
	} else {
		b.WriteString(s.Faint.Render("auto-clean off") + "\n")
	}
	b.WriteString(kc("C", "toggle auto-clean") + "\n")
	b.WriteString(kc("b", "drop page cache (root)") + "\n")
	b.WriteString(s.Muted.Render(fmt.Sprintf("free RAM: %s", engine.Bytes(m.snap.Sys.MemAvail))))

	badge := ""
	if active && m.zoomed {
		badge = s.Acc2.Render(" f unzoom ")
	}
	return s.pane("boost · clean", badge, w, h, active, b.String())
}
