package engine

import "golang.org/x/sys/unix"

// cpuModelName reads hw.model straight from the kernel. gopsutil's FreeBSD path
// starts by parsing /var/run/dmesg.boot and returns an error when that is
// unreadable — which is exactly what happens in a minimal or non-root install —
// so it cannot be relied on for a string sysctl answers directly.
func cpuModelName() string {
	name, err := unix.Sysctl("hw.model")
	if err != nil {
		return ""
	}
	return trimModel(name)
}
