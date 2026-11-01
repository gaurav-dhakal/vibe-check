//go:build unix

package engine

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

// findSocket walks the well-known Docker/Podman socket locations. The engine
// speaks HTTP over the socket directly, so no docker CLI or SDK is needed.
func findSocket() (string, error) {
	var candidates []string
	if xrd := os.Getenv("XDG_RUNTIME_DIR"); xrd != "" {
		candidates = append(candidates,
			filepath.Join(xrd, "docker.sock"),
			filepath.Join(xrd, "podman", "podman.sock"))
	}
	home := os.Getenv("HOME")
	extra := []string{
		"/var/run/docker.sock", "/run/docker.sock",
		"/run/user/1000/docker.sock",
		"/run/podman/podman.sock", "/var/run/podman/podman.sock",
	}
	if home != "" {
		extra = append(extra,
			filepath.Join(home, ".colima/default/docker.sock"),
			filepath.Join(home, ".docker/run/docker.sock"),
			filepath.Join(home, ".podman/podman.sock"))
	}
	candidates = append(candidates, extra...)
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.Mode()&os.ModeSocket != 0 {
			return c, nil
		}
	}
	return "", errNoSocket
}

func runtimeLabel() string { return "unix" }

// newSocketClient points an HTTP client at the socket: Host is irrelevant on a
// unix dial, so the transport ignores the URL and always connects locally.
func newSocketClient(sock string) *http.Client {
	dialer := func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}
	return &http.Client{Transport: &http.Transport{DialContext: dialer}}
}
