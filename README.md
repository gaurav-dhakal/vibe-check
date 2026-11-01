# vibe-check

**Live telemetry for your dev machine — processes, ports, containers and build junk in one 60 FPS
pane, with one keypress to reclaim the RAM and disk your abandoned dev servers left behind.**

[![CI](https://github.com/gaurav-dhakal/vibe-check/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/gaurav-dhakal/vibe-check/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/gaurav-dhakal/vibe-check?label=release)](https://github.com/gaurav-dhakal/vibe-check/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/gaurav-dhakal/vibe-check/total?label=downloads)](https://github.com/gaurav-dhakal/vibe-check/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/gaurav-dhakal/vibe-check?label=go)](https://go.dev/)
[![License MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Platforms](https://img.shields.io/badge/platforms-linux%20%C2%B7%20macOS%20%C2%B7%20windows-blue)](#platforms)

You keep asking the same four questions about your own machine: *what is eating CPU, what is holding
this port, what did I forget to kill, and where did my disk go.* `vibe-check` answers all four without
leaving the terminal — reading the kernel directly, never guessing, and never costing more than a few
percent of a core.

Real capture from a live run (120×40, narrowed here to fit; values are actual, not a mockup):

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
│ fd 9544 · 38 avg/proc            ││ disk 80 dirs·13.6GB │
╰──────────────────────────────────╯╰─────────────────────╯
```

---

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [Keyboard](#keyboard)
- [Engines](#engines)
- [Configuration](#configuration)
- [Headless output](#headless-output)
- [Accuracy](#accuracy)
- [Performance](#performance)
- [Platforms](#platforms)
- [Development](#development)
- [Contributing](#contributing)
- [Security](#security)
- [About](#about)

## Install

**Linux and macOS**

```sh
curl -fsSL https://raw.githubusercontent.com/gaurav-dhakal/vibe-check/main/install.sh | bash
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/gaurav-dhakal/vibe-check/main/install.ps1 | iex
```

Each installer detects OS and architecture, downloads the matching
[release](https://github.com/gaurav-dhakal/vibe-check/releases) asset, verifies it against that
release's `checksums.txt`, and installs two names — `vibe-check` and the shorter `vcheck`. No Go, no
admin rights, no telemetry, no self-updater.

| Want | Do |
|---|---|
| A pinned version | `VIBE_CHECK_VERSION=0.1.3 curl -fsSL .../install.sh \| bash` |
| A different location | `VIBE_CHECK_INSTALL_DIR=/usr/local/bin …` (macOS users: sudo may be needed there) |
| Go's own path | `go install github.com/gaurav-dhakal/vibe-check@latest` |
| No network at all | `git clone` → `make build` → `./vibe-check` |

## Quick start

```sh
vibe-check                 # the TUI
vcheck --json              # one snapshot as JSON, no TUI, pipe-friendly
vcheck --theme tokyonight  # catppuccin | tokyonight | gruvbox
vcheck config init         # write ~/.config/vibe-check/config.toml and print the path
```

| Flag | Meaning |
|---|---|
| `--config <path>` | use a specific config file |
| `--theme <name>` | override the configured theme |
| `--interval <ms>` | telemetry cadence, clamped to 250–1000 |
| `--json` | print one snapshot and exit; the disk scan runs unthrottled |
| `--no-mouse` | leave the terminal's mouse alone |

Five tabs: **dashboard**, **processes**, **ports**, **docker**, **disk artifacts**. The footer shows
what the engines are doing, and every action reports back as a toast.

## Keyboard

| Key | Action |
|---|---|
| `j` `k` / `↓` `↑` | move the cursor |
| `g` `G` | jump to top / bottom |
| `Ctrl+d` `Ctrl+u` | half page down / up |
| `h` `l` | switch pane |
| `1`–`5` | jump to tab |
| `f` or `Space` | zoom the current pane |
| `/` | live fuzzy filter on the current list |
| `Esc` | clear the filter · close an overlay · unzoom |
| `Ctrl+p` | command palette |
| `?` or `F1` | keymap drawer |
| `y` / `Y` | copy row / copy row as JSON, to the native clipboard |
| `x` / `X` | act on the row: SIGTERM / SIGKILL a process · purge an artifact (asks first) · stop or start a container |
| `o` | orphan-only filter |
| `s` | cycle sort order |
| `r` | rescan disk artifacts |
| `t` | cycle theme |
| `q` / `Ctrl+c` | quit — the terminal is always restored |

Mouse works as expected: click tabs, rows and panes; wheel to scroll.

## Engines

Each one is a goroutine loop that publishes an immutable snapshot; the UI never blocks on collection.

**Orphan and leak detector** — dev servers reparented to PID 1, or whose parent vanished, are flagged
with the RSS you can reclaim. `x` kills the selected row; `Ctrl+p` → *kill all orphaned dev servers*
does the sweep. Newly reparented daemons are ignored until they clear `orphan_min_age_sec` and
`orphan_min_rss_mb`, so the list stays honest instead of noisy.

**Artifact crawler** — walks your home directory for `node_modules`, `target`, `.next`, `.venv`,
`dist`, `build`, `out`, `.turbo`, `.parcel-cache`. On Linux it decodes `getdents64` directly with zero
per-entry allocation and shares one duty-cycle limiter across its workers, so it is capped at a fraction
of a core no matter how many directories it opens. Results publish incrementally. Sizes are
hardlink-aware and byte-exact against `du --apparent-size`. `x` hands a directory to a concurrent purge
engine that reports progress and emits events.

**Port mapper** — reads the kernel's TCP listener table (on Linux, `/proc/net/tcp` and `tcp6`, with
socket inodes resolved to owning PIDs through a cached `/proc/*/fd` walk), maps `watch_ports` that are
not bound as explicitly *not bound*, and probes listeners with a non-blocking synthetic HTTP HEAD —
TLS sniffing included — so `3000` shows status and latency rather than just `LISTEN`.

**Container view** — talks to Docker or Podman over the raw unix socket (named pipe on Windows, with
Colima and `~/.docker` endpoints discovered) and streams per-container stats. No SDK, no CLI dependency,
no daemon-restart cleverness.

**FD and I/O pressure** — `/proc/sys/fs/file-nr` against the system limit, plus per-process FD sampling
that only reads what changed since the last tick.

**Terminal recovery** — the alternate screen, cursor and attributes are restored on clean quit, on
`SIGINT`, `SIGTERM` and `SIGHUP`, and from a recovered panic inside the UI. A killed `vibe-check` never
leaves your shell in a broken state.

## Configuration

`$XDG_CONFIG_HOME/vibe-check/config.toml`, or `~/.config/vibe-check/config.toml`.
`vibe-check config init` writes it; every key is optional and defaults are applied for anything missing.

```toml
theme                 = "catppuccin"          # catppuccin | tokyonight | gruvbox
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

## Headless output

`--json` writes one snapshot to stdout and nothing else — restore sequences go to the tty, so
`vibe-check --json | jq` stays clean.

```
{
  "at":          timestamp,
  "sys":         hostname, os, kernel, uptime_ns, num_cpu, cpu_model, cpu_percent,
                 core_percents[], load_1/5/15, mem_used/total/avail/percent,
                 swap_used/total, disk_read_bps, disk_write_bps, net_recv_bps,
                 net_sent_bps, proc_total, fd_alloc, fd_count, fd_max, fd_per_proc,
                 orphan_count, orphan_rss
  "procs":       pid, ppid, uid, user, name, cmdline, state, rss, cpu_percent,
                 age_sec, threads, orphan
  "ports":       proto, addr, port, state, pid, proc_name, monitored, http{...}
  "containers":  docker/podman view, empty when no runtime socket exists
  "artifacts":   path, project, size, files, kind
  "docker_err":  why the container engine has nothing, when it is empty
  "scan_state":  scanning, dirs_seen, last_done_at
}
```

```sh
vibe-check --json | jq '[.procs[] | select(.orphan)] | map(.rss) | add'   # reclaimable bytes
```

## Accuracy

The claim is boring and deliberate: a number is shown only when the kernel or API supplied it.

- Artifact bytes and file counts are byte-exact against `du --apparent-size` and `find | wc`, including
  hardlink deduplication — pinned by tests that build a directory tree with spaces, unicode, empty dirs,
  symlinks, loops and absolute links.
- `num_cpu`, `mem_total`, `hostname`, `fd_max`, listener count and `load_1` are cross-checked against
  `nproc`, `/proc/meminfo`, `/proc/sys/fs/file-nr`, `ss -ltn` and `/proc/loadavg`.
- Counts that legitimately drift between two samplings (process total, available memory, uptime) are
  explained by sampling time, not fudged into agreement.
- When a platform has no source for a metric, the UI renders a dash. It does not substitute an estimate.

## Performance

Two loops, deliberately decoupled: collectors write snapshots through `atomic.Pointer`, the UI reads
snapshots and redraws on a diff basis across a dual buffer, so an idle screen issues almost no writes
and cannot flicker. Panes repaint at up to 60 FPS without collecting anything.

Measured on an 8-core Linux laptop, in 10-second CPU buckets of a live run: **27%** of a core during the
first ten seconds while the initial crawl runs (higher on a cold page cache), then **2.8–4.2%** at steady
idle. `--json` runs the crawl unthrottled and returns a complete snapshot in about **1.6 s**.

## Platforms

| OS | Notes |
|---|---|
| Linux | Full feature set and the `/proc` fast path: per-process CPU, FD counts, socket-inode ownership, `file-nr` limits, zero-allocation directory reads |
| macOS | Kernel collectors through gopsutil — per-process CPU, memory, listeners with PID ownership, orphan detection, disk crawl |
| Windows | Listener enumeration without `netstat`, Docker over the named pipe, kills through the process API (no POSIX signals); hardlinked files count once per name, because NTFS gives no cheap inode fingerprint |

CI proves this rather than asserting it. Every platform job builds, vets, runs the suite, then executes
the headless snapshot **on that OS** and fails unless hostname, kernel, CPU model, memory, process count
and the disk crawl come back with plausible values (`scripts/verify-snapshot.awk`). The run on `main`
reported an `Apple M1` with 564 processes on macOS, an `AMD EPYC 9V74` with 152 on Windows, and an
`Intel Xeon` with 187 on Linux — collected, not asserted.

## Development

```sh
make lint        # gofmt + go vet
make test        # suite (engine parity + UI)
make race        # suite under -race
make build       # ./vibe-check
make release     # dist/ assets for every platform + checksums.txt
```

Layout: `cmd/` is the cobra entry point and terminal-safety net, `internal/engine` holds the collectors
(one file per platform split where the kernel differs), `internal/ui` the Bubbletea model and views,
`internal/theme` the palettes, `internal/config` the TOML surface. Platform-specific code is chosen by
build tags, never by runtime OS sniffing.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Short version: `make lint test race` before you push, add a
test when a bug was caused by a number being wrong, and keep the honesty rule — an unavailable metric
beats an invented one. Bugs and feature requests use the issue templates; please read
[SECURITY.md](SECURITY.md) before filing anything that involves the purge or kill paths.

Release history lives in [CHANGELOG.md](CHANGELOG.md).

## Security

`vibe-check` runs as you and can therefore do what you can do: signal processes and delete directories.

- No outbound network traffic. The only connections are HTTP HEAD probes to sockets already listening on
  your machine. No update checks, no analytics, no crash reporting.
- Purge asks for confirmation, and only ever targets a directory that the crawler itself listed.
- Container control is limited to start/stop over the local runtime socket.
- Config and clipboard interaction stay local; `y` copies row data to your clipboard, which is worth
  knowing if a command line contains a secret.

Report vulnerabilities privately — see [SECURITY.md](SECURITY.md).

## About

**Vibecoded.** This project was built pair-programming with an AI agent (Qoder) across the whole
codebase — engine, UI, packaging and CI — with humans making the calls on architecture, naming and
"no, that number has to be real". The commit messages and comments reflect that: plenty of them record
a measurement that contradicted an assumption, which is the only kind of debugging that worked.

MIT licensed, see [LICENSE](LICENSE).
