//go:build windows

package cmd

import "os"

// exitSignals: Windows delivers no SIGHUP and only emulates SIGTERM, so the
// console control events are what actually reach us.
func exitSignals() []os.Signal { return []os.Signal{os.Interrupt} }
