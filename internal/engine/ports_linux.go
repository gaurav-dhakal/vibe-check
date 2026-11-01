//go:build linux

package engine

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// collectListeners reads /proc/net/tcp{,6} and resolves owners with one
// fd-walk per *changed* listener set. A socket inode is unique for its
// lifetime, so an unchanged set has unchanged owners and re-walking every
// /proc/<pid>/fd each tick would be pure waste.
func (e *Engine) collectListeners() ([]listenSock, error) {
	listens, err := scanListeningSockets()
	if err != nil || len(listens) == 0 {
		return listens, err
	}
	inodes := make(map[uint64]bool, len(listens))
	for _, l := range listens {
		inodes[l.inode] = true
	}
	if sig := inodeSig(inodes); sig != e.portSig || e.portOwners == nil {
		e.portSig = sig
		e.portOwners = socketOwnerMap(inodes)
	}
	for i := range listens {
		listens[i].pid = e.portOwners[listens[i].inode]
	}
	return listens, nil
}

const stateListen = "0A"

var stateNames = map[string]string{
	"01": "ESTABLISHED", "02": "SYN_SENT", "03": "SYN_RECV",
	"04": "FIN_WAIT1", "05": "FIN_WAIT2", "06": "TIME_WAIT",
	"07": "CLOSE", "08": "CLOSE_WAIT", "09": "LAST_ACK",
	"0A": "LISTEN", "0B": "CLOSING",
}

func scanListeningSockets() ([]listenSock, error) {
	var out []listenSock
	var errs []error
	for _, spec := range []struct{ path, proto string }{
		{"/proc/net/tcp", "tcp"},
		{"/proc/net/tcp6", "tcp6"},
	} {
		f, err := os.Open(spec.path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out, err = appendListenLines(out, f, spec.proto)
		f.Close()
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(out) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return out, nil
}

func appendListenLines(dst []listenSock, f *os.File, proto string) ([]listenSock, error) {
	data, err := io.ReadAll(f)
	if err != nil {
		return dst, err
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1:] { // skip header
		f := strings.Fields(line)
		if len(f) < 10 || f[3] != stateListen {
			continue
		}
		ip, port, ok := decodeHexAddr(f[1], proto == "tcp6")
		if !ok {
			continue
		}
		inode, _ := strconv.ParseUint(f[9], 10, 64)
		dst = append(dst, listenSock{proto: proto, ip: ip, port: port, inode: inode})
	}
	return dst, nil
}

// decodeHexAddr parses kernel-style little-endian hex addresses:
// "0100007F:1F90" → 127.0.0.1:8080.
func decodeHexAddr(s string, v6 bool) (net.IP, uint16, bool) {
	ipHex, portHex, ok := strings.Cut(s, ":")
	if !ok {
		return nil, 0, false
	}
	portU, err := strconv.ParseUint(portHex, 16, 32)
	if err != nil {
		return nil, 0, false
	}
	if !v6 {
		raw, err := hexLE(ipHex, 4)
		if err != nil {
			return nil, 0, false
		}
		return net.IPv4(raw[0], raw[1], raw[2], raw[3]), uint16(portU), true
	}
	bs, err := hex.DecodeString(strings.ToUpper(ipHex))
	if err != nil || len(bs) != 16 {
		return nil, 0, false
	}
	ip := make(net.IP, 16)
	// v6 is four 32-bit words, each little-endian in the hex dump
	for w := 0; w < 4; w++ {
		ip[w*4+0] = bs[w*4+3]
		ip[w*4+1] = bs[w*4+2]
		ip[w*4+2] = bs[w*4+1]
		ip[w*4+3] = bs[w*4+0]
	}
	return ip, uint16(portU), true
}

func hexLE(s string, bytes int) ([]byte, error) {
	if len(s) != bytes*2 {
		return nil, fmt.Errorf("bad hex len")
	}
	bs, err := hex.DecodeString(strings.ToUpper(s))
	if err != nil {
		return nil, err
	}
	out := make([]byte, bytes)
	for i := range bs {
		out[i] = bs[len(bs)-1-i]
	}
	return out, nil
}

// socketOwnerMap maps LISTEN socket inodes → owning pid by scanning
// /proc/[pid]/fd readlinks. Only fds pointing at our inodes are resolved.
func socketOwnerMap(want map[uint64]bool) map[uint64]int32 {
	out := make(map[uint64]int32, len(want))
	if len(want) == 0 {
		return out
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		if _, err := strconv.ParseInt(ent.Name(), 10, 32); err != nil {
			continue
		}
		fddir := filepath.Join("/proc", ent.Name(), "fd")
		fds, err := os.ReadDir(fddir)
		if err != nil {
			continue
		}
		pid, _ := strconv.ParseInt(ent.Name(), 10, 32)
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fddir, fd.Name()))
			if err != nil || !strings.HasPrefix(link, "socket:[") {
				continue
			}
			inodeStr := strings.TrimSuffix(link[len("socket:["):], "]")
			inode, err := strconv.ParseUint(inodeStr, 10, 64)
			if err != nil {
				continue
			}
			if want[inode] {
				out[inode] = int32(pid)
				if len(out) == len(want) {
					return out
				}
			}
		}
	}
	return out
}

// ---- synthetic HTTP health prober ----

func inodeSig(inodes map[uint64]bool) uint64 {
	var h uint64
	for in := range inodes {
		h ^= in*0x9e3779b97f4a7c15 + 0x1
	}
	return h
}
