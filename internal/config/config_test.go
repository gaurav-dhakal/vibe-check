package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveThemeKeepsTheRestOfTheFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := InitWithTheme("catppuccin"); err != nil {
		t.Fatal(err)
	}
	p := Path()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// A hand-edit the user cares about must survive a theme switch.
	withComment := strings.Replace(string(data), "theme =", "# my favourite\ntheme =", 1)
	if err := os.WriteFile(p, []byte(withComment), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SaveTheme(p, "nord"); err != nil {
		t.Fatalf("SaveTheme: %v", err)
	}
	out, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `theme = "nord"`) {
		t.Errorf("theme not rewritten:\n%s", s)
	}
	if !strings.Contains(s, "# my favourite") {
		t.Errorf("comment lost:\n%s", s)
	}
	if !strings.Contains(s, "fps") {
		t.Errorf("other keys lost:\n%s", s)
	}
	if strings.Contains(s, "Path") || strings.Contains(s, "path =") {
		t.Errorf("in-memory Path leaked into the file:\n%s", s)
	}

	// Reload must see the new theme.
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "nord" {
		t.Errorf("reloaded theme = %q, want nord", cfg.Theme)
	}
	if cfg.Path != filepath.Clean(p) && cfg.Path != p {
		t.Errorf("cfg.Path = %q, want %q", cfg.Path, p)
	}
}

func TestSaveThemeCreatesMissingFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "vibe-check", "config.toml")
	if err := SaveTheme(p, "dracula"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `theme = "dracula"`) {
		t.Errorf("fresh file lacks the choice:\n%s", data)
	}
}
