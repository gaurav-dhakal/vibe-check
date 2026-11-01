package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Theme              string   `toml:"theme"`           // catppuccin | tokyonight | gruvbox
	SysIntervalMS      int      `toml:"sys_interval_ms"` // telemetry cadence 250-1000
	ProcIntervalMS     int      `toml:"proc_interval_ms"`
	PortIntervalMS     int      `toml:"port_interval_ms"`
	DockerIntervalMS   int      `toml:"docker_interval_ms"`
	ArtifactScanMS     int      `toml:"artifact_scan_ms"`   // slow crawler cadence
	FPS                int      `toml:"fps"`                // render loop cap
	Mouse              bool     `toml:"mouse"`              // terminal mouse tracking
	RAMAlertPct        int      `toml:"ram_alert_pct"`      // badge turns warn above this
	OrphanMinAgeSec    int      `toml:"orphan_min_age_sec"` // ignore freshly reparented daemons
	OrphanMinRSSMB     int      `toml:"orphan_min_rss_mb"`  // only flag memory-hungry orphans
	HTTPProbeTimeoutMS int      `toml:"http_probe_timeout_ms"`
	ScanRoots          []string `toml:"scan_roots"`     // dirs walked by artifact crawler
	ArtifactKinds      []string `toml:"artifact_kinds"` // dir names treated as build junk
	WatchPorts         []int    `toml:"watch_ports"`    // extra ports always probed
	PurgeWorkers       int      `toml:"purge_workers"`  // concurrency of delete engine
	ScanWorkers        int      `toml:"scan_workers"`
}

func Default() Config {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "."
	}
	return Config{
		Theme:              "catppuccin",
		SysIntervalMS:      500,
		ProcIntervalMS:     1000,
		PortIntervalMS:     2500,
		DockerIntervalMS:   2000,
		ArtifactScanMS:     300000,
		FPS:                60,
		Mouse:              true,
		RAMAlertPct:        85,
		OrphanMinAgeSec:    120,
		OrphanMinRSSMB:     64,
		HTTPProbeTimeoutMS: 750,
		ScanRoots:          []string{home},
		ArtifactKinds: []string{
			"node_modules", "target", ".next", ".venv", "venv",
			"dist", "build", "out", ".turbo", ".parcel-cache",
		},
		WatchPorts:   []int{3000, 5173, 8080},
		PurgeWorkers: 16,
		ScanWorkers:  6,
	}
}

func Path() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "vibe-check", "config.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "vibe-check", "config.toml")
}

// Load reads the TOML config if present, merges over defaults, and
// clamps intervals into the safe 250ms-1000ms band for fast loops.
func Load(explicit string) (Config, error) {
	cfg := Default()
	p := explicit
	if p == "" {
		p = Path()
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Default(), err
	}
	cfg.SysIntervalMS = clamp(cfg.SysIntervalMS, 250, 1000)
	cfg.ProcIntervalMS = clamp(cfg.ProcIntervalMS, 250, 5000)
	cfg.PortIntervalMS = clamp(cfg.PortIntervalMS, 1000, 15000)
	cfg.DockerIntervalMS = clamp(cfg.DockerIntervalMS, 500, 15000)
	cfg.ArtifactScanMS = clamp(cfg.ArtifactScanMS, 10000, 3600_000)
	cfg.FPS = clamp(cfg.FPS, 10, 60)
	if cfg.PurgeWorkers < 1 {
		cfg.PurgeWorkers = 8
	}
	if cfg.ScanWorkers < 1 {
		cfg.ScanWorkers = 4
	}
	if len(cfg.ScanRoots) == 0 {
		cfg.ScanRoots = Default().ScanRoots
	}
	if len(cfg.ArtifactKinds) == 0 {
		cfg.ArtifactKinds = Default().ArtifactKinds
	}
	return cfg, nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Init writes the default config to disk if none exists. Returns the path.
func Init() (string, error) {
	p := Path()
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return p, err
	}
	f, err := os.Create(p)
	if err != nil {
		return p, err
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(Default()); err != nil {
		return p, err
	}
	return p, f.Sync()
}
