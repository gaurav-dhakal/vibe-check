//go:build windows

package engine

import "os"

// signalProcess: Windows has no SIGTERM for GUI/console processes, so both
// modes terminate the process handle; the label keeps the toast honest.
func signalProcess(pid int32, force bool) (string, error) {
	p, err := os.FindProcess(int(pid))
	if err != nil {
		return "", err
	}
	if !force {
		// Ctrl-C equivalent: only reaches console apps, and failing here is
		// normal, so fall through to the hard kill.
		if e := p.Signal(os.Interrupt); e == nil {
			return "CTRL_C", nil
		}
	}
	return "TERMINATE", p.Kill()
}
