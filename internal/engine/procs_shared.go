package engine

import (
	"os/user"
	"strconv"
	"time"
)

// procTick is the previous-tick fingerprint of one process, whatever the
// platform's collector can measure. cpuTicks is jiffies on Linux and CPU
// seconds scaled by 1e6 elsewhere; start identifies the process instance so a
// recycled pid never inherits the old sample.
type procTick struct {
	cpuTicks uint64
	start    uint64
	cmd      string
	uid      uint32
	fd       int // -1 when not sampled this tick
}

type procState struct {
	prev       map[int32]procTick
	prevTotal  uint64
	users      map[uint32]string
	haveTotal  bool
	ticks      int
	fdSum      uint64
	fdReadable int
	prevWall   time.Time
}

// devProcessPatterns: names whose parent being init almost always means a
// forgotten dev server or orphaned tool rather than a legitimate daemon.
var devProcessPatterns = []string{
	"node", "npm", "npx", "pnpm", "yarn", "bun", "deno", "vite",
	"vitest", "webpack", "esbuild", "turbo", "tsc", "ts-node",
	"next", "nuxt", "remix", "astro", "vitepress", "electron",
	"python", "python3", "uvicorn", "gunicorn", "flask", "django",
	"poetry", "conda", "jupyter", "uv", "ruff", "mypy", "celery",
	"java", "gradle", "mvn", "sbt", "kotlin", "rails", "rake",
	"ruby", "puma", "sidekiq", "dotnet", "msbuild", "go", "air",
	"reflex", "cargo", "playwright", "puppeteer", "chromedriver",
	"ollama", "litellm", "streamlit", "gradio",
}

var devPatternSet = buildDevPatternSet()

func buildDevPatternSet() map[string]bool {
	m := make(map[string]bool, len(devProcessPatterns))
	for _, p := range devProcessPatterns {
		m[p] = true
	}
	return m
}

// orphanFlags decides whether a process is a leak candidate. A parent outside
// the current process table means the original shell is gone for good, which
// is the actual signal; ppid==1 is the Unix way of saying the same thing.
func (e *Engine) orphanFlags(p *Proc, parentKnown func(int32) bool) {
	if !devPatternSet[p.Name] {
		return
	}
	p.Orphan = (p.PPid == 1 || !parentKnown(p.PPid)) &&
		p.AgeSec > float64(e.cfg.OrphanMinAgeSec) &&
		p.RSS > uint64(e.cfg.OrphanMinRSSMB)*1024*1024
}

// userName resolves a uid to a login name once and caches it in the
// worker-owned state (single goroutine, no lock needed).
func (e *Engine) userName(uid uint32) string {
	if n, ok := e.proc.users[uid]; ok {
		return n
	}
	name := strconv.FormatUint(uint64(uid), 10)
	if u, err := user.LookupId(name); err == nil {
		name = u.Username
	}
	e.proc.users[uid] = name
	return name
}

func (e *Engine) sysNumCPU() int {
	if n := e.sys.Load().NumCPU; n > 0 {
		return n
	}
	return 1
}
