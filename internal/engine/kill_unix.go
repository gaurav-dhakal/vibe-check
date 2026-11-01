//go:build unix

package engine

import "syscall"

// signalProcess uses real POSIX signals, so force=false gives the process a
// chance to flush and exit cleanly.
func signalProcess(pid int32, force bool) (string, error) {
	sig, label := syscall.SIGTERM, "SIGTERM"
	if force {
		sig, label = syscall.SIGKILL, "SIGKILL"
	}
	return label, syscall.Kill(int(pid), sig)
}
