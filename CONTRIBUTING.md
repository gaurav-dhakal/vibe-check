# Contributing to vibe-check

Thanks for taking an interest. This is a small, opinionated codebase; the notes below exist so your
change lands on the first try.

## Ground rules

1. **A number on screen must come from the kernel or an API.** If a platform has no source for a
   metric, render a dash — never an estimate, never a Linux value reused somewhere it does not apply.
2. **The UI never blocks on collection.** Collectors publish immutable snapshots through atomics; if
   your change makes the render loop wait on I/O, it is the wrong change.
3. **Politeness over the machine.** The disk crawler is duty-cycle limited for a reason. Do not raise
   its budget to make a benchmark look better.
4. Comments explain *why* — a constraint, an off-by-one that bit us, a kernel quirk. No narration of
   what the next line obviously does.

## Environment

Go 1.27.3 or newer — whatever the `go` directive in `go.mod` says is what CI uses. Nothing else: no
Docker, no database, no network at build or run time.

```sh
git clone https://github.com/gaurav-dhakal/vibe-check.git
cd vibe-check
make            # lint, test, build
```

| Target | What it does |
|---|---|
| `make fmt` | fails if anything is not gofmt-clean |
| `make vet` | `go vet ./...` |
| `make lint` | both of the above |
| `make test` | full suite, no cache |
| `make race` | full suite under the race detector |
| `make build` | `./vibe-check` |
| `make release` | cross-built `dist/` assets plus `checksums.txt` |
| `make install` | into `~/.local/bin`, with the `vcheck` alias |

## Where things live

```
cmd/                 cobra entry point, flag wiring, terminal restoration
internal/engine/     collectors: procs, ports, docker, artifacts, sys, kill, clipboard
internal/engine/*_linux.go, *_other.go, *_unix.go, *_windows.go
                     platform splits chosen by build tags — never runtime OS sniffing
internal/ui/         Bubbletea model, views, overlays, chrome
internal/theme/      palettes and colour-depth negotiation
internal/config/     TOML surface and defaults
scripts/             release build and the snapshot verifier used by CI
```

## Adding a metric

1. Collect it in `internal/engine`, into the struct that already owns the domain.
2. Add the field to `internal/engine/types.go` with a `json:"…"` tag — the headless output and the UI
   read the same snapshot, so there is exactly one place a number can be wrong.
3. Render it. Gauges use block characters; keep the 60-30-10 colour rule (one accent, not six).
4. If it came from parsing, add a test that pins it against the tool a human would use (`du`, `ss`,
   `/proc` files). `internal/engine/rawdir_test.go` is the model to copy.

## Adding a platform

Copy the pattern in `sys_other.go` / `cpuname_other.go`: a build tag, the same function signatures, and
an honest comment about what is unavailable. Then add the target to `scripts/build-release.sh` and the
cross list in `.github/workflows/ci.yml`. CI's per-OS job runs the headless snapshot and pipes it to
`scripts/verify-snapshot.awk`, so the platform is not "supported" until that verifier passes on a real
runner.

## Testing changes you can see

`go run . --json` proves the data. For the interface, run it in a real terminal — type, zoom, filter,
kill, purge, resize the window, and quit with `Ctrl+c` rather than `q` to confirm the terminal comes
back intact. Screenshots and recordings in PRs are welcome, mockups are not.

## Pull requests

Use the template. Before pushing:

```sh
make lint test race
```

One logical change per PR. Behaviour changes get a `CHANGELOG.md` entry under *Unreleased*. If a test
was failing on a runner, say so in the PR body rather than marking it skipped silently — a skip that
hides a platform bug is worse than a red build.

## Commit messages

Imperative, and about the *why*: `engine: sample FDs only for processes that survived the last tick`.
The body exists to record what the measurements said, since that is the information a future reader
cannot recover from the diff.
