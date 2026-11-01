package engine

import (
	"context"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
)

type sysCounters struct {
	prevAt       time.Time
	prevDiskR    uint64
	prevDiskW    uint64
	prevNetR     uint64
	prevNetW     uint64
	haveCounters bool
}

// trimModel keeps a CPU signature short enough for the dashboard header.
func trimModel(name string) string {
	name = strings.TrimSpace(name)
	if len(name) > 42 {
		return name[:42] + "…"
	}
	return name
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func satSub(a, b uint64) uint64 {
	if a < b {
		return 0
	}
	return a - b
}

func (e *Engine) updateSys() {
	ctx := context.Background()
	cur := e.sys.Load()
	ns := *cur

	if ns.Hostname == "" {
		if hi, err := host.InfoWithContext(ctx); err == nil && hi != nil {
			ns.Hostname = hi.Hostname
			ns.OS = hi.OS
			ns.Kernel = hi.KernelVersion
		}
		ns.NumCPU = runtimeNumCPU()
		ns.CPUModel = cpuModelName()
	}

	if up, err := host.UptimeWithContext(ctx); err == nil {
		ns.Uptime = time.Duration(up) * time.Second
	}

	if p, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(p) > 0 {
		ns.CPUPercent = clampPct(p[0])
	}
	if cores, err := cpu.PercentWithContext(ctx, 0, true); err == nil {
		ns.CorePercents = cores
	}
	if la, err := load.AvgWithContext(ctx); err == nil && la != nil {
		ns.Load1, ns.Load5, ns.Load15 = la.Load1, la.Load5, la.Load15
	}
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil && vm != nil {
		ns.MemTotal, ns.MemUsed, ns.MemAvail = vm.Total, vm.Used, vm.Available
		ns.MemPercent = clampPct(vm.UsedPercent)
	}
	if sm, err := mem.SwapMemoryWithContext(ctx); err == nil && sm != nil {
		ns.SwapTotal, ns.SwapUsed = sm.Total, sm.Used
	}

	e.diffIO(ctx, &ns)
	e.readFDs(&ns)

	e.sys.Store(&ns)

	e.histMu.Lock()
	h := &e.history
	if len(h.CPUCores) == 0 && len(ns.CorePercents) > 0 {
		for range ns.CorePercents {
			h.CPUCores = append(h.CPUCores, NewRing(histLen))
		}
	}
	h.CPU.Push(ns.CPUPercent)
	h.Mem.Push(ns.MemPercent)
	h.NetR.Push(float64(ns.NetRecvBps))
	h.NetW.Push(float64(ns.NetSentBps))
	h.DiskR.Push(float64(ns.DiskReadBps))
	h.DiskW.Push(float64(ns.DiskWriteBps))
	for i, c := range ns.CorePercents {
		if i < len(h.CPUCores) {
			h.CPUCores[i].Push(clampPct(c))
		}
	}
	e.histMu.Unlock()
}

// isLoopback skips the interface that would otherwise dominate the byte
// counters: on Linux "lo", on macOS/BSD "lo0", on Windows "Loopback Pseudo-Interface 1".
func isLoopback(name string) bool {
	return name == "lo" || name == "lo0" ||
		strings.HasPrefix(name, "Loopback") || strings.HasPrefix(name, "dummy")
}

// diffIO converts monotonic gopsutil counters into per-second rates using
// the delta between successive ticks — no sleep-based sampling.
func (e *Engine) diffIO(ctx context.Context, ns *System) {
	now := time.Now()
	var dR, dW, nR, nW uint64
	if di, err := disk.IOCountersWithContext(ctx); err == nil {
		for _, d := range di {
			if strings.HasPrefix(d.Name, "ram") { // ramdisk: not real I/O
				continue
			}
			dR += d.ReadBytes
			dW += d.WriteBytes
		}
	}
	if ni, err := gnet.IOCountersWithContext(ctx, false); err == nil {
		for _, n := range ni {
			if isLoopback(n.Name) {
				continue
			}
			nR += n.BytesRecv
			nW += n.BytesSent
		}
	}
	sec := now.Sub(e.sysc.prevAt).Seconds()
	if e.sysc.haveCounters && sec > 0.05 {
		ns.DiskReadBps = uint64(float64(satSub(dR, e.sysc.prevDiskR)) / sec)
		ns.DiskWriteBps = uint64(float64(satSub(dW, e.sysc.prevDiskW)) / sec)
		ns.NetRecvBps = uint64(float64(satSub(nR, e.sysc.prevNetR)) / sec)
		ns.NetSentBps = uint64(float64(satSub(nW, e.sysc.prevNetW)) / sec)
	}
	e.sysc.prevDiskR, e.sysc.prevDiskW, e.sysc.prevNetR, e.sysc.prevNetW = dR, dW, nR, nW
	e.sysc.prevAt = now
	e.sysc.haveCounters = true
}

func runtimeNumCPU() int {
	n, err := cpu.Counts(true)
	if err != nil || n == 0 {
		return 1
	}
	return n
}
