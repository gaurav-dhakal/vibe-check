package main

import (
	"runtime/debug"

	"github.com/gaurav-dhakal/vibe-check/cmd"
)

func main() {
	// A monitor must cost less than the machines it watches: keep the heap
	// small and the GC eager so long sessions never bloat.
	debug.SetGCPercent(40)
	debug.SetMemoryLimit(96 << 20)
	cmd.Execute()
}
