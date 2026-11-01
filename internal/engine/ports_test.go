package engine

import (
	"testing"
)

func TestScanListeningSocketsReal(t *testing.T) {
	got, err := scanListeningSockets()
	if err != nil {
		t.Fatalf("scan err: %v", err)
	}
	t.Logf("listeners found: %d", len(got))
	for _, l := range got {
		t.Logf("  %s %s:%d inode=%d", l.proto, l.ip, l.port, l.inode)
	}
	if len(got) == 0 {
		t.Errorf("expected at least one LISTEN socket on a normal desktop, got 0")
	}
}
