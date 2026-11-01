package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const maxScanDepth = 6

// crawlDutyCycle caps the background scan at this fraction of one CPU core.
// A full home pass costs a few seconds of work, so at 25% it spreads over
// tens of seconds and averages well under 1% of a core between rescans.
const (
	crawlDutyCycle = 0.25
	// debtFloor is how much unpaid time accumulates before the crawl actually
	// sleeps: parking on every small directory would cost more in timer
	// syscalls than the work itself.
	debtFloor = 2 * time.Millisecond
)

var skipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true, "Library": true,
	"snap": true, "AppData": true, ".Trash": true, ".local": true,
	"proc": true, "sys": true, ".docker": true, "lost+found": true,
}

type crawlJob struct {
	path  string
	depth int
}

// artifactLoop is deliberately slow + budgeted so disk crawling never
// starves the UI or the fast telemetry loops.
func (e *Engine) artifactLoop(ctx context.Context) {
	d := time.NewTicker(time.Duration(e.cfg.ArtifactScanMS) * time.Millisecond)
	defer d.Stop()
	e.runScan(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.C:
			e.runScan(ctx)
		}
	}
}

func (e *Engine) Rescan() { go e.runScan(e.ctx) }

// ScanFullSpeed drops the duty-cycle limit. One-shot callers (--json) want a
// complete answer now; only the interactive TUI should stay polite.
func (e *Engine) ScanFullSpeed() { e.scanFast.Store(true) }

func (e *Engine) runScan(ctx context.Context) {
	st := e.scan.Load()
	if st.Scanning {
		return
	}
	newSt := *st
	newSt.Scanning = true
	newSt.Error = ""
	e.scan.Store(&newSt)

	kinds := map[string]bool{}
	for _, k := range e.cfg.ArtifactKinds {
		kinds[k] = true
	}

	var (
		mu        sync.Mutex
		results   []Artifact
		seen      atomic.Int64
		wg        sync.WaitGroup
		measure   sync.WaitGroup
		workers   = e.cfg.ScanWorkers
		published time.Time
	)
	if workers < 1 {
		workers = 4
	}
	sem := make(chan struct{}, workers)
	// one shared budget across every goroutine: the scan can grow wider but
	// never hotter.
	var pace *pacer
	if !e.scanFast.Load() {
		pace = newPacer(crawlDutyCycle)
	}

	// publish hands the pane whatever is known so far. A paced scan can run for
	// minutes, so results have to arrive as they are found instead of all at
	// the end; the 400ms throttle keeps the sort off the hot path.
	publish := func(force bool) {
		mu.Lock()
		if !force && time.Since(published) < 400*time.Millisecond {
			mu.Unlock()
			return
		}
		published = time.Now()
		cp := make([]Artifact, len(results))
		copy(cp, results)
		mu.Unlock()
		sort.Slice(cp, func(i, j int) bool { return cp[i].Size > cp[j].Size })
		e.artifacts.Store(&cp)
		st := e.scan.Load()
		ns := *st
		ns.DirsSeen = int(seen.Load())
		e.scan.Store(&ns)
	}

	push := func(root string) {
		wg.Add(1)
		go func(root string) {
			defer wg.Done()
			e.walkCrawl(ctx, root, kinds, pace, &mu, &results, &seen, sem, &measure, publish)
		}(root)
	}

	for _, root := range e.cfg.ScanRoots {
		r := strings.TrimPrefix(root, "~")
		if r == "" || r == "/" {
			r = os.Getenv("HOME")
		} else if !filepath.IsAbs(r) && !strings.HasPrefix(root, "~") {
			r = filepath.Clean(filepath.Join(os.Getenv("HOME"), r))
		}
		if fi, err := os.Stat(r); err != nil || !fi.IsDir() {
			continue
		}
		push(r)
	}
	wg.Wait()
	measure.Wait()
	publish(true)

	st2 := e.scan.Load()
	ns := *st2
	ns.Scanning = false
	ns.DirsSeen = int(seen.Load())
	ns.LastDoneAt = time.Now()
	e.scan.Store(&ns)
	if len(results) > 0 {
		var total uint64
		for _, a := range results {
			total += a.Size
		}
		e.emit(Event{Level: "info", Text: fmt.Sprintf("disk scan: %d artifact dirs · %s reclaimable", len(results), Bytes(total))})
	}
}

func (e *Engine) walkCrawl(ctx context.Context, root string, kinds map[string]bool, pace *pacer, mu *sync.Mutex, out *[]Artifact, seen *atomic.Int64, sem chan struct{}, measure *sync.WaitGroup, publish func(bool)) {
	var buf dirBuf
	stack := []crawlJob{{path: root, depth: 0}}
	for len(stack) > 0 {
		if ctx.Err() != nil {
			return
		}
		job := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		turn := time.Now()
		seen.Add(1)
		err := scanDir(job.path, &buf, func(d *dentry) bool {
			// map lookups on string(d.Name) compile down to no-alloc compares,
			// so only the paths we really descend into cost anything.
			if !d.isDir || skipDirs[string(d.Name)] {
				return true
			}
			full := filepath.Join(job.path, string(d.Name))
			if kinds[string(d.Name)] {
				// blocking acquire = backpressure: the crawl can only run
				// ahead of the measurement pool by ScanWorkers dirs.
				sem <- struct{}{}
				measure.Add(1)
				go func(full string, name []byte) {
					defer func() { <-sem }()
					defer measure.Done()
					size, files := measureDir(ctx, full, pace)
					if size == 0 && files == 0 {
						return
					}
					mu.Lock()
					*out = append(*out, Artifact{
						Path: full, Kind: string(name), Size: size, Files: files,
						Project: projectOf(full, string(name)),
					})
					mu.Unlock()
					publish(false)
				}(full, cloneName(d.Name))
				return true
			}
			if job.depth+1 < maxScanDepth {
				stack = append(stack, crawlJob{path: full, depth: job.depth + 1})
			}
			return true
		})
		if err != nil {
			continue
		}
		// Charging after the walk also bills this turn's allocations, which is
		// where a wide directory really costs.
		if !pace.charge(ctx, time.Since(turn)) {
			return
		}
	}
}

// cloneName copies a buffer-aliased entry name that has to outlive the
// directory it came from.
func cloneName(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func projectOf(path, kind string) string {
	parent := filepath.Base(filepath.Dir(path))
	if parent == "/" || parent == "." {
		return kind
	}
	return parent
}

// pacer is a duty-cycle limiter shared by every crawl goroutine: the whole
// scan gets at most `target` of one core, whatever a directory turn costs on
// this machine. Workers report the time they just spent, and the debt that
// creates is paid back in one sleep once it is worth parking for.
type pacer struct {
	mu     sync.Mutex
	debt   time.Duration
	target float64
}

func newPacer(target float64) *pacer { return &pacer{target: target} }

// charge records a work unit that took d and sleeps once enough unpaid time
// has built up. Returns false if ctx was cancelled.
func (p *pacer) charge(ctx context.Context, d time.Duration) bool {
	if p == nil {
		return ctx.Err() == nil
	}
	p.mu.Lock()
	// Spending d of CPU at target of a core owes d/target of wall clock, so the
	// unpaid remainder is the sleep this turn bought.
	p.debt += time.Duration(float64(d)/p.target) - d
	if p.debt < debtFloor {
		p.mu.Unlock()
		return ctx.Err() == nil
	}
	wait := p.debt
	p.debt = 0
	p.mu.Unlock()

	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func measureDir(ctx context.Context, root string, pace *pacer) (uint64, int) {
	var (
		size  uint64
		files int
		buf   dirBuf
		links map[uint64]bool
	)
	stack := []string{root}
	for len(stack) > 0 {
		dir := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		turn := time.Now()
		if err := scanDir(dir, &buf, func(d *dentry) bool {
			if d.isDir {
				stack = append(stack, filepath.Join(dir, string(d.Name)))
				return true
			}
			// symlinks count as their own size, exactly as lstat reports them,
			// so a node_modules/.bin farm is not charged twice.
			if !d.stat() {
				return true
			}
			if d.nlink > 1 && d.linkKey != 0 {
				if links == nil {
					links = make(map[uint64]bool)
				}
				if links[d.linkKey] {
					return true
				}
				links[d.linkKey] = true
			}
			size += uint64(d.size)
			files++
			return true
		}); err != nil {
			continue
		}
		if !pace.charge(ctx, time.Since(turn)) {
			break
		}
	}
	return size, files
}

// purgeDir is the concurrent delete engine: parallel RemoveAll over the
// top-level entries (wide node_modules trees benefit most), then the root.
func purgeDir(path string, workers int) (uint64, int, error) {
	if workers < 1 {
		workers = 8
	}
	ents, err := os.ReadDir(path)
	if err != nil {
		return 0, 0, err
	}
	var freed uint64
	var nfiles int
	var mu sync.Mutex
	var firstErr error
	var onceErr sync.Once
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, ent := range ents {
		full := filepath.Join(path, ent.Name())
		if ent.IsDir() {
			wg.Add(1)
			sem <- struct{}{}
			go func(full string) {
				defer wg.Done()
				defer func() { <-sem }()
				// purge is an explicit user action: no pacing, real speed.
				s, f := measureDir(context.Background(), full, nil)
				if err := os.RemoveAll(full); err != nil {
					onceErr.Do(func() { firstErr = err })
					return
				}
				mu.Lock()
				freed += s
				nfiles += f
				mu.Unlock()
			}(full)
		} else {
			info, err := ent.Info()
			if err == nil {
				freed += uint64(info.Size())
			}
			nfiles++
			if err := os.Remove(full); err != nil {
				onceErr.Do(func() { firstErr = err })
			}
		}
	}
	wg.Wait()
	if err := os.Remove(path); err != nil && firstErr == nil && !os.IsNotExist(err) {
		firstErr = err
	}
	return freed, nfiles, firstErr
}

func errf(format string, a ...any) error { return fmt.Errorf(format, a...) }
