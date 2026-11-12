# Changelog

All notable changes to `vibe-check` are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); the project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Release binaries for every version are in [GitHub Releases](https://github.com/gaurav-dhakal/vibe-check/releases).

## [Unreleased]

### Added
- **Apps tab (`6`) — an inventory of everything installed.** One list built from whatever the host
  actually has: apt/dpkg, dnf/yum, rpm, pacman, snap and flatpak on Linux; brew formulae *and* casks on
  macOS; winget on Windows — plus npm globals and loose `.AppImage` files on every platform, because
  those live outside any manager's view. Size, version, manager and description per row, `/` to filter,
  `s` to sort by name/size/manager, `y`/`Y` to copy.
- **Per-app file browser.** `⏎` on a package lists every path it owns with per-folder file counts and
  byte totals (`apt`/`rpm`/`pacman` from the manager's own file list; snap, flatpak, casks, AppImages
  and npm from their real directories). Scrollable, `y` copies a path, `Esc` closes. The walk is
  asynchronous and capped, and selecting another app supersedes the walk in flight.
- **Uninstall, with a choice about the files.** `x` removes the app and leaves config/data alone; `X`
  removes it *and* its files using each manager's own purge semantics — `apt-get purge`, `dnf/yum
  remove`, `pacman -Rns`, `snap remove --purge`, `flatpak uninstall --delete-data`, `brew
  uninstall --purge` / `brew uninstall --cask --zap`, `winget uninstall --silent`. Both confirm first
  and name the exact command.
- **Install from a file (`i`).** `.deb` and `.rpm` go to apt/dnf/yum so dependencies resolve;
  `.tar.gz`/`.tar.xz`/`.tar.bz2`/`.zip` extract under `~/Apps`; a `.AppImage` is staged to `~/Apps`,
  made executable and registered in the desktop tray with a `.desktop` entry (and
  `update-desktop-database` when present). macOS accepts `.pkg`/`.dmg`/`.app`/`.zip`; Windows accepts
  `.msi`/`.exe`/`.zip`.
  Archive extraction is zip-slip checked — an entry that escapes the destination aborts the whole
  extract.
- **Privilege elevation that cannot hang.** Anything a manager wants root for runs through `pkexec`, or
  `sudo -n` when polkit is absent, and as the real user when the manager is a per-user one (flatpak,
  brew, npm). There is no path where the TUI waits on a password prompt it cannot show.
- **The dashboard is a task manager now: six panes** (`h`/`l` to select, `f` to zoom). CPU gained a
  history sparkline per core plus model/core-count/clock identity lines; a new `disk · usage` pane
  charts every real partition (tmpfs, overlay and friends filtered out) with free/total bytes and a
  summed free figure; `i/o · net` uses auto-scaled charts so 4 KB/s and 400 MB/s are both readable; and a
  new `boost · clean` pane collects every reclaim lever with live numbers.
- **Reclaim keys.** `c` runs a boost & clean sweep — kill orphans, purge build junk, drop the page
  cache — after listing exactly what it will do. `C` toggles auto-clean (unattended purge of junk at or
  above `auto_clean_junk_mb`, checked every minute) and persists it to config. `b` drops the kernel page
  cache so free RAM is honest.
- `--json` gains `sys.disks[]` (mount, fstype, total, used, percent) and `sys.core_mhz[]`.
- Config gains `auto_clean` (default off) and `auto_clean_junk_mb` (default 256, floored at 32).
- **The apps list is live.** A two-second watch on the fingerprints a package operation actually
  leaves behind — `/var/lib/dpkg/status`, the rpm and pacman databases, snapd state, the flatpak and
  brew folders, npm globals, `~/Apps` — reloads the inventory when something outside vibe-check
  changed them, so an `apt install` in another terminal shows up without `r`. It costs a handful of
  `stat` calls, never a shell-out, and it stops with the tab: an idle-released list has nothing to
  keep current.
- **The system's own file picker for install-from-file.** `i` then `Ctrl+f` opens the dialog the
  desktop itself provides — on Linux that is the XDG desktop portal's `FileChooser`, the same request
  a browser makes, so COSMIC, GNOME and KDE answer with their native picker even on a machine that has
  no `zenity` at all (`zenity`/`kdialog` remain as fallbacks); macOS gets `osascript`'s chooser and
  Windows the shell file dialog. A cancel is reported as a cancel and a portal refusal as its own
  error. Over SSH, in a container or on a bare VT the keys are the same; the path is typed instead,
  because opening a chooser with no compositor would hang forever.
- **Every apps and reclaim action has a CLI twin**, so nothing needs a terminal wide enough to see it:
  `apps list|files|remove|install|managers`, `boost` and `autoclean [on|off]`, each with `--json`
  where a machine wants to read it. `apps remove` refuses to act without `--yes`, and an ambiguous
  name errors with its candidates instead of guessing.
- Key-binding regression tests for the apps tab, the file browser and the install modal
  (`internal/ui/keycheck_test.go`), an engine test for the pinned/idle-release contract, plus AppImage
  name parsing, unknown-manager and missing-file cases in `internal/apps`.
- A render-geometry test (`internal/ui/render_test.go`) that draws all six tabs × the help drawer,
  palette, installer, file browser, confirmation and zoom at 80×24, 60×20, 50×14, 40×12 and 120×40,
  and fails if a single row escapes the terminal.

### Changed
- **Search ranks.** `/` now returns the best fuzzy matches first and suspends the sort column while a
  filter is live, so `vim` in the apps inventory leads with `vim-tiny` and `vim-common` instead of the
  name-sorted list's accidental subsequences. The apps list also stopped sorting the engine's own
  slice in place.
- **Memory.** The package inventory is the largest single thing the process holds, so it is read only
  when the apps tab opens (and stays pinned while the tab or file browser is on screen) and released
  about a minute after you leave, with the pages handed back to the OS. Measured on a 2 427-package
  host: live heap 1–2 MB, and `smaps_rollup` puts private anonymous memory at ~6 MB — the rest of the
  ~16 MB RSS is the mapped binary. The UI pins what it is reading instead of the engine guessing.
- Dashboard history is read into a reusable per-model buffer, so the 60 FPS render path no longer
  allocates a history slice per pane per frame.
- The runtime now collects eagerly (`GOGC=40`) against a 96 MB soft limit.
- Mouse input is ignored while an overlay owns the screen, so clicks cannot move the list underneath.
- README leads with the apps manager, the task-manager dashboard and the memory posture; `docs/` covers
  the new keys, config keys and JSON fields.

### Fixed
- **Nothing is hidden on a small or minimised terminal.** The bug behind the reports was measurement:
  a CSI's introducer `[` sits inside the byte range that *ends* an escape sequence, so the width
  helper stopped after the `ESC` and counted `1;38;2;137;179;250m` as twenty columns of text. Every
  truecolor hint looked far wider than it was, so the key bar, header and tab strip wrapped and
  shifted. With the measurement fixed: the key bar wraps to a second row instead of truncating (so
  `x uninstall` and `i install file` survive at 60 columns), the tab strip drops counts then shortens
  labels then gives its row back, list tables drop their right-most columns rather than wrap mid-row,
  the dashboard collapses from three panes across to two and one, every modal caps to the window and
  scrolls, and the old 20×8 "blank screen" floor is gone.
- A crash when a CPU history sparkline was drawn from a flat or single-sample series: the level table
  was measured in bytes instead of runes, so a zero-span gauge could index past the end.
- Returning to the apps tab after an idle release now re-reads the inventory instead of showing an
  empty list — the reload trigger looks at what is in memory, not at a generation counter the release
  also bumps.

## [0.1.4] — 2026-11-02

### Added
- **GPU stats.** A new collector reads `nvidia-smi` on any OS, or AMD's sysfs surface
  (`gpu_busy_percent`, `mem_info_vram_*`, hwmon temperature) on Linux, and the dashboard swaps the
  runtime pane for a GPU pane when a source exists. Cards with no readable utilisation counter —
  Intel iGPUs — keep the runtime pane instead of showing a guess. `--json` gains a `gpus` array.
- **First-run theme picker.** A fresh install now asks which theme you want instead of silently
  starting on catppuccin, and writes the answer to your config. `--default` takes the stock settings
  without asking; `--theme` and `--json` skip the question too.
- **Three more themes** — `nord`, `dracula` and `solarized` — alongside the existing three; the menu
  shows each palette's full name.
- **`vibe-check uninstall`** (also `vcheck --uninstall`) removes the `vibe-check` and `vcheck`
  binaries and the config directory, asking first unless `--yes` is given. When Homebrew or winget
  owns the install it prints the package-manager command instead of deleting files behind its back.
- `t` inside the app now persists the theme: it rewrites only the `theme` key, leaving the rest of
  the file (and its comments) untouched.
- `CONTRIBUTING.md` (ground rules, make targets, layout map, recipes for a new metric or platform),
  `SECURITY.md` (capability and blast-radius table, private reporting route, supported versions) and
  this changelog.
- README rewritten as a landing page: animated `demo.gif` above the fold, a comparison table against
  htop/lsof/docker stats/ncdu, a Mermaid architecture diagram, copy-paste `jq` usage examples, an FAQ,
  a star-history chart and a star callout; deep content (engines, accuracy, full config, keymap, JSON
  schema, development) moved to `docs/`.
- Homebrew tap [`gaurav-dhakal/homebrew-vibe-check`](https://github.com/gaurav-dhakal/homebrew-vibe-check)
  (`brew tap gaurav-dhakal/vibe-check && brew install vibe-check`) and a winget submission for 0.1.3
  (microsoft/winget-pkgs#468434) using the zip assets with nested portable aliases.
- `Makefile` with `fmt vet lint test race build install release clean`; `make` resolves the version from
  the git tag exactly like the release script, so a local build reports what a released one would.
- Issue forms for bugs and feature requests, a pull request template, and Dependabot for Go modules and
  GitHub Actions.
- `.editorconfig`, so tabs in Makefiles and two-space indentation elsewhere are a repo rule instead of a
  per-editor preference.

### Removed
- Termux as an advertised platform. The README install box, the Platforms tables and
  `docs/development.md` no longer claim it: Android hides other processes' `/proc` and `/proc/net` from
  unprivileged apps, so the interesting half of the dashboard would be dashes anyway. `install.sh` still
  installs into `$PREFIX/bin` when it detects Termux, and the `linux/arm64` asset is unchanged.

### Changed
- README install section gives every method its own labelled code box (curl, Homebrew, PowerShell,
  winget) plus an uninstall section, and `demo.gif` re-records the first-run theme choice.
- `.gitignore` no longer leaves CI's `snapshot.json` artifact in the working tree.

### Fixed
- The Linux release binaries are now position-independent (`-buildmode=pie`). Android's loader refuses
  `ET_EXEC` with `has unexpected e_type: 2`, which is why the 0.1.4 arm64 asset could not be exec'd on
  a phone at all. The release script builds Linux as PIE and fails the release if a Linux binary comes
  out as anything other than `ET_DYN`, so this cannot ship silently again. The 0.1.4 `linux_amd64` and
  `linux_arm64` assets were rebuilt and replaced in place; macOS and Windows assets are untouched.

## [0.1.3] — 2026-11-01

### Removed
- FreeBSD as a shipped platform: dropped from the release matrix, the installers, CI and the docs. It
  was the only OS that had no user-visible reason to be there. Linux, macOS and Windows are unchanged.

## [0.1.2] — 2026-11-01

### Fixed
- Release assets are named from the git tag instead of the version constant, so `install.sh` and
  `install.ps1` find what they ask for. (0.1.1 published assets named `0.1.0`, which made the pin-the-
  latest-release path return HTTP 404.)
- Both installers now select the asset from the release's own file list rather than constructing a
  filename, so a naming mismatch degrades to a working install instead of a failed download.
- The reported version (`vibe-check --version`) matches the release that shipped it.
- CI refuses to publish a release whose assets are not named for the tag being built.

## [0.1.1] — 2026-11-01

### Fixed
- FreeBSD CPU model string: read `hw.model` through sysctl instead of gopsutil, whose FreeBSD path
  parses `/var/run/dmesg.boot` and returns nothing when that file is unreadable.
- The engine test suite compiles on macOS and Windows; the Linux-only socket scanner is now behind a
  Linux build tag and the ports test compares against `collectListeners()`, the entry point every
  platform actually uses.
- `install.ps1` reports failures by throwing rather than exiting, because `exit` inside `irm | iex`
  closes the caller's shell.
- `install.sh` prints its guidance when no release exists instead of dying inside a `pipefail` pipeline.
- The hardlink-dedup assertion skips on platforms with no inode fingerprint, and the README stopped
  claiming du-exact sizes everywhere because of it.

### Added
- CI runs the headless snapshot on each OS and verifies that hostname, kernel, CPU model, memory,
  process count and the disk crawl return plausible values — proof it reads the platform, not just that
  it compiles for it.

## [0.1.0] — 2026-11-01

Initial public release.

### Added
- Bubbletea/lipgloss TUI with five tabs (dashboard, processes, ports, docker, disk artifacts), diff-based
  dual-buffer rendering at up to 60 FPS, Unicode rounded borders, block-character gauges, three dark
  themes with automatic truecolor/256/16 fallback, optional mouse, pane zoom, `/` live fuzzy filter,
  `?` keymap drawer and a `Ctrl+p` command palette.
- Orphaned-process and memory-leak detector with one-keypress SIGTERM/SIGKILL, tunable so freshly
  reparented daemons are not flagged.
- Low-priority build-artifact crawler: zero-allocation `getdents64` reads on Linux, a shared duty-cycle
  limiter, incremental publishing, hardlink-aware sizes matching `du --apparent-size`, and a concurrent
  purge engine with progress.
- Port mapper over `/proc/net/tcp`/`tcp6` with socket-inode-to-PID resolution and non-blocking synthetic
  HTTP HEAD latency probes, TLS detection and explicit *not bound* rows for watched ports.
- Docker and Podman view over the raw unix socket / Windows named pipe, no SDK, including Colima and
  `~/.docker` endpoint discovery.
- FD and I/O pressure reporting from `/proc/sys/fs/file-nr` plus per-process FD sampling.
- Terminal state restoration on clean quit, `SIGINT`, `SIGTERM`, `SIGHUP` and recovered panics.
- `--json` headless snapshots, TOML configuration under `$XDG_CONFIG_HOME/vibe-check`, and `config init`.
- `install.sh` and `install.ps1` with SHA-256 verification against release checksums; binaries ship
  under both `vibe-check` and `vcheck`.
- Cross-compilation for Linux and macOS (amd64/arm64) and Windows (amd64/arm64).

[Unreleased]: https://github.com/gaurav-dhakal/vibe-check/compare/v0.1.3...HEAD
[0.1.3]: https://github.com/gaurav-dhakal/vibe-check/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/gaurav-dhakal/vibe-check/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/gaurav-dhakal/vibe-check/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/gaurav-dhakal/vibe-check/releases/tag/v0.1.0
