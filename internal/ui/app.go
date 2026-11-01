package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gaurav-dhakal/vibe-check/internal/config"
	"github.com/gaurav-dhakal/vibe-check/internal/engine"
	"github.com/gaurav-dhakal/vibe-check/internal/theme"
	"github.com/sahilm/fuzzy"
)

type rect struct{ x, y, w, h int }

type Tab int

const (
	TabDashboard Tab = iota
	TabProcs
	TabPorts
	TabDocker
	TabDisk
	TabCount
)

var tabNames = [TabCount]string{"dashboard", "processes", "ports", "docker", "disk"}

type frameMsg struct{}
type eventMsg struct{ ev engine.Event }

// EventMsg wraps an engine telemetry event for the bubbletea message loop.
func EventMsg(ev engine.Event) tea.Msg { return eventMsg{ev: ev} }

type toastState struct {
	text    string
	level   string
	expires time.Time
}

type confirmState struct {
	title  string
	detail string
	onYes  func(m *model)
}

// geometry records clickable rects (screen coords) from the last render so
// mouse hits can be resolved in the next Update.
type geometry struct {
	tabBtns    []rect
	contentTop int
	contentH   int
	rowTop     int // first data row of a list tab (screen y)
	rowH       int
	panes      []rect // dashboard grid cells
	listRows   int
}

type model struct {
	eng *engine.Engine
	cfg config.Config
	th  *theme.Theme
	st  *Styles
	key string // theme key for cycling

	width, height int
	frame         time.Duration
	snap          engine.Snapshot
	start         time.Time

	tab     Tab
	paneSel int
	zoomed  bool

	sel      [TabCount]int
	off      [TabCount]int
	sortMode [TabCount]int

	orphanOnly bool

	searchOn bool
	search   textinput.Model
	filter   string

	paletteOn bool
	palette   textinput.Model
	palSel    int

	helpOn  bool
	confirm *confirmState

	toast    toastState
	flashPID map[int32]time.Time
	flashDir map[string]time.Time

	geo *geometry

	procsKey  filterKey
	procsVal  []engine.Proc
	portsKey  filterKey
	portsVal  []engine.Port
	dockerKey filterKey
	dockerVal []engine.Container
	diskKey   filterKey
	diskVal   []engine.Artifact
}

type filterKey struct {
	at      time.Time
	q       string
	mode    int
	orphans bool
}

func New(eng *engine.Engine, cfg config.Config, th *theme.Theme, themeKey string) *model {
	ti := textinput.New()
	ti.Prompt = "❯ "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(th.Accent))
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(th.Text))
	ti.CursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(th.Accent)).Bold(true)
	ti.Placeholder = "filter…"
	ti.CharLimit = 96

	pa := textinput.New()
	pa.Prompt = "▸ "
	pa.PromptStyle = ti.PromptStyle
	pa.TextStyle = ti.TextStyle
	pa.CursorStyle = ti.CursorStyle
	pa.Placeholder = "type a command…"
	pa.CharLimit = 64

	return &model{
		eng: eng, cfg: cfg, th: th, key: themeKey, st: NewStyles(th),
		frame:    time.Second / time.Duration(max(10, cfg.FPS)),
		search:   ti,
		palette:  pa,
		start:    time.Now(),
		flashPID: map[int32]time.Time{},
		flashDir: map[string]time.Time{},
		geo:      &geometry{rowH: 1},
	}
}

// spinFrame returns a braille spinner char driven by the wall clock so the
// UI thread never needs its own animation ticker.
func (m *model) spinFrame() string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	return m.st.Acc.Render(frames[int(time.Since(m.start)/(80*time.Millisecond))%len(frames)])
}

func (m *model) Init() tea.Cmd { return m.scheduleFrame() }

// scheduleFrame keeps the render loop honest: full 60 FPS only while something
// is actually animating (spinner, toast, flash, typing). Otherwise it idles at
// telemetry cadence, so a quiet dashboard costs almost no CPU.
func (m *model) scheduleFrame() tea.Cmd {
	d := 250 * time.Millisecond
	if m.animating() {
		d = m.frame
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return frameMsg{} })
}

func (m *model) animating() bool {
	return m.busy() || m.toast.text != "" || len(m.flashPID) > 0 || len(m.flashDir) > 0 ||
		m.searchOn || m.paletteOn || m.helpOn || m.confirm != nil
}

// ---- update ----

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	self := m // pointer receiver model; bubbletea stores the returned Model
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return self, nil

	case frameMsg:
		m.snap = m.eng.Snapshot()
		m.pruneFlash()
		if !m.toast.expires.IsZero() && time.Now().After(m.toast.expires) {
			m.toast = toastState{}
		}
		m.clampSelection()
		return self, m.scheduleFrame()

	case eventMsg:
		m.setToast(msg.ev.Text, msg.ev.Level)
		return self, nil

	case tea.MouseMsg:
		m.handleMouse(msg)
		return self, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return self, nil
}

func (m *model) clampSelection() {
	n := m.listLen()
	if n == 0 {
		m.sel[m.tab], m.off[m.tab] = 0, 0
		return
	}
	if m.sel[m.tab] > n-1 {
		m.sel[m.tab] = n - 1
	}
	if m.sel[m.tab] < 0 {
		m.sel[m.tab] = 0
	}
	vis := m.pageSize()
	if m.off[m.tab] > m.sel[m.tab] {
		m.off[m.tab] = m.sel[m.tab]
	}
	if m.off[m.tab]+vis <= m.sel[m.tab] && m.off[m.tab]+vis <= n-1 {
		m.off[m.tab] = m.sel[m.tab] - vis + 1
	}
}

func (m *model) setToast(text, level string) {
	dur := 4 * time.Second
	if level == "error" {
		dur = 7 * time.Second
	}
	m.toast = toastState{text: text, level: level, expires: time.Now().Add(dur)}
}

func (m *model) busy() bool { return m.snap.ScanState.Scanning }

func (m *model) pruneFlash() {
	now := time.Now()
	for pid, t := range m.flashPID {
		if now.Sub(t) > 900*time.Millisecond {
			delete(m.flashPID, pid)
		}
	}
	for p, t := range m.flashDir {
		if now.Sub(t) > 900*time.Millisecond {
			delete(m.flashDir, p)
		}
	}
}

func (m *model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirm != nil {
		switch msg.String() {
		case "y", "Y", "enter":
			fn := m.confirm.onYes
			m.confirm = nil
			if fn != nil {
				fn(m)
			}
		case "n", "N", "esc", "ctrl+c":
			m.confirm = nil
		}
		return m, nil
	}
	if m.paletteOn {
		return m.handlePaletteKey(msg)
	}
	if m.searchOn {
		return m.handleSearchKey(msg)
	}
	if m.helpOn {
		m.helpOn = false
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?", "f1":
		m.helpOn = true
		return m, nil
	case "ctrl+p":
		m.paletteOn = true
		m.palette.SetValue("")
		m.palSel = 0
		return m, m.palette.Focus()
	case "/":
		m.searchOn = true
		m.search.SetValue("")
		m.filter = ""
		return m, m.search.Focus()

	case "1", "2", "3", "4", "5":
		m.tab = Tab(int(msg.String()[0]-'0') - 1)
		m.zoomed = false
		return m, nil

	case "right", "l":
		if m.tab == TabDashboard {
			m.paneSel = (m.paneSel + 1) % 4
		} else {
			m.tab = nextTab(m.tab, 1)
		}
		return m, nil
	case "left", "h":
		if m.tab == TabDashboard {
			m.paneSel = (m.paneSel + 3) % 4
		} else {
			m.tab = nextTab(m.tab, -1)
		}
		return m, nil
	case "down", "j":
		m.move(1)
		return m, nil
	case "up", "k":
		m.move(-1)
		return m, nil
	case "g":
		m.sel[m.tab] = 0
		m.off[m.tab] = 0
		return m, nil
	case "G":
		m.sel[m.tab] = m.listLen() - 1
		m.scrollToSel()
		return m, nil
	case "ctrl+d":
		m.move(m.pageSize() / 2)
		return m, nil
	case "ctrl+u":
		m.move(-m.pageSize() / 2)
		return m, nil

	case "f", " ":
		m.zoomed = !m.zoomed
		return m, nil
	case "esc":
		if m.zoomed {
			m.zoomed = false
		} else if m.filter != "" {
			m.filter = ""
		}
		return m, nil

	case "r":
		if m.tab == TabDisk {
			m.eng.Rescan()
			m.setToast("rescan queued…", "info")
		} else {
			m.setToast("telemetry is live — all sources stream continuously", "info")
		}
		return m, nil

	case "t":
		names := theme.Names()
		next := names[0]
		for i, n := range names {
			if n == m.key {
				next = names[(i+1)%len(names)]
				break
			}
		}
		m.key = next
		m.th = theme.New(next, m.th.Depth)
		m.st = NewStyles(m.th)
		m.setToast("theme → "+m.th.Name, "info")
		return m, nil

	case "y":
		return m.copyRow(false), nil
	case "Y":
		return m.copyRow(true), nil

	case "s":
		if m.tab == TabProcs || m.tab == TabDisk {
			m.sortMode[m.tab] = (m.sortMode[m.tab] + 1) % 3
			m.sel[m.tab] = 0
		}
		return m, nil
	case "o":
		if m.tab == TabProcs {
			m.orphanOnly = !m.orphanOnly
			m.sel[m.tab] = 0
		}
		return m, nil

	case "x":
		m.act(false)
		return m, nil
	case "X":
		m.act(true)
		return m, nil
	}
	return m, nil
}

func nextTab(t Tab, d int) Tab {
	i := (int(t) + d)
	if i <= 0 {
		i = int(TabDisk)
	}
	if i > int(TabDisk) {
		i = 1
	}
	return Tab(i)
}

func (m *model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchOn = false
		m.search.Blur()
		m.filter = ""
		return m, nil
	case "enter":
		m.searchOn = false
		m.search.Blur()
		return m, nil
	}
	var c tea.Cmd
	m.search, c = m.search.Update(msg)
	m.filter = m.search.Value()
	m.sel[m.tab] = 0
	m.off[m.tab] = 0
	return m, c
}

func (m *model) handlePaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmds := m.paletteCommands(m.palette.Value())
	switch msg.String() {
	case "esc", "ctrl+c":
		m.paletteOn = false
		m.palette.Blur()
		return m, nil
	case "enter":
		m.paletteOn = false
		m.palette.Blur()
		if len(cmds) > 0 {
			i := clamp(m.palSel, 0, len(cmds)-1)
			cmds[i].run(m)
		}
		return m, nil
	case "up", "k":
		if m.palSel > 0 {
			m.palSel--
		}
		return m, nil
	case "down", "j":
		if m.palSel < len(cmds)-1 {
			m.palSel++
		}
		return m, nil
	}
	var c tea.Cmd
	m.palette, c = m.palette.Update(msg)
	m.palSel = 0
	return m, c
}

// ---- shared list behavior ----

func (m *model) listLen() int {
	switch m.tab {
	case TabProcs:
		return len(m.filteredProcs())
	case TabPorts:
		return len(m.filteredPorts())
	case TabDocker:
		return len(m.filteredDocker())
	case TabDisk:
		return len(m.filteredArtifacts())
	}
	return 0
}

func (m *model) pageSize() int { return max(3, m.innerHeight()) }

func (m *model) move(d int) {
	n := m.listLen()
	if n == 0 {
		return
	}
	m.sel[m.tab] = clamp(m.sel[m.tab]+d, 0, n-1)
	m.scrollToSel()
}

func (m *model) scrollToSel() {
	vis := m.pageSize()
	if m.sel[m.tab] < m.off[m.tab] {
		m.off[m.tab] = m.sel[m.tab]
	}
	if m.sel[m.tab] >= m.off[m.tab]+vis {
		m.off[m.tab] = m.sel[m.tab] - vis + 1
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// fuzzyKeep filters items keeping original order.
func fuzzyKeep[T any](q string, items []T, key func(T) string) []T {
	if q == "" {
		return items
	}
	strs := make([]string, len(items))
	for i, it := range items {
		strs[i] = key(it)
	}
	matches := fuzzy.Find(strings.ToLower(q), strs)
	hit := make([]bool, len(items))
	for _, mm := range matches {
		hit[mm.Index] = true
	}
	out := make([]T, 0, len(matches))
	for i, it := range items {
		if hit[i] {
			out = append(out, it)
		}
	}
	return out
}

// ---- actions ----

func (m *model) selectedIndex() int {
	return clamp(m.sel[m.tab], 0, max(0, m.listLen()-1))
}

func (m *model) copyRow(full bool) *model {
	text := ""
	switch m.tab {
	case TabDashboard:
		b, _ := json.MarshalIndent(m.snap, "", "  ")
		text = string(b)
	default:
		rows := m.rawRow(m.selectedIndex())
		if full {
			b, err := json.Marshal(rows)
			if err == nil {
				text = string(b)
			}
		} else {
			text = m.rowIdentity(m.selectedIndex())
		}
	}
	if text == "" {
		m.setToast("nothing selected to copy", "warn")
		return m
	}
	go func() { _, _ = engine.Copy(text) }()
	m.setToast("✔ copied → "+clipPreview(text), "ok")
	return m
}

func clipPreview(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 44 {
		return s[:44] + "…"
	}
	return s
}

func (m *model) act(force bool) {
	switch m.tab {
	case TabProcs:
		procs := m.filteredProcs()
		if len(procs) == 0 {
			return
		}
		p := procs[m.selectedIndex()]
		m.flashPID[p.Pid] = time.Now()
		go m.eng.Kill(p.Pid, force)
		label := "SIGTERM"
		if force {
			label = "SIGKILL"
		}
		m.setToast(fmt.Sprintf("⚡ %s → %s (%d)", label, p.Name, p.Pid), "warn")

	case TabDocker:
		cs := m.filteredDocker()
		if len(cs) == 0 {
			return
		}
		c := cs[m.selectedIndex()]
		if c.State == "running" {
			go m.eng.StopContainer(c.ID)
			m.setToast("stopping "+c.Name+"…", "warn")
		} else {
			go m.eng.StartContainer(c.ID)
			m.setToast("starting "+c.Name+"…", "info")
		}

	case TabDisk:
		arts := m.filteredArtifacts()
		if len(arts) == 0 {
			return
		}
		a := arts[m.selectedIndex()]
		m.confirm = &confirmState{
			title:  "purge artifact directory",
			detail: fmt.Sprintf("%s\n%s · %d files — this deletes for real.", a.Path, engine.Bytes(a.Size), a.Files),
			onYes: func(mm *model) {
				mm.flashDir[a.Path] = time.Now()
				go mm.eng.Purge(a.Path)
			},
		}
	}
}

// ---- view ----

func (m *model) View() string {
	if m.width < 20 || m.height < 8 {
		return "▮ vibe-check · waiting for terminal size…"
	}
	if m.snap.Sys.Hostname == "" {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			m.spinFrame()+"\n\n▮ vibe-check · warming up telemetry…")
	}

	m.resetGeo()

	if m.zoomed {
		return m.viewZoomed()
	}

	contentW := m.width
	contentH := m.height - m.headerH() - m.tabBarH() - 1 // minus footer

	body := m.viewTab(contentW, contentH)

	parts := make([]string, 0, 4)
	parts = append(parts, m.renderHeader())
	parts = append(parts, m.renderTabs())
	parts = append(parts, body)
	parts = append(parts, m.renderFooter())
	out := lipgloss.JoinVertical(lipgloss.Left, parts...)

	if m.helpOn {
		out = m.overlayHelp(out)
	} else if m.paletteOn {
		out = m.overlayPalette(out)
	} else if m.confirm != nil {
		out = m.overlayConfirm(out)
	}
	return out
}

func (m *model) viewTab(w, h int) string {
	switch m.tab {
	case TabProcs:
		return m.viewProcs(w, h)
	case TabPorts:
		return m.viewPorts(w, h)
	case TabDocker:
		return m.viewDocker(w, h)
	case TabDisk:
		return m.viewDisk(w, h)
	}
	return m.viewDashboard(w, h)
}

func (m *model) headerH() int { return 1 }
func (m *model) tabBarH() int { return 1 }

func (m *model) innerHeight() int {
	h := m.height - m.headerH() - m.tabBarH() - 1 - 2
	if m.zoomed {
		h = m.height - 1 - 2
	}
	return max(4, h)
}

func (m *model) resetGeo() {
	m.geo = &geometry{rowH: 1, contentTop: m.headerH() + m.tabBarH()}
}

func (m *model) viewZoomed() string {
	h := m.height - 1
	w := m.width
	body := m.viewTab(w, h)
	parts := []string{body, m.renderFooter()}
	out := lipgloss.JoinVertical(lipgloss.Left, parts...)
	if m.helpOn {
		out = m.overlayHelp(out)
	} else if m.paletteOn {
		out = m.overlayPalette(out)
	} else if m.confirm != nil {
		out = m.overlayConfirm(out)
	}
	return out
}
