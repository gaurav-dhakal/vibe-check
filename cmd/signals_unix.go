//go:build unix

package cmd

import (
	"os"
	"syscall"
)

// exitSignals are the ones that must restore the terminal before the process
// goes: a TUI leaving the shell in raw mode is the classic bad utility.
func exitSignals() []os.Signal {
	return []os.Signal{syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT}
}
