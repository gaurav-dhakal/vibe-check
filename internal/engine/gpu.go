package engine

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
)

// GPU stats come from whatever the platform actually exposes: nvidia-smi when
// present, sysfs busy/vram files on Linux, and nothing elsewhere — the
// dashboard then keeps the runtime pane instead of inventing numbers.
func (e *Engine) updateGPUs() {
	gpus := collectGPUs()
	e.gpus.Store(&gpus)
}

func probeNvidiaSMI() []GPU {
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil
	}
	out, err := exec.Command(path,
		"--query-gpu=name,utilization.gpu,memory.used,memory.total,temperature.gpu",
		"--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil
	}
	var gpus []GPU
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ", ")
		if len(fields) < 4 {
			continue
		}
		util, _ := strconv.ParseFloat(strings.TrimSpace(fields[1]), 64)
		used, _ := strconv.ParseUint(strings.TrimSpace(fields[2]), 10, 64)
		total, _ := strconv.ParseUint(strings.TrimSpace(fields[3]), 10, 64)
		g := GPU{
			Vendor:      "nvidia",
			Name:        strings.TrimSpace(fields[0]),
			UtilPercent: clampPct(util),
			MemUsed:     used * 1048576,
			MemTotal:    total * 1048576,
		}
		if len(fields) >= 5 {
			if t, err := strconv.ParseFloat(strings.TrimSpace(fields[4]), 64); err == nil {
				g.TempC, g.HasTemp = t, true
			}
		}
		gpus = append(gpus, g)
	}
	return gpus
}
