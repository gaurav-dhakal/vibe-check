//go:build !linux

package engine

import (
	"context"

	"github.com/shirou/gopsutil/v4/cpu"
)

// cpuModelName asks the platform API instead of /proc/cpuinfo.
func cpuModelName() string {
	infos, err := cpu.InfoWithContext(context.Background())
	if err != nil || len(infos) == 0 {
		return ""
	}
	return trimModel(infos[0].ModelName)
}
