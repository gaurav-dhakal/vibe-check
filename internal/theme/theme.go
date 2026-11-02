package theme

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/muesli/termenv"
)

// Depth describes the terminal's usable color range.
type Depth int

const (
	Depth16 Depth = iota
	Depth256
	DepthTrue
)

func (d Depth) String() string {
	switch d {
	case DepthTrue:
		return "truecolor"
	case Depth256:
		return "256-color"
	default:
		return "16-color"
	}
}

// Detect inspects COLORTERM / TERM / TUI env vars the way termenv does,
// but with explicit truecolor checks so we degrade gracefully.
func Detect() Depth {
	if v := os.Getenv("COLORTERM"); v == "truecolor" || v == "24bit" {
		return DepthTrue
	}
	t := os.Getenv("TERM")
	if strings.Contains(t, "256") {
		return Depth256
	}
	if strings.Contains(t, "color") || os.Getenv("TUI") == "color" {
		return Depth16
	}
	if termenv.EnvColorProfile() == termenv.TrueColor {
		return DepthTrue
	}
	return Depth16
}

// Palette is a full 60-30-10 color scheme. Every field is a hex string.
type Palette struct {
	Name    string
	Bg      string // 60% base
	Panel   string
	Text    string // 30% secondary text
	Muted   string
	Faint   string // 30% structural: inactive borders
	Border  string
	Accent  string // 10%: focused borders, selection
	Accent2 string
	Good    string
	Warn    string
	Bad     string
	Info    string
}

var palettes = map[string]Palette{
	"catppuccin": {
		Name: "Catppuccin Mocha", Bg: "#1e1e2e", Panel: "#181825",
		Text: "#cdd6f4", Muted: "#a6adc8", Faint: "#45475a", Border: "#313244",
		Accent: "#89b4fa", Accent2: "#cba6f7", Good: "#a6e3a1",
		Warn: "#f9e2af", Bad: "#f38ba8", Info: "#89dceb",
	},
	"tokyonight": {
		Name: "Tokyo Night", Bg: "#1a1b26", Panel: "#16161e",
		Text: "#c0caf5", Muted: "#7c83a8", Faint: "#3b4261", Border: "#414868",
		Accent: "#7aa2f7", Accent2: "#bb9af7", Good: "#9ece6a",
		Warn: "#e0af68", Bad: "#f7768e", Info: "#7dcfff",
	},
	"gruvbox": {
		Name: "Gruvbox Dark", Bg: "#282828", Panel: "#1d2021",
		Text: "#ebdbb2", Muted: "#a89984", Faint: "#504945", Border: "#665c54",
		Accent: "#fe8019", Accent2: "#d38696", Good: "#b8bb26",
		Warn: "#fabd2f", Bad: "#fb4934", Info: "#8ec07c",
	},
	"nord": {
		Name: "Nord Snow Storm", Bg: "#2e3440", Panel: "#272c36",
		Text: "#e5e9f0", Muted: "#a5b1c2", Faint: "#4c566a", Border: "#3b4251",
		Accent: "#88c0d0", Accent2: "#b48ead", Good: "#a3be8c",
		Warn: "#ebcb8b", Bad: "#bf616a", Info: "#81a1c1",
	},
	"dracula": {
		Name: "Dracula", Bg: "#282a36", Panel: "#21222c",
		Text: "#f8f8f2", Muted: "#cfd0e0", Faint: "#6272a4", Border: "#44475a",
		Accent: "#bd93f9", Accent2: "#ff79c6", Good: "#50fa7b",
		Warn: "#f1fa8c", Bad: "#ff5555", Info: "#8be9fd",
	},
	"solarized": {
		Name: "Solarized Dark", Bg: "#002b36", Panel: "#073642",
		Text: "#93a1a1", Muted: "#839496", Faint: "#586e75", Border: "#0e4150",
		Accent: "#268bd2", Accent2: "#d33682", Good: "#859900",
		Warn: "#b58900", Bad: "#dc322f", Info: "#2aa198",
	},
}

func Names() []string {
	return []string{"catppuccin", "tokyonight", "gruvbox", "nord", "dracula", "solarized"}
}

// DisplayName is the human label shown in the first-run menu and overlays.
func DisplayName(name string) string {
	if p, ok := palettes[name]; ok {
		return p.Name
	}
	return name
}

func Exists(name string) bool { _, ok := palettes[name]; return ok }

// Theme holds profile-adapted colors. Each color string is ready to hand
// to lipgloss: hex stays hex on truecolor, else nearest 256/16 index.
type Theme struct {
	Name    string
	Depth   Depth
	Bg      string
	Panel   string
	Text    string
	Muted   string
	Faint   string
	Border  string
	Accent  string
	Accent2 string
	Good    string
	Warn    string
	Bad     string
	Info    string
}

func New(name string, depth Depth) *Theme {
	p, ok := palettes[name]
	if !ok {
		p = palettes["catppuccin"]
	}
	adapt := func(hex string) string { return adaptColor(hex, depth) }
	return &Theme{
		Name: p.Name, Depth: depth,
		Bg: adapt(p.Bg), Panel: adapt(p.Panel), Text: adapt(p.Text),
		Muted: adapt(p.Muted), Faint: adapt(p.Faint), Border: adapt(p.Border),
		Accent: adapt(p.Accent), Accent2: adapt(p.Accent2),
		Good: adapt(p.Good), Warn: adapt(p.Warn), Bad: adapt(p.Bad),
		Info: adapt(p.Info),
	}
}

// Default picks the theme honoring NO_COLOR and the detected depth.
func Default(name string) *Theme {
	if os.Getenv("NO_COLOR") != "" {
		d := Depth16
		return &Theme{Name: "monochrome", Depth: d,
			Bg: "0", Panel: "0", Text: "15", Muted: "7", Faint: "8",
			Border: "8", Accent: "15", Accent2: "7", Good: "15",
			Warn: "7", Bad: "7", Info: "15"}
	}
	return New(name, Detect())
}

func adaptColor(hex string, d Depth) string {
	if d == DepthTrue {
		return hex
	}
	r, g, b := parseHex(hex)
	if d == Depth16 {
		return strconv.Itoa(nearest16(r, g, b))
	}
	return strconv.Itoa(nearest256(r, g, b))
}

func parseHex(hex string) (int, int, int) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 255, 255, 255
	}
	v, _ := strconv.ParseInt(hex, 16, 32)
	return int(v>>16) & 0xff, int(v>>8) & 0xff, int(v) & 0xff
}

// nearest256 snaps rgb to the 6x6x6 cube (or grayscale ramp) and returns
// the xterm-256 index, chosen by lowest weighted euclidean distance.
func nearest256(r, g, b int) int {
	cubeIdx := func(v int) int {
		if v < 48 {
			return 0
		}
		if v < 115 {
			return 1
		}
		return (v - 35) / 40
	}
	cr, cg, cb := cubeIdx(r), cubeIdx(g), cubeIdx(b)
	cubeVal := func(i int) int {
		if i == 0 {
			return 0
		}
		if i == 1 {
			return 95
		}
		return 55 + i*40
	}
	best, bestD := 16+36*cr+6*cg+cb, dist(r, g, b, cubeVal(cr), cubeVal(cg), cubeVal(cb))
	// grayscale ramp 232..255
	q := (r + g + b) / 3
	gi := (q - 8) * 24 / 247
	if gi < 0 {
		gi = 0
	}
	if gi > 23 {
		gi = 23
	}
	gv := 8 + gi*10
	if d := dist(r, g, b, gv, gv, gv); d < bestD {
		best, bestD = 232+gi, d
	}
	_ = bestD
	return best
}

// 16-color base values, indices 0..15 (bright colors at 8..15).
var base16 = [16][3]int{
	{0, 0, 0}, {128, 0, 0}, {0, 128, 0}, {128, 128, 0},
	{0, 0, 128}, {128, 0, 128}, {0, 128, 128}, {192, 192, 192},
	{128, 128, 128}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
	{0, 0, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
}

func nearest16(r, g, b int) int {
	best, bestD := 7, 1<<30
	for i, c := range base16 {
		if d := dist(r, g, b, c[0], c[1], c[2]); d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// Human-friendly distance weighting perceptual-ish.
func dist(r1, g1, b1, r2, g2, b2 int) int {
	dr, dg, db := r1-r2, g1-g2, b1-b2
	return 2*dr*dr + 4*dg*dg + 3*db*db
}

func (t *Theme) Describe() string {
	return fmt.Sprintf("%s · %s", t.Name, t.Depth)
}
