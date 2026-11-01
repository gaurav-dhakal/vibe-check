# Security policy

## What this binary can do

`vibe-check` runs with your privileges and is deliberately able to act on your machine. That is the
product, so treat these as the attack surface:

| Capability | Path | Blast radius |
|---|---|---|
| Signal any process you own | `x` / `X`, *kill all orphans* | kills work you did not expect to lose |
| Delete directories | `x` on a disk artifact | irreversible; asks for confirmation and only lists paths the crawler found |
| Start/stop containers | `x` on the docker tab | local runtime socket only |
| Read command lines, sockets, disk metadata | every collector | shown to you and copied by `y`/`Y` to your clipboard |
| Read the container API socket | docker engine | the docker socket is root-equivalent on most hosts; the tool only ever reads it or calls start/stop |

There is no inbound surface: the tool listens on nothing, and it makes no outbound connections. The
only traffic it generates is HTTP HEAD probes to sockets already listening on your own machine, used
for the latency column.

## What is guaranteed

- No telemetry, no analytics, no update checks, no remote code paths.
- Artifact bytes, listener ownership and container state are read-only queries; nothing is written
  outside the purge action you confirm.
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
