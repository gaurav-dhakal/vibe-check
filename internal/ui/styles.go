package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gaurav-dhakal/vibe-check/internal/theme"
)

// Styles precomputes every lipgloss style from the active theme so the
// 60fps render loop never re-derives styles per frame.
type Styles struct {
	Root       lipgloss.Style
	Brand      lipgloss.Style
	BrandDot   lipgloss.Style
	HeaderText lipgloss.Style
	HeaderDim  lipgloss.Style

	TabActive   lipgloss.Style
	TabInactive lipgloss.Style

	PaneActive   lipgloss.Style
	PaneInactive lipgloss.Style

	Row        lipgloss.Style
	RowSel     lipgloss.Style
	RowFlash   lipgloss.Style
	RowKill    lipgloss.Style
	HeadDim    lipgloss.Style
	HeadAccent lipgloss.Style

	Muted lipgloss.Style
	Faint lipgloss.Style
	Text  lipgloss.Style
	Info  lipgloss.Style
	Good  lipgloss.Style
	Warn  lipgloss.Style
	Bad   lipgloss.Style
	Acc   lipgloss.Style
	Acc2  lipgloss.Style

	KVKey   lipgloss.Style
	KVVal   lipgloss.Style
	Modal   lipgloss.Style
	Shadow  lipgloss.Style
	Footer  lipgloss.Style
	KeyCap  lipgloss.Style
	KeyHint lipgloss.Style

	th *theme.Theme
}

func NewStyles(th *theme.Theme) *Styles {
	s := &Styles{th: th}
	col := func(c string) lipgloss.Color { return lipgloss.Color(c) }

	s.Root = lipgloss.NewStyle()
	s.Brand = lipgloss.NewStyle().Bold(true).Foreground(col(th.Text))
	s.BrandDot = lipgloss.NewStyle().Foreground(col(th.Accent))
	s.HeaderText = lipgloss.NewStyle().Foreground(col(th.Text))
	s.HeaderDim = lipgloss.NewStyle().Foreground(col(th.Muted))

	s.TabActive = lipgloss.NewStyle().
		Bold(true).Foreground(col(th.Panel)).Background(col(th.Accent)).
		Padding(0, 1)
	s.TabInactive = lipgloss.NewStyle().
		Foreground(col(th.Muted)).Background(col(th.Border)).
		Padding(0, 1)

	box := func(borderCol string, active bool) lipgloss.Style {
		st := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(col(borderCol)).
			Padding(0, 1)
		if active {
			st = st.Bold(true)
		}
		return st
	}
	s.PaneActive = box(th.Accent, true)
	s.PaneInactive = box(th.Faint, false)

	s.Row = lipgloss.NewStyle().Foreground(col(th.Text))
	s.RowSel = lipgloss.NewStyle().
		Foreground(col(th.Accent)).Bold(true).Background(col(th.Panel))
	s.RowFlash = lipgloss.NewStyle().Foreground(col(th.Bad)).Bold(true)
	s.RowKill = lipgloss.NewStyle().Foreground(col(th.Warn)).Background(col(th.Panel))
	s.HeadDim = lipgloss.NewStyle().Foreground(col(th.Faint)).Bold(true)
	s.HeadAccent = lipgloss.NewStyle().Foreground(col(th.Accent2)).Bold(true)

	s.Muted = lipgloss.NewStyle().Foreground(col(th.Muted))
	s.Faint = lipgloss.NewStyle().Foreground(col(th.Faint))
	s.Text = lipgloss.NewStyle().Foreground(col(th.Text))
	s.Info = lipgloss.NewStyle().Foreground(col(th.Info))
	s.Good = lipgloss.NewStyle().Foreground(col(th.Good))
	s.Warn = lipgloss.NewStyle().Foreground(col(th.Warn))
	s.Bad = lipgloss.NewStyle().Foreground(col(th.Bad))
	s.Acc = lipgloss.NewStyle().Foreground(col(th.Accent))
	s.Acc2 = lipgloss.NewStyle().Foreground(col(th.Accent2))

	s.KVKey = lipgloss.NewStyle().Foreground(col(th.Muted))
	s.KVVal = lipgloss.NewStyle().Foreground(col(th.Text))
	s.Modal = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(col(th.Accent2)).
		Background(col(th.Panel)).
		Padding(1, 2)
	s.Shadow = lipgloss.NewStyle().Foreground(col(th.Faint))
	s.Footer = lipgloss.NewStyle().Foreground(col(th.Muted)).Background(col(th.Border))
	s.KeyCap = lipgloss.NewStyle().Bold(true).Foreground(col(th.Accent))
	s.KeyHint = lipgloss.NewStyle().Foreground(col(th.Muted))

	return s
}

// ---- micro-visual components ----

const sparkChars = "▁▂▃▄▅▆▇█"
const fracChars = " ▏▎▍▌▋▊▉█"

// sparkline renders values across width columns using block chars.
func sparkline(vals []float64, width int, sty lipgloss.Style) string {
	if len(vals) == 0 || width <= 0 {
		return sty.Render(strings.Repeat("▁", min(width, 8)))
	}
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}
	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	span := hi - lo
	var b strings.Builder
	for _, v := range vals {
		idx := 0
		if span > 0 {
			idx = int((v - lo) / span * float64(len(sparkChars)-1))
		} else if v > 0 {
			idx = len(sparkChars) / 2
		}
		if idx >= len(sparkChars) {
			idx = len(sparkChars) - 1
		}
		b.WriteString(string([]rune(sparkChars)[idx]))
	}
	for b.Len() < width*len("█") && len(vals) < width { // pad narrow history
		b.WriteString("▁")
	}
	return sty.Render(b.String())
}

// gaugeBar renders a horizontal percent gauge with eighth-block precision:
// accent fill on a faint ░ track.
func gaugeBar(pct float64, width int, s *Styles) string {
	if width <= 0 {
		return ""
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	cells := pct / 100 * float64(width)
	var b strings.Builder
	full := int(cells)
	frac := cells - float64(full)
	for i := 0; i < full && i < width; i++ {
		b.WriteString("█")
	}
	if full < width {
		if frac > 0.06 {
			idx := int(frac * 8)
			if idx > 7 {
				idx = 7
			}
			r := []rune(fracChars)
			b.WriteString(string(r[idx+1]))
			b.WriteString(s.Faint.Render(strings.Repeat("░", width-full-1)))
		} else {
			b.WriteString(s.Faint.Render(strings.Repeat("░", width-full)))
		}
	}
	out := s.Acc.Render(b.String())
	return out
}

// badge renders a pill badge: bright bg, dark fg.
func badge(text, bg, fg string) string {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(bg)).
		Background(lipgloss.Color(fg)).
		Padding(0, 1).
		Render(text)
}

// softBadge is a subtle inline tag with just a colored fg (no bg).
func (s *Styles) softBadge(mark, color string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render(mark)
}

func (s *Styles) badgePill(text, color string) string {
	return badge(text, color, s.th.Bg)
}

// kvRow renders a dim label + value pair padded to width.
func (s *Styles) kv(key, val string, w int) string {
	k := s.KVKey.Render(key)
	v := s.KVVal.Render(val)
	line := fmt.Sprintf("%s %s", k, v)
	if lipgloss.Width(line) > w {
		return lipgloss.NewStyle().Width(w).MaxWidth(w).Render(line)
	}
	return line
}

// pane wraps content in a rounded, padded section box.
func (s *Styles) pane(title, badgeStr string, w, h int, active bool, body string) string {
	boxStyle := s.PaneInactive
	if active {
		boxStyle = s.PaneActive
	}
	boxStyle = boxStyle.Width(max(0, w-2)).Height(max(0, h-2)).MaxHeight(h)
	return boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		s.paneHeader(title, badgeStr, w-2), body))
}

func (s *Styles) paneHeader(title, badgeStr string, w int) string {
	head := s.HeadAccent.Render(title)
	if badgeStr == "" {
		return head
	}
	avail := w - lipgloss.Width(head) - 1
	if avail < 1 {
		return truncate(head, w)
	}
	if lipgloss.Width(badgeStr) > avail {
		badgeStr = ansiCut(badgeStr, 0, avail-1) + "…"
	}
	gap := avail - lipgloss.Width(badgeStr)
	return head + strings.Repeat(" ", gap+1) + badgeStr
}

// truncate pads or cuts to an exact display width. Cuts go through ansiCut so
// an embedded SGR sequence is never split mid-escape.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	dw := lipgloss.Width(s)
	switch {
	case dw == w:
		return s
	case dw < w:
		return s + strings.Repeat(" ", w-dw)
	default:
		out := ansiCut(s, 0, w-1)
		lw := lipgloss.Width(out)
		if strings.ContainsRune(out, '\x1b') {
			out += "\x1b[0m"
		}
		return out + "…" + strings.Repeat(" ", max(0, w-1-lw))
	}
}

func rightPad(s string, w int) string {
	dw := lipgloss.Width(s)
	if dw >= w {
		return s
	}
	return strings.Repeat(" ", w-dw) + s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
