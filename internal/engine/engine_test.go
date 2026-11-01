package engine

import (
	"context"
	"testing"
	"time"

	"github.com/gaurav-dhakal/vibe-check/internal/config"
)

func TestUpdatePortsViaEngine(t *testing.T) {
	cfg := config.Default()
	e := New(cfg)
	e.ctx = context.Background()
	e.updatePorts()
	s := e.Snapshot()
	raw, err := e.collectListeners()
	if err != nil {
		t.Fatalf("collectListeners: %v", err)
	}
	t.Logf("engine stored %d ports, kernel reported %d listeners", len(s.Ports), len(raw))
	if len(raw) == 0 {
		t.Skip("this host has no LISTEN sockets to check against")
	}
	if len(s.Ports) < len(raw) {
		t.Fatalf("engine stored %d ports but the kernel reported %d listeners", len(s.Ports), len(raw))
	}
	for _, p := range s.Ports {
		t.Logf("  %d %s http=%v", p.Port, p.Proto, p.HTTP)
	}
}

func TestUpdateProcsSysPortsRun(t *testing.T) {
	cfg := config.Default()
	cfg.ArtifactScanMS = 60000
	e := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Run(ctx)
	time.Sleep(2 * time.Second)
	s := e.Snapshot()
	if s.Sys.Hostname == "" {
		t.Errorf("sys loop failed: empty hostname")
	}
	if len(s.Procs) < 10 {
		t.Errorf("proc loop: %d procs", len(s.Procs))
	}
	if len(s.Ports) == 0 {
		t.Errorf("port loop: 0 ports")
	}
}
