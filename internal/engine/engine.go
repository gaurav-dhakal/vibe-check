package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gaurav-dhakal/vibe-check/internal/config"
)

const histLen = 180

type Engine struct {
	cfg config.Config
	ctx context.Context

	sys        atomic.Pointer[System]
	procs      atomic.Pointer[[]Proc]
	ports      atomic.Pointer[[]Port]
	containers atomic.Pointer[[]Container]
	dockerErr  atomic.Pointer[string]
	artifacts  atomic.Pointer[[]Artifact]
	scan       atomic.Pointer[ScanState]

	orphanCount atomic.Int64
	orphanRSS   atomic.Uint64
	scanFast    atomic.Bool
	fdSum       atomic.Uint64
	fdReadable  atomic.Int64
	procTotal   atomic.Int64

	events chan Event

	histMu  sync.Mutex
	history History

	// worker-owned state
	proc    procState
	sysc    sysCounters
	docker  *dockerRuntime
	portRes sync.Map // probeKey -> *HTTPProbe
	portSig uint64   // listener inode set fingerprint
	// portOwners is read only while the listener set is unchanged, so it
	// never needs to be re-walked for a Snapshot.
	portOwners map[uint64]int32
}

type History struct {
	CPU      Ring
	Mem      Ring
	CPUCores []Ring
	NetR     Ring
	NetW     Ring
	DiskR    Ring
	DiskW    Ring
}

// Ring is a fixed-capacity circular buffer of float64 samples.
type Ring struct {
	buf  []float64
	next int
	full bool
}

func NewRing(n int) Ring { return Ring{buf: make([]float64, n)} }

func (r *Ring) Push(v float64) {
	if len(r.buf) == 0 {
		r.buf = make([]float64, histLen)
	}
	r.buf[r.next] = v
	r.next++
	if r.next >= len(r.buf) {
		r.next = 0
		r.full = true
	}
}

// Values returns samples in chronological order.
func (r *Ring) Values() []float64 {
	if !r.full {
		out := make([]float64, r.next)
		copy(out, r.buf[:r.next])
		return out
	}
	out := make([]float64, 0, len(r.buf))
	out = append(out, r.buf[r.next:]...)
	out = append(out, r.buf[:r.next]...)
	return out
}

func New(cfg config.Config) *Engine {
	e := &Engine{cfg: cfg, events: make(chan Event, 256)}
	e.sys.Store(&System{})
	emptyP := []Proc{}
	emptyPort := []Port{}
	emptyC := []Container{}
	emptyA := []Artifact{}
	e.procs.Store(&emptyP)
	e.ports.Store(&emptyPort)
	e.containers.Store(&emptyC)
	e.artifacts.Store(&emptyA)
	e.dockerErr.Store(new(string))
	e.scan.Store(&ScanState{})
	e.history.CPU = NewRing(histLen)
	e.history.Mem = NewRing(histLen)
	e.history.NetR = NewRing(histLen)
	e.history.NetW = NewRing(histLen)
	e.history.DiskR = NewRing(histLen)
	e.history.DiskW = NewRing(histLen)
	e.docker = newDockerRuntime()
	e.proc.prev = map[int32]procTick{}
	e.proc.users = map[uint32]string{}
	return e
}

func (e *Engine) Run(ctx context.Context) {
	e.ctx = ctx
	go e.loop(ctx, "sys", ms(e.cfg.SysIntervalMS), e.updateSys)
	go e.loop(ctx, "proc", ms(e.cfg.ProcIntervalMS), e.updateProcs)
	go e.loop(ctx, "ports", ms(e.cfg.PortIntervalMS), e.updatePorts)
	go e.loop(ctx, "docker", ms(e.cfg.DockerIntervalMS), e.updateDocker)
	go e.artifactLoop(ctx)
}

// loop runs collect() on a fixed cadence; panics in one worker must never
// take down the telemetry pipeline, so each tick is fenced.
func (e *Engine) loop(ctx context.Context, name string, d time.Duration, collect func()) {
	t := time.NewTicker(d)
	defer t.Stop()
	e.fence(name, collect)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.fence(name, collect)
		}
	}
}

func (e *Engine) fence(name string, collect func()) {
	defer func() {
		if r := recover(); r != nil {
			e.emit(Event{Level: "error", Text: fmt.Sprintf("%s worker panic: %v", name, r)})
		}
	}()
	collect()
}

func (e *Engine) Snapshot() Snapshot {
	s := *e.sys.Load()
	s.OrphanCount = int(e.orphanCount.Load())
	s.OrphanRSS = e.orphanRSS.Load()
	s.ProcTotal = int(e.procTotal.Load())
	return Snapshot{
		At:         time.Now(),
		Sys:        s,
		Procs:      *e.procs.Load(),
		Ports:      *e.ports.Load(),
		Containers: *e.containers.Load(),
		Artifacts:  *e.artifacts.Load(),
		DockerErr:  *e.dockerErr.Load(),
		ScanState:  *e.scan.Load(),
	}
}

func (e *Engine) History() *History {
	e.histMu.Lock()
	defer e.histMu.Unlock()
	return &e.history
}

func (e *Engine) Events() <-chan Event { return e.events }

func (e *Engine) emit(ev Event) {
	select {
	case e.events <- ev:
	default: // drop rather than ever block a worker
	}
}

func (e *Engine) Cfg() config.Config { return e.cfg }

// Kill terminates a process: SIGTERM, or SIGKILL when force is set.
func (e *Engine) Kill(pid int32, force bool) error {
	label, err := signalProcess(pid, force)
	if err != nil {
		e.emit(Event{Level: "error", Text: fmt.Sprintf("✘ kill %d failed: %v", pid, err)})
		return err
	}
	name := ""
	for _, pr := range *e.procs.Load() {
		if pr.Pid == pid {
			name = pr.Name
			break
		}
	}
	if name == "" {
		name = "proc"
	}
	e.emit(Event{Level: "ok", Text: fmt.Sprintf("✔ sent %s to %s (%d)", label, name, pid)})
	return nil
}

// Purge deletes an artifact directory with the concurrent delete engine,
// asynchronously, and emits progress + result as events.
func (e *Engine) Purge(path string) {
	if err := safePurgeTarget(e.Snapshot().Artifacts, path); err != nil {
		e.emit(Event{Level: "error", Text: "✘ " + err.Error()})
		return
	}
	go func() {
		start := time.Now()
		freed, files, err := purgeDir(path, e.cfg.PurgeWorkers)
		elapsed := time.Since(start)
		if err != nil {
			e.emit(Event{Level: "error", Text: fmt.Sprintf("✘ purge %s: %v", path, err)})
			return
		}
		e.emit(Event{Level: "ok", Text: fmt.Sprintf("✔ purged %s · %s reclaimed in %dms (%d files)",
			path, Bytes(freed), elapsed.Milliseconds(), files)})
	}()
}

func safePurgeTarget(list []Artifact, path string) error {
	for _, a := range list {
		if a.Path == path {
			return nil
		}
	}
	return errf("refusing to purge path not reported by scanner: %s", path)
}

// StartContainer / StopContainer proxy to the runtime socket.
func (e *Engine) StartContainer(id string) { e.doCtl(id, "start") }
func (e *Engine) StopContainer(id string)  { e.doCtl(id, "stop") }

func (e *Engine) doCtl(id, action string) {
	go func() {
		if err := e.docker.ctl(id, action); err != nil {
			e.emit(Event{Level: "error", Text: fmt.Sprintf("✘ %s %s: %v", action, shortID(id), err)})
			return
		}
		e.emit(Event{Level: "ok", Text: fmt.Sprintf("✔ %s → %s", action, shortID(id))})
	}()
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }
