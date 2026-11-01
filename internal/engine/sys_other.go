//go:build !linux

package engine

// readFDs has no portable kernel source: /proc/sys/fs/file-nr is Linux, and
// macOS/BSD hide the same counter behind sysctl. The per-process fd sampler
// still fills the columns that do not need it, so leave the totals at zero and
// let the pane render a dash rather than a wrong number.
func (e *Engine) readFDs(ns *System) {}
