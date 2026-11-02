# Changelog

All notable changes to `vibe-check` are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); the project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Release binaries for every version are in [GitHub Releases](https://github.com/gaurav-dhakal/vibe-check/releases).

## [Unreleased]

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
- Termux support: `install.sh` detects Termux, installs into `$PREFIX/bin` (already on PATH) and prints
  the Android `/proc` visibility caveat; the static `linux/arm64` release asset runs unmodified.
- `Makefile` with `fmt vet lint test race build install release clean`; `make` resolves the version from
  the git tag exactly like the release script, so a local build reports what a released one would.
- Issue forms for bugs and feature requests, a pull request template, and Dependabot for Go modules and
  GitHub Actions.
- `.editorconfig`, so tabs in Makefiles and two-space indentation elsewhere are a repo rule instead of a
  per-editor preference.

### Changed
- README install section gives every method its own labelled code box (curl, Homebrew, PowerShell,
  winget, Termux) plus an uninstall section, and `demo.gif` re-records the first-run theme choice.
- `.gitignore` no longer leaves CI's `snapshot.json` artifact in the working tree.

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
