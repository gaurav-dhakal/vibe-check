# Development

```sh
make lint        # gofmt + go vet
make test        # suite (engine parity + UI)
make race        # suite under -race
make build       # ./vibe-check
make release     # dist/ assets for every platform + checksums.txt
```

## Layout

`cmd/` is the cobra entry point and terminal-safety net, `internal/engine` holds the collectors
(one file per platform split where the kernel differs), `internal/ui` the Bubbletea model and views,
`internal/theme` the palettes, `internal/config` the TOML surface. Platform-specific code is chosen by
build tags, never by runtime OS sniffing.

## Platforms

| OS | Notes |
|---|---|
| Linux | Full feature set and the `/proc` fast path: per-process CPU, FD counts, socket-inode ownership, `file-nr` limits, zero-allocation directory reads |
| macOS | Kernel collectors through gopsutil — per-process CPU, memory, listeners with PID ownership, orphan detection, disk crawl |
| Windows | Listener enumeration without `netstat`, Docker over the named pipe, kills through the process API (no POSIX signals); hardlinked files count once per name, because NTFS gives no cheap inode fingerprint |
| Termux (Android) | Runs the static `linux/arm64` build. The dashboard, your own process stats and the disk crawl work out of the box; Android hides other processes' `/proc` entries and `/proc/net` from unprivileged apps, so ports, orphans and other-process CPU render as dashes until you run as root (`pkg install tsu && tsu`) |

## CI evidence

CI proves the platforms rather than asserting them. Every platform job builds, vets, runs the suite,
then executes the headless snapshot **on that OS** and fails unless hostname, kernel, CPU model,
memory, process count and the disk crawl come back with plausible values
(`scripts/verify-snapshot.awk`). The run on `main` reported an `Apple M1` with 564 processes on macOS,
an `AMD EPYC 9V74` with 152 on Windows, and an `Intel Xeon` with 187 on Linux — collected, not asserted.
