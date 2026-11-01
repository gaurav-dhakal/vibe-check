//go:build windows

package engine

import (
	"context"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// Docker Desktop and Podman-machine expose the same HTTP API over a Windows
// named pipe; Go's net stack cannot dial "npipe" without cgo-free helpers, but
// the pipe is openable as a file, which is enough for a request/response
// transport.
const dockerPipe = `\\.\pipe\docker_engine`

func findSocket() (string, error) {
	if host := os.Getenv("DOCKER_HOST"); strings.HasPrefix(host, "tcp://") {
		return host, nil
	}
	if _, err := os.Stat(dockerPipe); err == nil {
		return dockerPipe, nil
	}
	return "", errNoSocket
}

func runtimeLabel() string { return "windows" }

func newSocketClient(endpoint string) *http.Client {
	if strings.HasPrefix(endpoint, "tcp://") {
		return &http.Client{Transport: &http.Transport{}}
	}
	dialer := func(ctx context.Context, _, _ string) (net.Conn, error) {
		f, err := os.OpenFile(endpoint, os.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
		return &pipeConn{f}, nil
	}
	return &http.Client{Transport: &http.Transport{DialContext: dialer}}
}

// pipeConn adapts an opened named pipe to net.Conn.
type pipeConn struct{ f *os.File }

func (c *pipeConn) Read(b []byte) (int, error)  { return c.f.Read(b) }
func (c *pipeConn) Write(b []byte) (int, error) { return c.f.Write(b) }
func (c *pipeConn) Close() error                { return c.f.Close() }
func (c *pipeConn) LocalAddr() net.Addr         { return pipeAddr(c.f.Name()) }
func (c *pipeConn) RemoteAddr() net.Addr        { return pipeAddr(c.f.Name()) }

// SetDeadline is a no-op: a synchronous pipe handle has no async I/O to
// cancel, so the client-level timeout is what bounds a stuck request.
func (c *pipeConn) SetDeadline(time.Time) error      { return nil }
func (c *pipeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *pipeConn) SetWriteDeadline(time.Time) error { return nil }

type pipeAddr string

func (a pipeAddr) Network() string { return "pipe" }
func (a pipeAddr) String() string  { return string(a) }
