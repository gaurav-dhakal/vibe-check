//go:build !linux

package engine

// Windows and macOS have no cheap, honest GPU source that doesn't require
// elevation or a driver SDK; only nvidia-smi on PATH yields real numbers,
// and the dashboard keeps the runtime pane when there is none.
func collectGPUs() []GPU { return probeNvidiaSMI() }
