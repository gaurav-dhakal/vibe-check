//go:build !linux

package engine

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// cpuScale converts gopsutil's CPU seconds to the integer deltas the shared
// tick math uses: 1e6 units per second keeps ~15 minutes of runtime in int64.
const cpuScale = 1_000_000

// updateProcs is the portable collector: gopsutil per-pid reads, with CPU%
// derived from cumulative-time deltas over wall-clock time so it matches the
// Linux path's one-core-relative scale instead of reporting lifetime averages.
func (e *Engine) updateProcs() {
	list, err := process.Processes()
	if err != nil {
		return
	}
	now := time.Now()
	wall := now.Sub(e.proc.prevWall).Seconds()
	e.proc.prevWall = now

	pids := make(map[int32]bool, len(list))
	for _, p := range list {
		pids[p.Pid] = true
	}

	newPrev := make(map[int32]procTick, len(list)+8)
	procs := make([]Proc, 0, len(list))
	var orphans int
	var orphanRSS uint64
	fallbackUser := currentUserName()

	for _, p := range list {
		pid := p.Pid
		ppid, err := p.Ppid()
		if err != nil {
			continue
		}
		name, _ := p.Name()
		stl, _ := p.Status()
		st := strings.Join(stl, " ")
		threads, _ := p.NumThreads()
		rss := uint64(0)
		if mi, err := p.MemoryInfo(); err == nil && mi != nil {
			rss = uint64(mi.RSS)
		}
		var cpuSecs float64
		if ts, err := p.Times(); err == nil && ts != nil {
			cpuSecs = ts.User + ts.System
		}
		ct, err := p.CreateTime()
		if err != nil {
			ct = 0
		}
		var uid uint32
		if uids, err := p.Uids(); err == nil && len(uids) > 0 {
			uid = uint32(uids[0])
		}

		cur := uint64(cpuSecs * cpuScale)
		ticks := uint64(ct / 10) // centiseconds: a stable-enough instance id
		tick := procTick{cpuTicks: cur, start: ticks, uid: uid, fd: -1}

		proc := Proc{
			Pid: pid, PPid: ppid, UID: uid,
			Name:  filepath.Base(name),
			State: stateName(st),
			RSS:   rss, Threads: int(threads),
		}
		if ct > 0 {
			proc.AgeSec = now.Sub(time.UnixMilli(ct)).Seconds()
		}
		if proc.Name == "" {
			proc.Name = "pid " + strconv.Itoa(int(pid))
		}
		if runtime.GOOS == "windows" {
			proc.User = fallbackUser
		} else {
			proc.User = e.userName(uid)
		}

		prev, seen := e.proc.prev[pid]
		if seen && prev.start == ticks {
			proc.CmdLine = prev.cmd
			tick.cmd = prev.cmd
		} else {
			cmd, _ := p.Cmdline()
			if len(cmd) > 160 {
				cmd = cmd[:160] + "…"
			}
			proc.CmdLine = cmd
			tick.cmd = cmd
		}
		if wall > 0.05 && seen {
			proc.CPUPercent = float64(cur-prev.cpuTicks) / cpuScale / wall * 100
		}
		e.orphanFlags(&proc, func(ppid int32) bool { return pids[ppid] })
		if proc.Orphan {
			orphans++
			orphanRSS += proc.RSS
		}
		newPrev[pid] = tick
		procs = append(procs, proc)
	}

	sort.Slice(procs, func(i, j int) bool { return procs[i].CPUPercent > procs[j].CPUPercent })

	e.proc.prev = newPrev
	e.proc.haveTotal = true
	e.procs.Store(&procs)
	e.procTotal.Store(int64(len(procs)))
	e.orphanCount.Store(int64(orphans))
	e.orphanRSS.Store(orphanRSS)
}

// stateName maps gopsutil status strings to the short codes the STATE column
// shows, keeping the Linux /proc letters so both paths read the same.
func stateName(s string) string {
	switch strings.ToLower(s) {
	case "running":
		return "R"
	case "sleeping", "idle":
		return "S"
	case "stopped":
		return "T"
	case "zombie":
		return "Z"
	case "wait":
		return "W"
	case "":
		return "?"
	}
	return s[:1]
}

func currentUserName() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return os.Getenv("USERNAME")
}
