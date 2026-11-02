package cmd

import (
	"io"
	"strings"
	"testing"
)

func TestPromptTheme(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"\n", "catppuccin"},
		{"  \n", "catppuccin"},
		{"1\n", "catppuccin"},
		{"2\n", "tokyonight"},
		{"3\n", "gruvbox"},
		{"gruvbox\n", "gruvbox"},
		{"9\n", "catppuccin"},
		{"neon\n", "catppuccin"},
		{"", ""}, // stdin closed: nobody answered, so write no config
	}
	for _, c := range cases {
		if got := promptTheme(io.Discard, strings.NewReader(c.in)); got != c.want {
			t.Errorf("input %q: got %q, want %q", c.in, got, c.want)
		}
	}
}
