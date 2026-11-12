package ui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/gaurav-dhakal/vibe-check/internal/apps"
	"github.com/gaurav-dhakal/vibe-check/internal/config"
	"github.com/gaurav-dhakal/vibe-check/internal/engine"
	"github.com/gaurav-dhakal/vibe-check/internal/theme"
)

// renderModel builds a frame that looks populated without touching the OS: no
// inventory shell-out, no crawler, no docker socket.
func renderModel(t *testing.T, w, h int) *model {
	t.Helper()
	m := newTestModel(w, h)
	m.snap = engine.Snapshot{
		Sys: engine.System{Hostname: "test", NumCPU: 8, OS: "linux", CPUPercent: 33,
			MemPercent: 55, MemTotal: 8 << 30, MemUsed: 4 << 30, ProcTotal: 250,
			CorePercents: []float64{10, 20, 30, 40, 50, 60, 70, 80},
			OrphanCount:  2, OrphanRSS: 1 << 30},
		Ports:     []engine.Port{{Proto: "tcp", Addr: "127.0.0.1", Port: 3000, State: "LISTEN", Pid: 42, ProcName: "node"}},
		Procs:     []engine.Proc{{Pid: 42, Name: "node", CPUPercent: 12, RSS: 1 << 30, Orphan: true}},
		Artifacts: []engine.Artifact{{Path: "/tmp/x/node_modules", Size: 1 << 30, Kind: "node_modules", Files: 1200}},
	}
	m.appsKey = appsFilterKey{gen: 1}
	m.appsVal = []apps.Package{{Manager: "apt", Name: "vim", Version: "9.1", SizeKB: 3000, Desc: "editor"}}
	return m
}

// TestFrameNeverEscapesTheTerminal is the guard for the small-screen report:
// every tab and every overlay, at sizes a laptop actually uses, must fit the
// box it was drawn in.
func TestFrameNeverEscapesTheTerminal(t *testing.T) {
	sizes := [][2]int{{80, 24}, {60, 20}, {50, 14}, {40, 12}, {120, 40}}
	tabs := []Tab{TabDashboard, TabProcs, TabPorts, TabDocker, TabDisk, TabApps}
	overlays := []struct {
		name string
		on   func(*model)
	}{
		{"none", func(*model) {}},
		{"help", func(m *model) { m.helpOn = true }},
		{"palette", func(m *model) { m.paletteOn = true }},
		{"install", func(m *model) { m.installOn = true }},
		{"files", func(m *model) {
			m.filesOn = true
			m.filesPkg = apps.Package{Manager: "apt", Name: "vim"}
		}},
		{"confirm", func(m *model) {
			m.confirm = &confirmState{title: "uninstall vim?", detail: strings.Repeat("word ", 60)}
		}},
		{"zoomed", func(m *model) { m.zoomed = true }},
	}

	for _, sz := range sizes {
		for _, tab := range tabs {
			for _, ov := range overlays {
				name := tabNames[tab] + "/" + ov.name
				m := renderModel(t, sz[0], sz[1])
				m.tab = tab
				ov.on(m)
				out := m.View()
				if h := lipgloss.Height(out); h > sz[1] {
					t.Errorf("%s at %dx%d: frame is %d rows, want ≤ %d\n%s",
						name, sz[0], sz[1], h, sz[1], stripANSI(out))
					continue
				}
				for i, line := range strings.Split(out, "\n") {
					if w := lipgloss.Width(line); w > sz[0] {
						t.Errorf("%s at %dx%d: row %d is %d columns, want ≤ %d\n%s",
							name, sz[0], sz[1], i+1, w, sz[0], stripANSI(out))
						break
					}
				}
			}
		}
	}
}

// TestFooterKeepsTabActionsAtNarrowWidth: the report was that the apps tab gave
// no way to see, let alone find, uninstall. The bar may wrap, but it must not
// drop the keys that do something.
func TestFooterKeepsTabActionsAtNarrowWidth(t *testing.T) {
	for _, tab := range []Tab{TabApps, TabProcs, TabDisk, TabDashboard} {
		m := renderModel(t, 60, 20)
		m.tab = tab
		var want []string
		switch tab {
		case TabApps:
			want = []string{"uninstall", "install file"}
		case TabProcs:
			want = []string{"kill", "sort"}
		case TabDisk:
			want = []string{"purge", "rescan"}
		case TabDashboard:
			want = []string{"boost", "auto-clean"}
		}
		foot, _ := m.footerBlock()
		text := stripANSI(foot)
		for _, w := range want {
			if !strings.Contains(text, w) {
				t.Errorf("%s footer at 60 columns lost %q:\n%s", tabNames[tab], w, text)
			}
		}
	}
}

// TestAnsiWidthSkipsWholeSequences guards the measurement bug behind most of
// the clipping: a CSI's introducer '[' is not its final byte, so the scan has
// to run on to the terminator instead of stopping after the '['.
func TestAnsiWidthSkipsWholeSequences(t *testing.T) {
	st := NewStyles(theme.Default(config.Default().Theme))
	for _, s := range []string{"hello", "uninstall", "▮ 100% cpu", "1 · 2 · 3"} {
		styled := st.KeyCap.Render(s)
		if got, want := ansiWidth(styled), lipgloss.Width(s); got != want {
			t.Errorf("ansiWidth(styled %q) = %d, want %d", s, got, want)
		}
		if got := ansiWidth(stripANSI(styled)); got != lipgloss.Width(s) {
			t.Errorf("stripANSI(%q) measures %d, want %d", s, got, lipgloss.Width(s))
		}
	}
}

// stripANSI drops escape sequences so failures and keyword checks read as text.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '\x1b' {
			r, n := utf8.DecodeRuneInString(s[i:])
			b.WriteRune(r)
			i += n
			continue
		}
		i++
		if i < len(s) && s[i] == '[' {
			for i++; i < len(s) && !(s[i] >= 0x40 && s[i] <= 0x7e); i++ {
			}
			i++
		} else if i < len(s) && s[i] == ']' {
			for i++; i < len(s) && s[i] != '\x07' && s[i] != '\x1b'; i++ {
			}
		}
	}
	return b.String()
}
