package engine

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/gaurav-dhakal/vibe-check/internal/apps"
)

// AppsPackages returns the last loaded inventory. The pointer is stable while
// pinned, so callers can hold the slice for one render.
func (e *Engine) AppsPackages() []apps.Package { return *e.appsList.Load() }

// AppsPinned reports whether the inventory is currently protected from the
// idle release; SetAppsPinned is how the UI claims that protection while the
// apps tab or the per-app files overlay is on screen.
func (e *Engine) AppsPinned() bool      { return e.appsPinned.Load() }
func (e *Engine) SetAppsPinned(on bool) { e.appsPinned.Store(on) }

// AppsIdleRelease drops the inventory once the user leaves the apps tab. It
// is the only list in the process that costs a second of shell-outs to
// rebuild and nothing reads once the apps tab is closed (~2 400 packages is
// about 1 MB of strings).
func (e *Engine) AppsIdleRelease() {
	if len(*e.appsList.Load()) == 0 {
		return // already released; keep the UI's cached view alive
	}
	empty := []apps.Package{}
	e.appsList.Store(&empty)
	e.appsGen.Add(1)
	e.appsStamp.Store(0) // re-baseline the watch when the list comes back
	// Go hands freed pages back to the OS on its own schedule, which can be
	// minutes; this is the one release the user can see coming.
	runtime.GC()
	debug.FreeOSMemory()
}

// appsIdleTick is the minute tick's memory half: the inventory goes only when
// the UI says nothing on screen is reading it.
func (e *Engine) appsIdleTick() {
	if !e.AppsPinned() {
		e.AppsIdleRelease()
	}
}

// AppsStatus reports loading state, an inventory generation counter (bumps
// on every successful reload so the UI can cache filtered views), and the
// last load error.
func (e *Engine) AppsStatus() (loading bool, gen int64, err string) {
	return e.appsLoading.Load(), e.appsGen.Load(), *e.appsErr.Load()
}

// AppsManagers lists the package managers detected on this host.
func (e *Engine) AppsManagers() []string { return apps.Available() }

// AppsReload shells out to every native manager. It runs once per user
// action, never on the telemetry cadence: the load is too heavy for 500ms.
func (e *Engine) AppsReload() {
	if !e.appsLoading.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer e.appsLoading.Store(false)
		list, err := apps.Load()
		if err != nil {
			s := err.Error()
			e.appsErr.Store(&s)
			e.emit(Event{Level: "error", Text: "✘ apps: " + s})
			return
		}
		empty := ""
		e.appsErr.Store(&empty)
		e.appsList.Store(&list)
		e.appsGen.Add(1)
		e.emit(Event{Level: "ok", Text: fmt.Sprintf("✔ %d packages · %v", len(list), apps.Available())})
	}()
}

// appsWatchTick reloads the inventory when something *outside* vibe-check
// changed a package database: apt in another terminal, the snap store
// self-updating, an AppImage dropped into ~/Apps. The check is a handful of
// stats, so it can run every couple of seconds; the shell-out only happens
// after a real change.
func (e *Engine) appsWatchTick() {
	if e.appsLoading.Load() || len(*e.appsList.Load()) == 0 {
		return // nothing on screen to keep current
	}
	s := apps.StateStamp()
	prev := e.appsStamp.Swap(s)
	if prev == 0 || prev == s {
		return // first look establishes the baseline
	}
	e.AppsReload()
}

// AppsUninstall removes one package, then reloads the inventory. purge=true
// additionally deletes the app's configuration and data.
func (e *Engine) AppsUninstall(p apps.Package, purge bool) {
	go func() {
		if err := apps.Uninstall(p, purge); err != nil {
			e.emit(Event{Level: "error", Text: fmt.Sprintf("✘ uninstall %s: %v", p.Name, err)})
			return
		}
		what := "removed"
		if purge {
			what = "removed with files"
		}
		e.emit(Event{Level: "warn", Text: fmt.Sprintf("✔ %s %s (%s)", what, p.Name, p.Manager)})
		e.AppsReload()
	}()
}

// AppsFiles resolves what one package put on disk. Only one listing is live
// at a time; moving the cursor and asking again supersedes a walk still in
// flight, which is why completion is guarded by a request token rather than a
// lock.
func (e *Engine) AppsFiles(p apps.Package) {
	key := AppsFileKey(p)
	if e.filesKey() == key && e.filesDone.Load() == e.filesReq.Load() {
		return // already shown or in flight for this exact package
	}
	n := e.filesReq.Add(1)
	e.filesErr.Store(new(string))
	e.appsFiles.Store(nil)
	e.filesFor.Store(&key)
	e.filesGen.Add(1)
	go func() {
		fl, err := apps.Files(p)
		if e.filesReq.Load() != n {
			return // a newer request owns the pane now
		}
		if err != nil {
			s := err.Error()
			e.filesErr.Store(&s)
		} else {
			e.appsFiles.Store(fl)
		}
		e.filesDone.Store(n)
	}()
}

// AppsFilesResult returns the current listing, the package it belongs to,
// whether a load is in flight, and the last error.
func (e *Engine) AppsFilesResult() (fl *apps.FileList, key string, loading bool, err string) {
	if p := e.filesErr.Load(); p != nil {
		err = *p
	}
	return e.appsFiles.Load(), e.filesKey(), e.filesDone.Load() != e.filesReq.Load(), err
}

func (e *Engine) filesKey() string {
	if p := e.filesFor.Load(); p != nil {
		return *p
	}
	return ""
}

// AppsFileKey names the package a listing belongs to, for the UI's cache.
func AppsFileKey(p apps.Package) string {
	return p.Manager + "\x00" + p.Name + "\x00" + p.Version
}

// AppsInstall runs the per-OS file installer for path, then reloads.
func (e *Engine) AppsInstall(path string) {
	go func() {
		e.emit(Event{Level: "info", Text: "installing " + path + "…"})
		desc, err := apps.Install(path)
		if err != nil {
			e.emit(Event{Level: "error", Text: "✘ install: " + err.Error()})
			return
		}
		e.emit(Event{Level: "ok", Text: "✔ " + desc})
		e.AppsReload()
	}()
}

// BoostCaches asks the OS to release its page cache so the free-RAM number is
// honest. Elevation belongs to the apps package, so this is the same pkexec /
// sudo -n path an uninstall uses.
func (e *Engine) BoostCaches() {
	go func() {
		if err := apps.DropPageCache(); err != nil {
			e.emit(Event{Level: "warn", Text: "page cache kept: " + err.Error()})
			return
		}
		e.emit(Event{Level: "ok", Text: "✔ page cache dropped — free RAM up now"})
	}()
}

func (e *Engine) SetAutoClean(on bool) { e.autoClean.Store(on) }
func (e *Engine) AutoClean() bool      { return e.autoClean.Load() }

// autoCleanTick reclaims build junk above the configured floor. It is a
// no-op unless the user enabled auto-clean; junk regenerates, so this is
// the one destructive loop we are willing to run unattended.
func (e *Engine) autoCleanTick() {
	if !e.autoClean.Load() {
		return
	}
	floor := uint64(e.cfg.AutoCleanJunkMB) << 20
	for _, a := range *e.artifacts.Load() {
		if a.Size >= floor {
			e.emit(Event{Level: "warn", Text: "auto-clean: purging " + a.Path})
			e.Purge(a.Path)
		}
	}
}
