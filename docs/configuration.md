# Configuration & flags

## Config file

`$XDG_CONFIG_HOME/vibe-check/config.toml`, or `~/.config/vibe-check/config.toml`.
The first launch asks for a theme and writes the file; `vibe-check config init` writes the stock
defaults without asking, and `--default` does the same on the first run. Every key is optional and
defaults are applied for anything missing.

```toml
theme                 = "catppuccin"   # catppuccin · tokyonight · gruvbox · nord · dracula · solarized
sys_interval_ms       = 500                   # 250-1000
proc_interval_ms      = 1000
port_interval_ms      = 2500
docker_interval_ms    = 2000
artifact_scan_ms      = 300000                # background crawl cadence
fps                   = 60
mouse                 = true
ram_alert_pct         = 85
orphan_min_age_sec    = 120
orphan_min_rss_mb     = 64
http_probe_timeout_ms = 750
scan_roots            = ["/home/you"]
artifact_kinds        = ["node_modules", "target", ".next", ".venv", "venv",
                         "dist", "build", "out", ".turbo", ".parcel-cache"]
watch_ports           = [3000, 5173, 8080]
purge_workers         = 16
scan_workers          = 6
```

## CLI flags

| Flag | Meaning |
|---|---|
| `--config <path>` | use a specific config file |
| `--theme <name>` | override the configured theme for this run |
| `--default` | take the stock settings and record them, so nothing is asked on first run |
| `--interval <ms>` | telemetry cadence, clamped to 250–1000 |
| `--json` | print one snapshot and exit; the disk scan runs unthrottled |
| `--no-mouse` | leave the terminal's mouse alone |
| `--uninstall` | same as the `uninstall` subcommand |

## Subcommands

| Command | Meaning |
|---|---|
| `vibe-check` | the TUI |
| `vcheck --json` | one snapshot as JSON, no TUI, pipe-friendly |
| `vcheck config init` | write the config file and print its path |
| `vcheck uninstall` | remove the binaries and config dir; `--yes` for scripts |
