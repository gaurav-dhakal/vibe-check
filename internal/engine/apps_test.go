package engine

import (
	"testing"

	"github.com/gaurav-dhakal/vibe-check/internal/apps"
	"github.com/gaurav-dhakal/vibe-check/internal/config"
)

func seededEngine(n int) *Engine {
	e := New(config.Default())
	list := make([]apps.Package, n)
	for i := range list {
		list[i] = apps.Package{Manager: "apt", Name: "pkg"}
	}
	e.appsList.Store(&list)
	return e
}

func invGen(e *Engine) int64 {
	_, gen, _ := e.AppsStatus()
	return gen
}

// The inventory is the one thing the engine is allowed to forget: pinned while
// the apps tab is on screen, dropped by the minute tick after it closes.
func TestAppsIdleTickHonoursThePin(t *testing.T) {
	e := seededEngine(3)

	e.SetAppsPinned(true)
	e.appsIdleTick()
	if got := len(e.AppsPackages()); got != 3 {
		t.Fatalf("pinned inventory must survive a tick, got %d packages", got)
	}

	e.SetAppsPinned(false)
	e.appsIdleTick()
	if got := len(e.AppsPackages()); got != 0 {
		t.Fatalf("unpinned inventory must be released, got %d packages", got)
	}

	// A tick against an already-empty inventory changes nothing, so the UI's
	// cached filtered view is not invalidated once a minute forever.
	gen := invGen(e)
	e.appsIdleTick()
	if invGen(e) != gen {
		t.Fatal("empty release must not bump the generation")
	}
}
