package ui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gaurav-dhakal/vibe-check/internal/engine"
)

// ---- filtered + sorted + cached lists ----

func (m *model) filteredProcs() []engine.Proc {
	key := filterKey{m.snap.At, strings.ToLower(m.filter), m.sortMode[TabProcs], m.orphanOnly}
	if m.procsKey == key {
		return m.procsVal
	}
	src := m.snap.Procs
	out := make([]engine.Proc, 0, len(src))
	for _, p := range src {
		if m.orphanOnly && !p.Orphan {
			continue
		}
		out = append(out, p)
	}
	if m.filter != "" {
		out = fuzzyKeep(m.filter, out, func(p engine.Proc) string {
			return strings.ToLower(fmt.Sprintf("%s %s %d", p.Name, p.CmdLine, p.Pid))
		})
	}
	switch m.sortMode[TabProcs] {
	case 1:
		sort.Slice(out, func(i, j int) bool { return out[i].RSS > out[j].RSS })
	case 2:
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	default:
		sort.Slice(out, func(i, j int) bool { return out[i].CPUPercent > out[j].CPUPercent })
	}
	m.procsKey, m.procsVal = key, out
	return out
}

func (m *model) filteredPorts() []engine.Port {
	key := filterKey{m.snap.At, strings.ToLower(m.filter), 0, false}
	if m.portsKey == key {
		return m.portsVal
	}
	src := m.snap.Ports
	out := fuzzyKeep(m.filter, src, func(p engine.Port) string {
		return strings.ToLower(fmt.Sprintf("%d %s %s %s", p.Port, p.Proto, p.ProcName, p.Addr))
	})
	m.portsKey, m.portsVal = key, out
	return out
}

func (m *model) filteredDocker() []engine.Container {
	key := filterKey{m.snap.At, strings.ToLower(m.filter), 0, false}
	if m.dockerKey == key {
		return m.dockerVal
	}
	src := m.snap.Containers
	out := fuzzyKeep(m.filter, src, func(c engine.Container) string {
		return strings.ToLower(c.Name + " " + c.Image + " " + c.ID + " " + c.State)
	})
	m.dockerKey, m.dockerVal = key, out
	return out
}

func (m *model) filteredArtifacts() []engine.Artifact {
	key := filterKey{m.snap.At, strings.ToLower(m.filter), m.sortMode[TabDisk], false}
	if m.diskKey == key {
		return m.diskVal
	}
	src := m.snap.Artifacts
	out := fuzzyKeep(m.filter, src, func(a engine.Artifact) string {
		return strings.ToLower(a.Path + " " + a.Kind + " " + a.Project)
	})
	switch m.sortMode[TabDisk] {
	case 1:
		sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	case 2:
		sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	default:
		sort.Slice(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	}
	m.diskKey, m.diskVal = key, out
	return out
}

func (m *model) rawRow(i int) any {
	switch m.tab {
	case TabProcs:
		if l := m.filteredProcs(); i < len(l) {
			return l[i]
		}
	case TabPorts:
		if l := m.filteredPorts(); i < len(l) {
			return l[i]
		}
	case TabDocker:
		if l := m.filteredDocker(); i < len(l) {
			return l[i]
		}
	case TabDisk:
		if l := m.filteredArtifacts(); i < len(l) {
			return l[i]
		}
	}
	return struct{}{}
}

func (m *model) rowIdentity(i int) string {
	switch m.tab {
	case TabProcs:
		if l := m.filteredProcs(); i < len(l) {
			if l[i].CmdLine != "" {
				return l[i].CmdLine
			}
			return l[i].Name
		}
	case TabPorts:
		if l := m.filteredPorts(); i < len(l) {
			p := l[i]
			s := fmt.Sprintf("%d (%s) pid %d %s", p.Port, p.Proto, p.Pid, p.ProcName)
			if p.HTTP != nil && p.HTTP.OK {
				s += fmt.Sprintf(" → %s %d %.1fms", p.HTTP.Scheme, p.HTTP.Status, p.HTTP.LatencyMs)
			}
			return s
		}
	case TabDocker:
		if l := m.filteredDocker(); i < len(l) {
			return l[i].ID
		}
	case TabDisk:
		if l := m.filteredArtifacts(); i < len(l) {
			return l[i].Path
		}
	}
	return ""
}

// ---- shared table renderer ----

type column struct {
	title string
	w     int
	flex  bool // absorbs remaining width (last column only)
	right bool
}

type cellSpec struct {
	text  string
	sty   lipgloss.Style
	right bool
}

func flexCols(cols []column, total int) []column {
	fixed := 0
	out := make([]column, len(cols))
	copy(out, cols)
	for i, c := range out {
		if c.flex {
			out[i].w = max(4, total-fixed)
			continue
		}
		fixed += c.w
	}
	return out
}

func rowCells(cols []column, cells []cellSpec) string {
	var b strings.Builder
	for i, c := range cols {
		if i >= len(cells) {
			break
		}
		cell := cells[i]
		w := c.w
		txt := cell.text
		if lipgloss.Width(txt) > w {
			txt = truncate(txt, w)
		} else if lipgloss.Width(txt) < w {
			pad := strings.Repeat(" ", w-lipgloss.Width(txt))
			if cell.right {
				txt = pad + txt
			} else {
				txt = txt + pad
			}
		}
		b.WriteString(cell.sty.Render(txt))
		if i < len(cols)-1 && i < len(cells)-1 {
			b.WriteString(" ")
		}
	}
	return b.String()
}

// renderList draws one full-size bordered pane containing a table.
// It records rowTop/listRows in geo for mouse hits.
func (m *model) renderList(w, h int, title, badge string, cols []column, cells func(idx int) []cellSpec, n int, selStyle func(bool) lipgloss.Style) string {
	iw := max(w-4, 20)
	cols = flexCols(cols, iw-1-max(0, len(cols)-1))
	visible := max(1, h-2-3) // borders + title + col header + separator

	var b strings.Builder
	hdrCells := make([]cellSpec, len(cols))
	for i, c := range cols {
		hdrCells[i] = cellSpec{text: truncate(c.title, c.w), sty: m.st.HeadDim, right: c.right}
	}
	b.WriteString(rowCells(cols, hdrCells) + "\n")
	b.WriteString(m.st.Faint.Render(strings.Repeat("─", iw)) + "\n")

	off := m.off[m.tab]
	shown := 0
	for i := off; i < n && shown < visible; i++ {
		sel := i == m.sel[m.tab]
		line := rowCells(cols, cells(i))
		sty := selStyle(sel)
		b.WriteString(sty.Render(line) + "\n")
		shown++
	}
	for shown < visible {
		b.WriteString("\n")
		shown++
	}
	m.geo.rowTop = m.geo.contentTop + 1 + 2 // border + title + header
	m.geo.listRows = shown
	return m.st.pane(title, badge, w, h, true, strings.TrimRight(b.String(), "\n"))
}

// ---- view: processes ----

func procColumns(w int) []column {
	return []column{
		{"PID", 6, false, true},
		{"CPU%", 5, false, true},
		{"MEM", 8, false, true},
		{"AGE", 6, false, true},
		{"USER", 8, false, false},
		{"NAME", 16, false, false},
		{"COMMAND", 0, true, false},
	}
}

func (m *model) viewProcs(w, h int) string {
	rows := m.filteredProcs()
	n := len(rows)
	sortName := []string{"cpu", "mem", "name"}[m.sortMode[TabProcs]]
	badge := m.st.Faint.Render(fmt.Sprintf("%d shown · sort:%s", n, sortName))
	if m.orphanOnly {
		badge = m.st.badgePill("orphans only", m.st.th.Accent2) + " " + badge
	}

	cols := procColumns(w)
	cells := func(i int) []cellSpec {
		p := rows[i]
		cpuSty := m.pctStyle(p.CPUPercent)
		cmd := p.CmdLine
		if cmd == "" {
			cmd = "[" + p.Name + "]"
		}
		c := []cellSpec{
			{fmt.Sprint(p.Pid), m.st.Muted, true},
			{fmt.Sprintf("%.1f", p.CPUPercent), cpuSty, true},
			{engine.Bytes(p.RSS), m.st.Text, true},
			{engine.Seconds(p.AgeSec), m.st.Faint, true},
			{p.User, m.st.Muted, false},
			{p.Name, m.st.Text, false},
			{cmd, m.st.Muted, false},
		}
		if p.Orphan {
			c[5] = cellSpec{"⚠ " + p.Name, m.st.Warn, false}
		}
		return c
	}
	selStyle := func(sel bool) lipgloss.Style {
		if sel {
			return m.st.RowSel
		}
		return m.st.Row
	}
	// selection highlight spans whole row; flash overrides
	out := m.renderList(w, h, "processes", badge, cols, cells, n, selStyle)
	return out
}

// ---- view: ports ----

func (m *model) viewPorts(w, h int) string {
	rows := m.filteredPorts()
	n := len(rows)
	ok := 0
	for _, p := range rows {
		if p.HTTP != nil && p.HTTP.OK {
			ok++
		}
	}
	badge := m.st.Faint.Render(fmt.Sprintf("%d listeners · %d http up", n, ok))

	cols := flexCols([]column{
		{"PORT", 6, false, true},
		{"PROTO", 5, false, false},
		{"BIND", 13, false, false},
		{"STATE", 9, false, false},
		{"PID", 6, false, true},
		{"PROCESS", 14, false, false},
		{"WATCH", 5, false, false},
		{"HTTP HEALTH", 0, true, false},
	}, max(w-4, 20)-1)

	cells := func(i int) []cellSpec {
		p := rows[i]
		httpTxt := ""
		httpSty := m.st.Faint
		if p.HTTP != nil {
			if p.HTTP.OK {
				httpTxt = fmt.Sprintf("● %d  %.1fms  %s", p.HTTP.Status, p.HTTP.LatencyMs, urlOf(p))
				httpSty = m.st.Good
			} else {
				httpTxt = "○ no http"
			}
		}
		watchTxt := ""
		if p.Monitored {
			watchTxt = m.st.Acc2.Render("★")
		}
		stateSty := m.st.Muted
		if p.State != "LISTEN" {
			stateSty = m.st.Faint
		}
		c := []cellSpec{
			{fmt.Sprint(p.Port), m.st.Acc, true},
			{p.Proto, m.st.Faint, false},
			{p.Addr, m.st.Muted, false},
			{p.State, stateSty, false},
			{pidLabel(p.Pid), m.st.Muted, true},
			{p.ProcName, m.st.Text, false},
			{watchTxt, lipgloss.Style{}, false},
			{httpTxt, httpSty, false},
		}
		return c
	}
	return m.renderList(w, h, "ports · localhost map", badge, cols, cells, n, func(sel bool) lipgloss.Style {
		if sel {
			return m.st.RowSel
		}
		return m.st.Row
	})
}

func hostOf(p engine.Port) string {
	if p.Addr == "" || p.Addr == "—" {
		return "127.0.0.1"
	}
	return p.Addr
}

// urlOf mirrors the address the probe actually hit, so a displayed URL can be
// pasted into a browser and work.
func urlOf(p engine.Port) string {
	h := hostOf(p)
	if strings.ContainsRune(h, ':') {
		h = "[" + h + "]"
	}
	return fmt.Sprintf("%s://%s:%d", p.HTTP.Scheme, h, p.Port)
}

// displayPath keeps long artifact paths readable by anchoring at the home dir.
func displayPath(p string) string {
	if h := homeDir(); h != "" && strings.HasPrefix(p, h+"/") {
		return "~" + p[len(h):]
	}
	return p
}

// Root-owned listeners hide their PID from unprivileged /proc scans.
func pidLabel(pid int32) string {
	if pid == 0 {
		return "—"
	}
	return fmt.Sprint(pid)
}

// ---- view: docker ----

func (m *model) viewDocker(w, h int) string {
	rows := m.filteredDocker()
	n := len(rows)
	badge := ""
	if m.snap.DockerErr != "" {
		badge = m.st.badgePill("socket unavailable", m.st.th.Bad)
	} else {
		run := 0
		for _, c := range m.snap.Containers {
			if c.State == "running" {
				run++
			}
		}
		badge = m.st.Faint.Render(fmt.Sprintf("%d/%d running", run, len(m.snap.Containers)))
	}

	cols := flexCols([]column{
		{"STATE", 9, false, false},
		{"NAME", 18, false, false},
		{"IMAGE", 20, false, false},
		{"CPU%", 5, false, true},
		{"MEM", 6, false, true},
		{"MEM BAR", 0, true, false},
	}, max(w-4, 20)-1)

	cells := func(i int) []cellSpec {
		c := rows[i]
		state := c.State
		sty := m.st.Muted
		switch state {
		case "running":
			sty = m.st.Good
		case "exited", "dead":
			sty = m.st.Bad
		case "paused", "created", "restarting":
			sty = m.st.Warn
		}
		var memBar string
		if c.MemLimit > 0 {
			pct := float64(c.MemUsage) / float64(c.MemLimit) * 100
			memBar = gaugeBar(pct, 16, m.st) + m.st.Faint.Render(fmt.Sprintf(" %s/%s",
				engine.Bytes(c.MemUsage), engine.Bytes(c.MemLimit)))
		}
		return []cellSpec{
			{"● " + state, sty, false},
			{c.Name, m.st.Text, false},
			{c.Image, m.st.Muted, false},
			{fmt.Sprintf("%.1f", c.CPUPercent), m.pctStyle(c.CPUPercent), true},
			{engine.Bytes(c.MemUsage), m.st.Muted, true},
			{memBar, lipgloss.Style{}, false},
		}
	}
	return m.renderList(w, h, "containers · "+socketLabel(m.snap.DockerErr), badge, cols, cells, n, func(sel bool) lipgloss.Style {
		if sel {
			return m.st.RowSel
		}
		return m.st.Row
	})
}

func socketLabel(err string) string {
	if err != "" {
		return "runtime"
	}
	return "runtime socket"
}

// ---- view: disk ----

func (m *model) viewDisk(w, h int) string {
	rows := m.filteredArtifacts()
	n := len(rows)
	var total uint64
	for _, a := range rows {
		total += a.Size
	}
	badge := m.st.Faint.Render(fmt.Sprintf("%d dirs · %s reclaimable", n, engine.Bytes(total)))
	if m.snap.ScanState.Scanning {
		badge = m.st.Acc.Render(m.spinFrame() + fmt.Sprintf(" scanning · %d dirs seen", m.snap.ScanState.DirsSeen))
	} else if !m.snap.ScanState.LastDoneAt.IsZero() {
		badge += m.st.Faint.Render(" · r rescan")
	}

	cols := flexCols([]column{
		{"SIZE", 9, false, true},
		{"KIND", 12, false, false},
		{"FILES", 7, false, true},
		{"PROJECT", 14, false, false},
		{"PATH", 0, true, false},
	}, max(w-4, 20)-1)

	maxSize := uint64(1)
	for _, a := range m.snap.Artifacts {
		if a.Size > maxSize {
			maxSize = a.Size
		}
	}
	sortLabel := []string{"size", "path", "kind"}[m.sortMode[TabDisk]]

	cells := func(i int) []cellSpec {
		a := rows[i]
		return []cellSpec{
			{engine.Bytes(a.Size), m.st.Acc, true},
			{a.Kind, m.st.Acc2, false},
			{fmt.Sprint(a.Files), m.st.Muted, true},
			{a.Project, m.st.Muted, false},
			{displayPath(a.Path), m.st.Faint, false},
		}
	}
	return m.renderList(w, h, "build artifacts", badge+m.st.Faint.Render(" · sort:"+sortLabel), cols, cells, n, func(sel bool) lipgloss.Style {
		if sel {
			return m.st.RowSel
		}
		return m.st.Row
	})
}

func homeDir() string {
	if homeOnce == "" {
		if h, err := os.UserHomeDir(); err == nil {
			homeOnce = h
		} else {
			homeOnce = "~"
		}
	}
	return homeOnce
}

var homeOnce string
