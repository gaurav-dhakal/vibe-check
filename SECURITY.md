# Security policy

## What this binary can do

`vibe-check` runs with your privileges and is deliberately able to act on your machine. That is the
product, so treat these as the attack surface:

| Capability | Path | Blast radius |
|---|---|---|
| Signal any process you own | `x` / `X`, *kill all orphans* | kills work you did not expect to lose |
| Delete directories | `x` on a disk artifact | irreversible; asks for confirmation and only lists paths the crawler found |
| Uninstall packages, as root where the manager demands it | `x` / `X` on the apps tab | runs `apt-get remove/purge`, `dnf`/`yum remove`, `rpm -e`, `pacman -Rns`, `snap remove [--purge]`, `flatpak uninstall [--delete-data]`, `brew uninstall [--purge/--zap]`, `winget uninstall` — elevated through `pkexec` (or `sudo -n`) only when the manager needs root, and `X` deletes the app's config/data too |
| Install from a file you point at | `i` on the apps tab | runs the host installer on that file, extracts archives under `~/Apps`, and for an AppImage writes an executable plus a `.desktop` tray entry |
| Drop the kernel page cache | `b`, `c` | evicts clean/dirty cache pages; no data loss, but it re-warms disk for a while, and it is elevated |
| Start/stop containers | `x` on the docker tab | local runtime socket only |
| Read command lines, sockets, disk metadata, package file lists | every collector | shown to you and copied by `y`/`Y` to your clipboard |
| Read the container API socket | docker engine | the docker socket is root-equivalent on most hosts; the tool only ever reads it or calls start/stop |

There is no inbound surface: the tool listens on nothing, and it makes no outbound connections. The
only traffic it generates is HTTP HEAD probes to sockets already listening on your own machine, used
for the latency column.

## What is guaranteed

- No telemetry, no analytics, no update checks, no remote code paths.
- Artifact bytes, listener ownership, container state and package inventories are read-only queries.
  Everything that writes — purge, uninstall, install, kill — is a key you pressed, and destructive ones
  show you the target list first.
- Elevation is `pkexec` when polkit is available and `sudo -n` otherwise, never `sudo -S` or a hidden
  prompt: a privileged action either gets a real authorisation prompt or fails with a toast.
- Archive installers refuse an entry that resolves outside the destination directory (zip-slip), and
  the whole extract aborts rather than half-writing it.
- `--json` writes only to stdout; terminal restore sequences go to the tty, so piping cannot corrupt
  your shell.
- Release assets are published with `checksums.txt`, and both installers refuse to install an artifact
  whose SHA-256 does not match.

## Reporting a vulnerability

Please use [GitHub private vulnerability reporting](https://github.com/gaurav-dhakal/vibe-check/security/advisories/new)
rather than a public issue. If you cannot access that, open a normal issue that says only "security:
see private report" and file the advisory.

Include the version (`vibe-check --version`), the OS and architecture, and the output of
`vibe-check --json` with anything sensitive removed.

## Supported versions

| Version | Supported |
|---|---|
| 0.1.x | yes |
| anything older | upgrade; release assets for older tags may lack current fixes |

Because this is a local tool with no server component, the practical fix for most findings is "upgrade
to the latest release", so version hygiene matters more here than a long backport tail.
