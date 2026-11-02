//go:build linux

package engine

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func collectGPUs() []GPU {
	if gpus := probeNvidiaSMI(); len(gpus) > 0 {
		return gpus
	}
	var gpus []GPU
	cards, _ := filepath.Glob("/sys/class/drm/card[0-9]*/device")
	for _, dev := range cards {
		driver := deviceDriver(dev)
		if driver == "" {
			continue
		}
		util, ok := readInt(filepath.Join(dev, "gpu_busy_percent"))
		if !ok {
			continue // this card exposes no honest utilization source
		}
		g := GPU{Vendor: driver, Name: driverName(driver), UtilPercent: clampPct(float64(util))}
		if used, uok := readInt(filepath.Join(dev, "mem_info_vram_used")); uok {
			if total, tok := readInt(filepath.Join(dev, "mem_info_vram_total")); tok {
				g.MemUsed, g.MemTotal = uint64(used), uint64(total)
			}
		}
		if t, tok := readInt(hwmonTemp(dev)); tok {
			g.TempC, g.HasTemp = float64(t)/1000, true
		}
		gpus = append(gpus, g)
	}
	return gpus
}

func deviceDriver(dev string) string {
	link, err := filepath.EvalSymlinks(filepath.Join(dev, "driver"))
	if err != nil {
		return ""
	}
	return filepath.Base(link)
}

func driverName(driver string) string {
	switch driver {
	case "amdgpu":
		return "AMD GPU (amdgpu)"
	case "i915":
		return "Intel GPU (i915)"
	default:
		return driver
	}
}

func hwmonTemp(dev string) string {
	hw, _ := filepath.Glob(filepath.Join(dev, "hwmon", "hwmon*", "temp1_input"))
	if len(hw) == 0 {
		return filepath.Join(dev, "no-temp")
	}
	return hw[0]
}

func readInt(path string) (int64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
