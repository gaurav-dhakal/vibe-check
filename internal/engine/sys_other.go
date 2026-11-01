//go:build !linux

package engine

import (
	"context"
	"strings"

	"github.com/shirou/gopsutil/v4/cpu"
)

// readFDs has no portable kernel source: /proc/sys/fs/file-nr is Linux, and
// macOS/BSD hide the same counter behind sysctl. The per-process fd sampler
// still fills the columns that do not need it, so leave the totals at zero and
// let the pane render a dash rather than a wrong number.
func (e *Engine) readFDs(ns *System) {}

// cpuModelName asks the platform instead of /proc/cpuinfo.
func cpuModelName() string {
	infos, err := cpu.InfoWithContext(context.Background())
	if err != nil || len(infos) == 0 {
		return ""
	}
	name := strings.TrimSpace(infos[0].ModelName)
	if len(name) > 42 {
		name = name[:42] + "…"
	}
	return name
}
