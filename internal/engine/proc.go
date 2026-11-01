//go:build linux

package engine

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Walking every /proc/<pid>/fd is by far the most expensive part of the proc
// scan, and the dashboard only needs the aggregate — so sample it on a slow
// cadence and reuse the per-pid counts between samples.
const fdSampleEvery = 5

// initAlwaysParent: on Linux pid 1 is always present, so ppid==1 alone is the
// orphan signal and the extra table lookup would be pure overhead.
func initAlwaysParent(int32) bool { return true }

// updateProcs does one pass over /proc: two file reads per pid (stat,
// cmdline) gives us ppid, rss, cpu, threads, state, age — then diffs
// jiffies against the previous tick for htop-style CPU%.
func (e *Engine) updateProcs() {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return // non-Linux fallback: gopsutil list is too slow for 1s ticks
	}
	uptimeSec := procUptimeSec()
	totalTicks := procTotalJiffies()

	newPrev := make(map[int32]procTick, len(e.proc.prev)+64)
	procs := make([]Proc, 0, len(e.proc.prev)+64)
	var orphans int
	var orphanRSS uint64
	var fdSum uint64
	var fdReadable int

	deltat := uint64(0)
	if e.proc.haveTotal && totalTicks > e.proc.prevTotal {
		deltat = totalTicks - e.proc.prevTotal
	}

	e.proc.ticks++
	sampleFD := e.proc.ticks%fdSampleEvery == 1

	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		pid, err := strconv.ParseInt(ent.Name(), 10, 32)
		if err != nil {
			continue
		}
		base := filepath.Join("/proc", ent.Name())
		stat, err := os.ReadFile(filepath.Join(base, "stat"))
		if err != nil {
			continue
		}
		p, ok := parseProcStat(string(stat), int32(pid))
		if !ok {
			continue
		}

		prev, seen := e.proc.prev[p.Pid]
		sameInstance := seen && prev.start == p.startTicks
		if sameInstance {
			// cmdline/uid are stable for a process instance; /proc can be
			// noisy, so only re-read them when the pid is new or reused.
			p.CmdLine = prev.cmd
			p.UID = prev.uid
		} else {
			p.CmdLine = procCmdline(p.Pid)
			p.UID = procUID(p.Pid)
		}

		p.AgeSec = ageSec(uptimeSec, p.startTicks)

		cur := uint64(p.utime + p.stime)
		if sameInstance && deltat > 0 {
			p.CPUPercent = float64(cur-prev.cpuTicks) / float64(deltat) * 100 * float64(e.sysNumCPU())
		}
		p.RSS = rssBytes(p.rssPages)
		p.User = e.userName(p.UID)
		e.orphanFlags(&p.Proc, initAlwaysParent)

		if p.Orphan {
			orphans++
			orphanRSS += p.RSS
		}
		fd := -1
		if sampleFD {
			fd = countFDs(base)
			if fd >= 0 {
				fdSum += uint64(fd)
				fdReadable++
			}
		} else if seen {
			if prev.fd >= 0 {
				fdSum += uint64(prev.fd)
				fdReadable++
			}
		}

		newPrev[p.Pid] = procTick{cpuTicks: cur, start: p.startTicks, cmd: p.CmdLine, uid: p.UID, fd: fd}
		procs = append(procs, p.Proc)
	}

	sort.Slice(procs, func(i, j int) bool { return procs[i].CPUPercent > procs[j].CPUPercent })

	e.proc.prev = newPrev
	e.proc.prevTotal = totalTicks
	e.proc.haveTotal = true
	e.procs.Store(&procs)
	e.procTotal.Store(int64(len(procs)))
	e.orphanCount.Store(int64(orphans))
	e.orphanRSS.Store(orphanRSS)
	e.fdSum.Store(fdSum)
	e.fdReadable.Store(int64(fdReadable))
}

// parsed holds raw stat fields before conversion.
type rawProc struct {
	Proc
	utime      uint64
	stime      uint64
	startTicks uint64
	rssPages   uint64
}

// parseProcStat splits /proc/[pid]/stat robustly: comm can contain
// spaces and parentheses, so we anchor on the LAST ')'.
func parseProcStat(line string, pid int32) (rawProc, bool) {
	open := strings.IndexByte(line, '(')
	close := strings.LastIndexByte(line, ')')
	if open < 0 || close <= open || close == len(line)-1 {
		return rawProc{}, false
	}
	name := line[open+1 : close]
	fields := strings.Fields(line[close+1:])
	// fields[0]=state(3) ... index = fieldNo-3
	if len(fields) < 22 {
		return rawProc{}, false
	}
	num := func(i int) uint64 {
		v, _ := strconv.ParseUint(fields[i], 10, 64)
		return v
	}
	numi := func(i int) int64 {
		v, _ := strconv.ParseInt(fields[i], 10, 64)
		return v
	}
	r := rawProc{}
	r.Pid = pid
	r.Name = name
	r.State = fields[0]
	ppid, _ := strconv.ParseInt(fields[1], 10, 32)
	r.PPid = int32(ppid)
	r.Threads = int(numi(17))
	r.startTicks = num(19)
	vsize, _ := strconv.ParseUint(fields[20], 10, 64)
	_ = vsize
	r.rssPages = num(21)
	r.utime = num(11)
	r.stime = num(12)
	r.RSS = rssBytes(r.rssPages)
	return r, true
}

func procCmdline(pid int32) string {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(int(pid)), "cmdline"))
	if err != nil {
		return ""
	}
	s := strings.ReplaceAll(string(data), "\x00", " ")
	s = strings.TrimSpace(s)
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

func procUptimeSec() float64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	f := strings.Fields(string(data))
	if len(f) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[0], 64)
	return v
}

func procTotalJiffies() uint64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		var total uint64
		for _, f := range strings.Fields(line)[1:] {
			v, _ := strconv.ParseUint(f, 10, 64)
			total += v
		}
		return total
	}
	return 0
}

// ageSec converts starttime (jiffies since boot) to wall-clock age.
func ageSec(uptimeSec float64, startTicks uint64) float64 {
	return uptimeSec - float64(startTicks)/userHZ
}

var userHZ = detectHZ()

func detectHZ() float64 {
	// USER_HZ is 100 on essentially every Linux; respect it if set.
	if v := os.Getenv("VIBE_CHECK_HZ"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return 100.0
}

func rssBytes(pages uint64) uint64 { return pages * uint64(os.Getpagesize()) }

// procUID reads the effective uid from /proc/[pid]/status.
func procUID(pid int32) uint32 {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(int(pid)), "status"))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "Uid:"); ok {
			f := strings.Fields(v)
			if len(f) > 0 {
				u, _ := strconv.ParseUint(f[0], 10, 32)
				return uint32(u)
			}
		}
	}
	return 0
}

func countFDs(base string) int {
	ents, err := os.ReadDir(filepath.Join(base, "fd"))
	if err != nil {
		return -1
	}
	return len(ents)
}
