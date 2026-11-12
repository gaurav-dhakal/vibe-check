package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gaurav-dhakal/vibe-check/internal/apps"
	"github.com/gaurav-dhakal/vibe-check/internal/config"
	"github.com/gaurav-dhakal/vibe-check/internal/engine"
)

func testEngine(t *testing.T) *engine.Engine {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	eng := engine.New(config.Default())
	eng.Run(ctx)
	return eng
}

// seedApps fills the model's filter cache directly: setTab would fire a real
// inventory load, and that shells out for half a second.
func seedApps(m *model, pkgs []apps.Package) {
	m.tab = TabApps
	m.appsKey = appsFilterKey{0, "", 0}
	m.appsVal = pkgs
	m.sel[TabApps] = 0
}

// update drives the model the way bubbletea would; the second return value is
// a Cmd, not an error, so it is dropped rather than checked.
func update(m *model, msg tea.Msg) {
	if _, cmd := m.Update(msg); cmd != nil {
		_ = cmd() // run focus/tick commands so textinput state settles
	}
}

func sendKey(m *model, k string) { update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}) }
func sendEnter(m *model)         { update(m, tea.KeyMsg{Type: tea.KeyEnter}) }
func sendEsc(m *model)           { update(m, tea.KeyMsg{Type: tea.KeyEscape}) }

func TestAppsTabNumberKey(t *testing.T) {
	m := newTestModel(140, 45)
	m.eng = testEngine(t)
	sendKey(m, "6")
	if m.tab != TabApps {
		t.Fatalf("key 6 → tab %d, want apps", m.tab)
	}
	sendKey(m, "1")
	if m.tab != TabDashboard {
		t.Fatalf("key 1 → tab %d, want dashboard", m.tab)
	}
	// On the dashboard, l/h cycle the focused pane, not the tab.
	for i := 0; i < 3; i++ {
		sendKey(m, "l")
	}
	if m.tab != TabDashboard || m.paneSel != 3 {
		t.Fatalf("dashboard l: tab %d pane %d, want 0/3", m.tab, m.paneSel)
	}
}

func TestAppsFilesOverlayAndUninstallKeys(t *testing.T) {
	m := newTestModel(140, 45)
	m.eng = testEngine(t)
	seedApps(m, []apps.Package{
		{Manager: "apt", Name: "vim", Version: "9.1", SizeKB: 3000, Desc: "editor"},
	})

	sendEnter(m)
	if !m.filesOn {
		t.Fatal("enter did not open the files overlay")
	}
	if m.filesPkg.Name != "vim" {
		t.Fatalf("overlay opened for %q", m.filesPkg.Name)
	}
	if v := m.View(); !strings.Contains(v, "files on disk") {
		t.Fatalf("overlay missing its header:\n%s", v)
	}
	skip := m.filesSkip
	sendKey(m, "j")
	if m.filesSkip != skip+1 {
		t.Fatalf("j did not scroll: %d → %d", skip, m.filesSkip)
	}
	sendKey(m, "k")
	if m.filesSkip != skip {
		t.Fatalf("k did not scroll back: %d", m.filesSkip)
	}
	sendEsc(m)
	if m.filesOn {
		t.Fatal("esc did not close the overlay")
	}

	sendKey(m, "x")
	if m.confirm == nil {
		t.Fatal("x asked nothing")
	}
	if strings.Contains(m.confirm.title, "delete its files") {
		t.Fatalf("x must be the app-only uninstall: %q", m.confirm.title)
	}
	m.confirm = nil

	sendKey(m, "X")
	if m.confirm == nil || !strings.Contains(m.confirm.title, "delete its files") {
		t.Fatalf("X must offer uninstall + files, got %+v", m.confirm)
	}
}

func TestAppsInstallKeyOpensModal(t *testing.T) {
	m := newTestModel(140, 45)
	m.eng = testEngine(t)
	seedApps(m, []apps.Package{{Manager: "apt", Name: "vim"}})
	sendKey(m, "i")
	if !m.installOn {
		t.Fatal("i did not open the install modal")
	}
	if v := m.View(); !strings.Contains(v, "install app from file") {
		t.Fatalf("install modal missing:\n%s", v)
	}
	sendEsc(m)
	if m.installOn {
		t.Fatal("esc did not close the install modal")
	}
}
