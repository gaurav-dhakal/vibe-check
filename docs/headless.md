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

## Recipes

```sh
# reclaimable bytes from orphaned dev servers
vcheck --json | jq '[.procs[] | select(.orphan)] | map(.rss) | add'

# who owns port 3000, and is it answering HTTP
vcheck --json | jq '.ports[] | select(.port == 3000)'

# the ten biggest node_modules directories
vcheck --json | jq -r '.artifacts[] | select(.kind == "node_modules")
  | "\(.size)\t\(.path)"' | sort -rn | head

# fail a CI step when a watch port is not bound
vcheck --json | jq -e '.ports[] | select(.port == 8080 and .monitored)' >/dev/null
```
