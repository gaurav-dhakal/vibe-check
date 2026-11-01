# vibe-check

A developer-machine TUI that answers the questions you keep re-asking with `ps`, `ss`, `du` and
`docker ps` — in one screen, at 60 FPS, without ever spiking your CPU.

Processes, ports, containers and build junk, live. Kill a forgotten dev server with one keypress.
Purge a stale `node_modules` with another. Every number on screen comes from the kernel, not a guess.

```
● vibe-check  pop-os · linux/8c ▮ 12% cpu  ·  ▮ 52% mem  ·  up 1h39m
   dashboard   processes   ports 12   docker   disk 13.6GB
╭──────────────────────────────────╮╭─────────────────────╮
│ cpu                              ││ memory              │
│  12.0%  ███▋░░░░░░░░░░░░░░░░░░   ││  52.3%  ███████████▊│
│ 11th Gen Intel(R) Core(TM) i7…   ││ used 3.9GB / 7.4GB  │
│ load 2.45 4.00 3.18              ││ swap ███▍░░░ 2.6GB  │
│ c0  ██▊░░░░░░░░░░░░░░░░░░░   12  ││ ▄▄▄▄▄▄▄▄▄▄▄▄▄       │
│ c5  ████▌░░░░░░░░░░░░░░░░░░   20 ││                     │
╰──────────────────────────────────╯╰─────────────────────╯
╭──────────────────────────────────╮╭─────────────────────╮
│ i/o · fd                         ││ runtime             │
│ disk read      40.0KB/s ▁███████ ││ ports 12 listeners  │
│ net down       17.3KB/s ▁███████ ││ ✔ no orphaned devs  │
│ fd 9544 · 38 avg/proc              ││ disk 80 dirs·13.6GB │
╰──────────────────────────────────╯╰─────────────────────╯
```

Captured from a real 120×40 run (narrowed here so it reads in a browser).

## Install

**Linux / macOS / FreeBSD**

```sh
curl -fsSL https://raw.githubusercontent.com/gaurav-dhakal/vibe-check/main/install.sh | bash
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/gaurav-dhakal/vibe-check/main/install.ps1 | iex
```

Both install prebuilt release binaries, verify the release `checksums.txt`, and put the tool on your
`PATH`. The binary installs under two names — `vibe-check` and the short `vcheck` — so either works.

Pin a version or location:

```sh
VIBE_CHECK_VERSION=0.1.0 VIBE_CHECK_INSTALL_DIR=/usr/local/bin curl -fsSL .../install.sh | bash
```

**From source** (needs Go 1.27+, no network beyond the module mirror on first build):

```sh
go install github.com/gaurav-dhakal/vibe-check@latest
```

## Run

```sh
vibe-check              # the TUI
vcheck                  # same thing
vibe-check --json       # one snapshot as JSON, no TUI, pipe-friendly
vibe-check --help
```

```
--config <path>      path to config.toml
--theme <name>       catppuccin | tokyonight | gruvbox
--interval <ms>      telemetry cadence, 250–1000
--json               print one snapshot and exit (runs the disk scan at full speed)
--no-mouse           disable terminal mouse tracking
```

Subcommands: `vibe-check config init` writes the default config and prints its path.
`--version` reports the release.

Nothing leaves your machine. There is no telemetry, no update pings, and no outbound requests — the
only connections it makes are the HTTP HEAD health probes to sockets already listening on your own
machine.

## Keys

| Key | Action |
|---|---|
| `j ↓` / `k ↑` | move cursor |
| `g` / `G` | jump to top / bottom |
| `ctrl+d` / `ctrl+u` | half page down / up |
| `h ←` / `l →` | switch pane |
| `1`–`5` | jump to tab (dashboard, processes, ports, docker, disk) |
| `f` or `space` | fullscreen focus the current pane |
| `/` | fuzzy filter the current list, live |
| `ctrl+p` | command palette |
| `?` or `F1` | keymap drawer |
| `y` / `Y` | copy row / copy row as JSON (native clipboard) |
| `x` / `X` | act on the row: SIGTERM / SIGKILL a process · purge an artifact (asks first) · stop or start a container |
| `o` | orphan-only filter |
| `s` | cycle sort order |
| `r` | rescan disk artifacts |
| `t` | cycle theme |
| `q` / `ctrl+c` | quit — the terminal is always restored |

Mouse works too: click tabs, rows and panes, wheel to scroll.

## What it actually does

**Orphan + leak detector.** Dev servers reparented to PID 1 (or whose parent vanished) get flagged
with the RSS you can reclaim. `x` kills the selected one; the command palette (`ctrl+p`) has
*kill all orphaned dev servers* for the sweep. Freshly reparented daemons are ignored until they
pass `orphan_min_age_sec` and `orphan_min_rss_mb`, so the list stays honest rather than noisy.

**Artifact crawler.** Walks your home directory for `node_modules`, `target`, `.next`, `.venv`,
`dist`, `build`, … with a concurrent, *low-priority* engine: zero-allocation `getdents64` reads on
Linux plus a shared duty-cycle limiter pinned to a fraction of one core. It publishes results
incrementally, so you see sizes while it still walks. Sizes are hardlink-aware and match
`du --apparent-size` exactly on Linux and macOS (NTFS gives no cheap inode fingerprint, so on Windows
a file reached through two hardlinks counts twice). Purge runs a concurrent delete engine with
progress.

**Port mapper.** Reads `/proc/net/tcp` / `tcp6` (and the platform equivalent elsewhere), resolves
each socket inode to its owning process, and probes listeners with a non-blocking synthetic HTTP
HEAD — including TLS detection — so `3000` shows latency and status instead of just "LISTEN".

**Container view.** Talks to the Docker or Podman API over the raw unix socket (named pipe on
Windows, with Colima and `~/.docker` endpoints discovered) with no SDK dependency, and streams
per-container stats.

**Terminal recovery.** The engine never writes raw escape codes on exit paths that can be skipped.
On `SIGINT`, `SIGTERM`, `SIGHUP`, or a panic inside the UI, the alternate screen and cursor are
restored (`\x1b[?1049l\x1b[?25h`) so your shell is left usable.

**Headless.** `--json` emits one complete snapshot for scripts and CI.

## Configuration

`~/.config/vibe-check/config.toml` (XDG-aware). `vibe-check config init` writes this; every key is
optional:

```toml
theme              = "catppuccin"   # catppuccin | tokyonight | gruvbox
sys_interval_ms    = 500            # 250-1000
proc_interval_ms   = 1000
port_interval_ms   = 2500
docker_interval_ms = 2000
artifact_scan_ms   = 300000         # background disk crawl cadence
fps                = 60
mouse              = true
ram_alert_pct      = 85
orphan_min_age_sec = 120
orphan_min_rss_mb  = 64
http_probe_timeout_ms = 750
scan_roots         = ["/home/you"]
artifact_kinds     = ["node_modules", "target", ".next", ".venv", "venv", "dist", "build", "out", ".turbo", ".parcel-cache"]
watch_ports        = [3000, 5173, 8080]
purge_workers      = 16
scan_workers       = 6
```

## Rendering and performance

Two decoupled loops: background collectors write immutable snapshots through atomics, and the UI
reads them at up to 60 FPS. Redraws are diff-based across a dual buffer, so an idle screen costs
almost no writes and never flickers. Colour depth is negotiated (truecolor → 256 → 16) and the
collectors on Linux read `/proc` directly rather than shelling out.

Measured on a laptop-class 8-core Linux desktop (10-second CPU buckets of a live run): a 27% burst
of one core during the first ten seconds while the initial crawl runs — higher on a cold page cache —
then 2.8–4.2% at steady idle. `--json` runs the scan at full speed and returns a complete snapshot in
about 1.6s.

## Platforms

| OS | Notes |
|---|---|
| Linux | Full feature set, `/proc` fast path (per-process CPU, FD counts, socket-inode ownership, file-nr limits) |
| macOS | gopsutil collectors; orphan detection and port list included |
| Windows | gopsutil + `netstat`-free listener enumeration, named-pipe Docker, no POSIX signals (kills use the process API) |
| FreeBSD | Builds and runs; per-process FD limits degrade gracefully |

Where a metric has no portable source it is reported as unavailable instead of being faked.

## Development

```sh
go test ./...                  # engine parity + UI tests
./scripts/build-release.sh     # cross-compile every asset into dist/
```

## License

MIT — see [LICENSE](LICENSE).
