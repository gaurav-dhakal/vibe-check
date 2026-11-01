//go:build linux

package engine

import (
	"os"
	"strconv"
	"strings"
)

func (e *Engine) readFDs(ns *System) {
	data, err := os.ReadFile("/proc/sys/fs/file-nr")
	if err != nil {
		return // non-Linux: gopsutil has no portable fd source
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 3 {
		ns.FDAlloc, _ = strconv.ParseUint(fields[0], 10, 64)
		ns.FDMax, _ = strconv.ParseUint(fields[2], 10, 64)
	}
	ns.FDCount = int(e.fdReadable.Load())
	if n := e.fdReadable.Load(); n > 0 {
		ns.FDPerProc = int(e.fdSum.Load() / uint64(n))
	}
}

// cpuModelName reads the first "model name" line: the cheapest exact string
// the kernel offers, and the one /proc has always had.
func cpuModelName() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		name, ok := strings.CutPrefix(line, "model name")
		if !ok {
			continue
		}
		return trimModel(strings.TrimLeft(name, " :\t"))
	}
	return ""
}
