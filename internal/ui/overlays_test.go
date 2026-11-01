package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/gaurav-dhakal/vibe-check/internal/config"
	"github.com/gaurav-dhakal/vibe-check/internal/engine"
	"github.com/gaurav-dhakal/vibe-check/internal/theme"
)

func newTestModel(w, h int) *model {
	cfg := config.Default()
	th := theme.Default(cfg.Theme)
	m := New(nil, cfg, th, cfg.Theme)
	m.width, m.height = w, h
	m.snap = engine.Snapshot{Sys: engine.System{Hostname: "test", NumCPU: 8, OS: "linux"}}
	return m
}

// filler base so any box glyph in the output must come from the overlay.
func fillerFrame(w, h int) string {
	var b strings.Builder
	for y := 0; y < h; y++ {
		if y > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.Repeat(".", w))
	}
	return b.String()
}

func colOf(t *testing.T, line string, glyph rune) int {
	t.Helper()
	col := 0
	for _, r := range line {
		if r == '\x1b' {
			break
		}
		if r == glyph {
			return col
		}
		col += runeWidth(r)
	}
	return -1
}

func checkOverlay(t *testing.T, frame string, w, h int) {
	t.Helper()
	lines := strings.Split(frame, "\n")
	if len(lines) != h {
		t.Fatalf("frame height %d, want %d", len(lines), h)
	}
	var left, right, top int = -1, -1, -1
	seen := 0
	for i, l := range lines {
		if dw := lipgloss.Width(l); dw > w {
			t.Errorf("line %d overflows: width %d > %d", i, dw, w)
		}
		for _, g := range []rune{'╭', '╮', '│', '▓', '▄'} {
			c := colOf(t, l, g)
			switch g {
			case '╭':
				if c >= 0 {
					top = i
					left = c
					seen++
				}
			case '╮':
				if c >= 0 {
					right = c
					seen++
				}
			case '│':
				if c >= 0 && left >= 0 && c != left {
					t.Errorf("line %d: left edge at %d, corner at %d", i, c, left)
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("overlay painted no box glyphs")
	}
	if left < 1 {
		t.Errorf("modal starts at column %d, want centered", left)
	}
	if right > 0 && right-left < 20 {
		t.Errorf("modal only %d columns wide", right-left)
	}
	if top > h/2 {
		t.Errorf("modal top row %d of %d, not centered", top, h)
	}
	fmt.Printf("overlay: left col %d, width %d, top row %d\n", left, right-left, top)
}

func TestOverlayPaletteGeometry(t *testing.T) {
	m := newTestModel(120, 40)
	m.paletteOn = true
	checkOverlay(t, m.overlayPalette(fillerFrame(120, 40)), 120, 40)
}

func TestOverlayHelpGeometry(t *testing.T) {
	m := newTestModel(120, 40)
	m.helpOn = true
	checkOverlay(t, m.overlayHelp(fillerFrame(120, 40)), 120, 40)
}

func TestOverlayNarrowTerminal(t *testing.T) {
	m := newTestModel(60, 20)
	m.helpOn = true
	checkOverlay(t, m.overlayHelp(fillerFrame(60, 20)), 60, 20)
}
