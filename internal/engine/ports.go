package engine

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// listenSock is one bound TCP socket as reported by the platform collector.
type listenSock struct {
	proto string // tcp | tcp6
	ip    net.IP
	port  uint16
	inode uint64 // Linux: socket inode behind the owner cache
	pid   int32  // owning process, resolved by the platform collector
}

func (e *Engine) updatePorts() {
	listens, err := e.collectListeners()
	if err != nil {
		return
	}
	// collectListeners already resolved ownership: by inode cache on Linux,
	// straight from the kernel API elsewhere.
	watch := make(map[uint16]bool)
	for _, p := range e.cfg.WatchPorts {
		watch[uint16(p)] = true
	}

	ports := make([]Port, 0, len(listens)+len(watch))
	seen := map[uint16]bool{}
	for _, l := range listens {
		p := Port{
			Proto:     l.proto,
			Addr:      l.ip.String(),
			Port:      l.port,
			State:     "LISTEN",
			Monitored: watch[l.port],
		}
		if l.pid > 0 {
			p.Pid = l.pid
			p.ProcName = e.procNameOf(l.pid)
		}
		seen[l.port] = true
		ports = append(ports, p)
	}
	for _, p := range e.cfg.WatchPorts {
		port := uint16(p)
		if !seen[port] {
			ports = append(ports, Port{Proto: "tcp", Addr: "—", Port: port, State: "not bound", Monitored: true})
			seen[port] = true
		}
	}

	e.probePorts(&ports)
	sort.Slice(ports, func(i, j int) bool { return ports[i].Port < ports[j].Port })
	e.ports.Store(&ports)
}

// procNameOf looks up a cached process name for the ports table.
func (e *Engine) procNameOf(pid int32) string {
	for _, p := range *e.procs.Load() {
		if p.Pid == pid {
			return p.Name
		}
	}
	return ""
}

var probeClient = &http.Client{
	Transport: &http.Transport{
		DisableKeepAlives:   true, // one HEAD per port: avoids unsolicited-response noise from non-HTTP servers (CUPS/IPP etc.)
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		TLSHandshakeTimeout: time.Second,
	},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func (e *Engine) probePorts(ports *[]Port) {
	timeout := time.Duration(e.cfg.HTTPProbeTimeoutMS) * time.Millisecond
	sem := make(chan struct{}, 32)
	var wg sync.WaitGroup
	for i := range *ports {
		p := &(*ports)[i]
		if p.State != "LISTEN" && !p.Monitored {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(p *Port) {
			defer wg.Done()
			defer func() { <-sem }()
			p.HTTP = e.probeOne(p, timeout)
		}(p)
	}
	wg.Wait()
}

func (e *Engine) probeOne(p *Port, timeout time.Duration) *HTTPProbe {
	host := p.Addr
	if host == "" || host == "—" || host == "::" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	tlsPort := p.Port == 443 || p.Port == 8443 || p.Port == 9443 || p.Port == 3443
	schemes := []string{"http"}
	if tlsPort {
		schemes = []string{"https", "http"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for _, scheme := range schemes {
		if scheme == "https" && !tlsPort {
			// upgrade fallback: cheap port sniff first
			if !looksTLS(p.Port) {
				continue
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodHead,
			fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, strconv.Itoa(int(p.Port)))), nil)
		if err != nil {
			continue
		}
		start := time.Now()
		resp, err := probeClient.Do(req)
		if err != nil {
			if scheme == "http" && !tlsPort {
				// retry once over https for servers that hard-redirect
				if strings.Contains(err.Error(), "400") || strings.Contains(err.Error(), "malformed") {
					continue
				}
			}
			continue
		}
		resp.Body.Close()
		probe := &HTTPProbe{OK: true, Status: resp.StatusCode, LatencyMs: float64(time.Since(start).Microseconds()) / 1000, Scheme: scheme, CheckedAt: time.Now()}
		return probe
	}
	return &HTTPProbe{OK: false, LatencyMs: 0, CheckedAt: time.Now()}
}

func looksTLS(port uint16) bool {
	switch port {
	case 443, 8443, 9443, 3443, 6443:
		return true
	}
	return false
}

// portTable is used by headless JSON export (kept out of UI deps).
func PortRows(p Port) string {
	if p.HTTP != nil && p.HTTP.OK {
		return fmt.Sprintf("%d %s %d %.0fms", p.Port, p.HTTP.Scheme, p.HTTP.Status, p.HTTP.LatencyMs)
	}
	return fmt.Sprintf("%d", p.Port)
}

// inodeSig fingerprints a listener inode set (order independent).
