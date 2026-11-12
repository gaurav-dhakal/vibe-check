# Architecture

Two loops, deliberately decoupled: collectors write snapshots through `atomic.Pointer`, the UI reads
snapshots and redraws on a diff basis across a dual buffer, so an idle screen issues almost no writes
and cannot flicker. Panes repaint at up to 60 FPS without collecting anything.

```mermaid
flowchart LR
    subgraph E[Engines · one goroutine loop each]
        P[processes · /proc + syscalls]
        O[ports · kernel TCP tables + HEAD probe]
        D[containers · raw docker/podman socket]
        C[artifact crawler · getdents64]
        A[apps · native managers, on demand]
    end
    E -- immutable snapshot · atomic.Pointer --> S[snapshot store]
    S -- diff read --> U[Bubbletea UI · 60 FPS dual-buffer redraw]
    U -- "x: kill / purge / uninstall · i: install" --> E
```

## The engines

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

**GPU collector** — `nvidia-smi --query-gpu=…` when the NVIDIA driver is installed on any OS, otherwise
the AMD sysfs surface (`/sys/class/drm/card*/device/gpu_busy_percent`, `mem_info_vram_*`, and a hwmon
temperature). A card with no utilisation file is skipped rather than guessed at, so Intel iGPUs leave
the runtime pane in place; the dashboard swaps that pane for GPU stats only when a source exists.

**FD and I/O pressure** — `/proc/sys/fs/file-nr` against the system limit, plus per-process FD sampling
that only reads what changed since the last tick.

**Apps inventory** — the one collector that is not on a timer, because it cannot be: building the list
means shelling out to every manager on the host (`dpkg-query`, `rpm -qa`, `pacman -Qii`, `snap list`,
`flatpak list`, `brew info --json=v2`, `winget list`) and reading `package.json` for each npm global. So
it runs once when you open the tab, publishes through the same `atomic.Pointer`, and is dropped again
about a minute after you leave. Per-app file listings run in their own goroutine, keyed by
manager+name+version: selecting a different app supersedes the walk in flight rather than queueing it,
and the walk recurses only into directories the manager cannot enumerate itself (AppImages, npm, snap,
flatpak, casks) with a 60 000-file cap, because recursing through `/usr/share` for every shared path is
both slow and meaningless. Uninstall and install shell out to the native manager — never to a reaper of
our own invention — and ask polkit (`pkexec`, else `sudo -n`) when the manager demands root.

**Reclaim levers** — the dashboard's boost pane is a thin front for three engines that already exist:
the orphan killer, the purge engine, and a page-cache drop (`/proc/sys/vm/drop_caches`, `purge(8)`,
elevated). `auto_clean` puts the purge engine on the minute tick for build junk above
`auto_clean_junk_mb`; it is off by default because it is the only loop that deletes without asking, and
toggling it writes the choice back to config so it survives a restart.

**Terminal recovery** — the alternate screen, cursor and attributes are restored on clean quit, on
`SIGINT`, `SIGTERM` and `SIGHUP`, and from a recovered panic inside the UI. A killed `vibe-check` never
leaves your shell in a broken state.

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

Measured on an 8-core Linux laptop, in 10-second CPU buckets of a live run: **27%** of a core during the
first ten seconds while the initial crawl runs (higher on a cold page cache), then **2.8–4.2%** at steady
idle. `--json` runs the crawl unthrottled and returns a complete snapshot in about **1.6 s**.

## Memory

The same run, measured rather than estimated: `gctrace` reports the heap at **1→2 MB with a 1 MB goal**,
and `smaps_rollup` reports **6 MB of private anonymous memory** inside ~16 MB of RSS — the other 10 MB is
the mapped binary, clean pages the kernel reclaims first under pressure.

Three rules keep it there. History is read into a reusable per-model buffer (`Engine.RingValues`), so a
60 FPS frame allocates no slices for seven rings plus one per core. The package inventory — the only
collector large enough to be worth timing, about 1 MB for 2 400 packages on this host — is pinned only
while the apps tab or a file listing is on screen, then released by the minute tick with the pages handed
back to the OS. And the runtime starts with `GOGC=40` plus a 96 MB soft limit, so long sessions collect
eagerly instead of growing.
