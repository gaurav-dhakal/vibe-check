//go:build !linux

package engine

import (
	"context"
	"net"
	"strconv"
	"time"

	gnet "github.com/shirou/gopsutil/v4/net"
)

// collectListeners uses the platform's connection table directly. gopsutil
// already returns the owning pid, so there is no fd-walk to cache here.
func (e *Engine) collectListeners() ([]listenSock, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conns, err := gnet.ConnectionsWithContext(ctx, "tcp")
	if err != nil {
		return nil, err
	}
	out := make([]listenSock, 0, len(conns))
	for _, c := range conns {
		if status := normalizeState(c.Status); status != "LISTEN" {
			continue
		}
		ip := net.ParseIP(c.Laddr.IP)
		if ip == nil {
			ip = net.IPv4zero
		}
		proto := "tcp"
		if ip.To4() == nil {
			proto = "tcp6"
		}
		out = append(out, listenSock{
			proto: proto, ip: ip, port: uint16(c.Laddr.Port), pid: c.Pid,
		})
	}
	return dedupe(out), nil
}

// normalizeState folds per-platform status spellings onto the /proc names the
// ports table shows, so both collectors feed one identical view.
func normalizeState(s string) string {
	if s == "LISTENING" { // Windows
		return "LISTEN"
	}
	return s
}

// dedupe drops (proto, ip, port) repeats: Windows and macOS both surface a
// dual-stack listener once per address family.
func dedupe(in []listenSock) []listenSock {
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, l := range in {
		k := l.proto + "|" + l.ip.String() + "|" + strconv.FormatUint(uint64(l.port), 10)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, l)
	}
	return out
}
