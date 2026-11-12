# Headless output (`--json`)

`--json` writes one snapshot to stdout and nothing else — restore sequences go to the tty, so
`vibe-check --json | jq` stays clean. The disk scan runs unthrottled; a complete snapshot returns in
about 1.6 s.

## Fields

```
{
  "at":          timestamp,
  "sys":         hostname, os, kernel, uptime_ns, num_cpu, cpu_model, cpu_percent,
                 core_percents[], load_1/5/15, mem_used/total/avail/percent,
                 swap_used/total, disk_read_bps, disk_write_bps, net_recv_bps,
                 net_sent_bps, proc_total, fd_alloc, fd_count, fd_max, fd_per_proc,
                 disks[]{mount,fstype,total,used,percent}, core_mhz[],
                 orphan_count, orphan_rss
  "procs":       pid, ppid, uid, user, name, cmdline, state, rss, cpu_percent,
                 age_sec, threads, orphan
  "ports":       proto, addr, port, state, pid, proc_name, monitored, http{...}
  "containers":  docker/podman view, empty when no runtime socket exists
  "gpus":        vendor, name, util_percent, mem_used, mem_total, temp_c, has_temp
                 (omitted when the machine exposes no readable GPU)
  "artifacts":   path, project, size, files, kind
  "docker_err":  why the container engine has nothing, when it is empty
  "scan_state":  scanning, dirs_seen, last_done_at
}
```

## Recipes

```sh
# reclaimable bytes from orphaned dev servers
vcheck --json | jq '[.procs[] | select(.orphan)] | map(.rss) | add'

# who owns port 3000, and is it answering HTTP
vcheck --json | jq '.ports[] | select(.port == 3000)'

# the ten biggest node_modules directories
vcheck --json | jq -r '.artifacts[] | select(.kind == "node_modules")
  | "\(.size)\t\(.path)"' | sort -rn | head

# is any GPU pinned while the tests run
vcheck --json | jq -r '.gpus[]? | "\(.name) \(.util_percent)% \(.mem_used)/\(.mem_total)"'

# fail a CI step when a watch port is not bound
vcheck --json | jq -e '.ports[] | select(.port == 8080 and .monitored)' >/dev/null
```

## Commands without the TUI

Every action the apps tab and the reclaim panel expose has a subcommand, so nothing you can do with a
key needs a terminal that is wide enough to see it in.

```
vibe-check apps list [--filter <substr>] [--json]   # apt dnf rpm pacman snap flatpak brew winget npm appimage
vibe-check apps files <name> [--json]               # every path the app owns + file/folder/byte totals
vibe-check apps remove <name> --yes                 # the manager's own uninstall
vibe-check apps remove <name> --purge --yes         # …and its files and settings
vibe-check apps install <path>                      # .deb .rpm .tar.gz .tar.xz .AppImage (per-platform more)
vibe-check apps managers                            # which managers this host reports
vibe-check boost                                    # drop the page cache; prints free RAM before → after
vibe-check autoclean [on|off]                       # read or write the config toggle
```

Behaviour notes:

- `apps remove` without `--yes` is a refusal, not a prompt: automation should decide, not discover a
  hung process waiting on stdin.
- A name is matched exactly first, then as a unique substring; an ambiguous name errors with the
  candidates instead of picking one.
- Elevation follows the TUI: `pkexec`, else `sudo -n` — one visible prompt, and it fails rather than
  asking for a password it cannot show.
- `--json` on the apps subcommands emits the same shapes as `vibe-check --json` sections.
