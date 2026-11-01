package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
)

// dockerRuntime talks to the container socket directly (no subprocess):
// a plain HTTP client whose transport dials a Unix domain socket.
type dockerRuntime struct {
	sock   string
	client *http.Client

	mu       sync.Mutex
	stats    map[string]*Container // live rows keyed by id
	streams  map[string]context.CancelFunc
	eventsCB context.CancelFunc
	probed   bool
}

func newDockerRuntime() *dockerRuntime {
	return &dockerRuntime{
		stats:   map[string]*Container{},
		streams: map[string]context.CancelFunc{},
	}
}

func (e *Engine) updateDocker() {
	d := e.docker
	if !d.probed {
		d.sock, _ = findSocket()
		d.probed = true
	}
	if d.sock == "" {
		msg := runtimeLabel() + ": no container runtime endpoint found"
		e.dockerErr.Store(&msg)
		return
	}
	if d.client == nil {
		d.client = newSocketClient(d.sock)
	}

	list, err := d.list(context.Background())
	if err != nil {
		msg := err.Error()
		e.dockerErr.Store(&msg)
		return
	}
	msg := ""
	e.dockerErr.Store(&msg)

	running := map[string]bool{}
	d.mu.Lock()
	next := map[string]*Container{}
	for _, c := range list {
		row := *c
		if prev, ok := d.stats[c.ID]; ok {
			row.CPUPercent, row.MemUsage, row.MemLimit = prev.CPUPercent, prev.MemUsage, prev.MemLimit
			row.NetRx, row.NetTx, row.Pids = prev.NetRx, prev.NetTx, prev.Pids
		}
		next[c.ID] = &row
		if row.State == "running" {
			running[c.ID] = true
		}
	}
	d.stats = next
	d.mu.Unlock()

	// reconcile stats streams
	d.mu.Lock()
	for id, cancel := range d.streams {
		if !running[id] {
			cancel()
			delete(d.streams, id)
		}
	}
	d.mu.Unlock()
	for id := range running {
		d.mu.Lock()
		_, exists := d.streams[id]
		d.mu.Unlock()
		if !exists {
			ctx, cancel := context.WithCancel(e.ctx)
			d.mu.Lock()
			d.streams[id] = cancel
			d.mu.Unlock()
			go d.statsStream(ctx, id)
		}
	}

	d.mu.Lock()
	out := make([]Container, 0, len(d.stats))
	for _, c := range d.stats {
		out = append(out, *c)
	}
	d.mu.Unlock()
	sortContainers(out)
	e.containers.Store(&out)
	e.startEventWatch()
}

func sortContainers(c []Container) {
	order := map[string]int{"running": 0, "restarting": 1, "paused": 2, "created": 3, "exited": 4, "dead": 5, "removing": 6}
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && order[c[j].State] < order[c[j-1].State]; j-- {
			c[j], c[j-1] = c[j-1], c[j]
		}
	}
}

type containerJSON struct {
	ID     string   `json:"Id"`
	Names  []string `json:"Names"`
	Image  string   `json:"Image"`
	State  string   `json:"State"`
	Status string   `json:"Status"`
}

func (d *dockerRuntime) list(ctx context.Context) ([]*Container, error) {
	var rows []containerJSON
	if err := d.get(ctx, "/containers/json?all=1&limit=-1", &rows); err != nil {
		return nil, err
	}
	out := make([]*Container, 0, len(rows))
	for _, r := range rows {
		name := r.ID
		if len(r.Names) > 0 {
			name = strings.TrimPrefix(r.Names[0], "/")
		}
		out = append(out, &Container{
			ID: r.ID, Name: name, Image: r.Image, State: r.State, Status: r.Status,
		})
	}
	return out, nil
}

func (d *dockerRuntime) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("socket unreachable: %v", cleanDialErr(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("docker api %d on %s", resp.StatusCode, path)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// statsStream opens GET /containers/{id}/stats with the default
// streaming=true and parses each newline-delimited JSON sample.
func (d *dockerRuntime) statsStream(ctx context.Context, id string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/"+id+"/stats?stream=true", nil)
	if err != nil {
		return
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return
	}
	scan := bufio.NewScanner(resp.Body)
	scan.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var prev *statsMsg
	for scan.Scan() {
		var s statsMsg
		if err := json.Unmarshal(scan.Bytes(), &s); err != nil || s.CPUStats.SystemCPUs == 0 && s.SystemCPUUsage == 0 {
			continue
		}
		cpu := 0.0
		if prev != nil {
			dUser := s.CPUStats.CPUUsage.TotalUsage - prev.CPUStats.CPUUsage.TotalUsage
			dSys := s.SystemCPUUsage - prev.SystemCPUUsage
			if dSys > 0 && dUser < (1<<62) && dSys < (1<<62) {
				online := float64(s.CPUStats.OnlineCPUs)
				if online == 0 {
					online = float64(runtime.NumCPU())
				}
				cpu = float64(dUser) / float64(dSys) * online * 100
			}
		}
		p := prev
		prev = &s
		_ = p

		d.mu.Lock()
		if c, ok := d.stats[id]; ok {
			c.CPUPercent = cpu
			c.MemUsage = s.MemoryStats.Usage
			c.MemLimit = s.MemoryStats.Limit
			c.Pids = s.PidsStats.Current
			var rx, tx uint64
			for _, n := range s.Networks {
				rx += n.RxBytes
				tx += n.TxBytes
			}
			c.NetRx, c.NetTx = rx, tx
		}
		d.mu.Unlock()
	}
	if scan.Err() != nil {
		return
	}
}

type statsMsg struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		OnlineCPUs int    `json:"online_cpus"`
		SystemCPUs uint64 `json:"system_cpus"`
	} `json:"cpu_stats"`
	SystemCPUUsage uint64 `json:"system_cpu_usage"`
	MemoryStats    struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
	Networks  map[string]netSample `json:"networks"`
	PidsStats struct {
		Current int `json:"current"`
	} `json:"pids_stats"`
}

type netSample struct {
	RxBytes uint64 `json:"rx_bytes"`
	TxBytes uint64 `json:"tx_bytes"`
}

// startEventWatch opens /events once; lifecycle events force a list refresh
// so container deaths show up without waiting for the poll interval.
func (e *Engine) startEventWatch() {
	d := e.docker
	if d.client == nil {
		return
	}
	d.mu.Lock()
	armed := d.eventsCB != nil
	d.mu.Unlock()
	if armed {
		return
	}
	ctx, cancel := context.WithCancel(e.ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://docker/events?filters=%7B%22event%22%3A%5B%22die%22%2C%22start%22%2C%22kill%22%2C%22pause%22%2C%22unpause%22%5D%7D", nil)
	if err != nil {
		cancel()
		return
	}
	d.mu.Lock()
	d.eventsCB = cancel
	d.mu.Unlock()
	go func() {
		defer cancel()
		resp, err := d.client.Do(req)
		if err != nil {
			d.mu.Lock()
			d.eventsCB = nil
			d.mu.Unlock()
			return
		}
		defer resp.Body.Close()
		scan := bufio.NewScanner(resp.Body)
		for scan.Scan() {
			var ev struct {
				Status string `json:"status"`
				From   string `json:"From"`
			}
			if json.Unmarshal(scan.Bytes(), &ev) == nil && ev.Status != "" {
				e.emit(Event{Level: "info", Text: fmt.Sprintf("docker: %s %s", ev.Status, ev.From)})
				e.updateDocker()
			}
		}
		d.mu.Lock()
		d.eventsCB = nil
		d.mu.Unlock()
	}()
}

func (d *dockerRuntime) ctl(id, action string) error {
	if d.client == nil {
		return errors.New("runtime socket unavailable")
	}
	path := "/containers/" + id + "/" + action
	if action == "stop" {
		path += "?t=3"
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://docker"+path, nil)
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return cleanDialErr(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode == 204 || resp.StatusCode == 201 || resp.StatusCode == 200 {
		return nil
	}
	if resp.StatusCode == 304 {
		return nil // already in state
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	return fmt.Errorf("api %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func cleanDialErr(err error) error {
	s := err.Error()
	if i := strings.Index(s, "dial unix "); i >= 0 {
		return errors.New(strings.TrimSpace(s[i:]))
	}
	return err
}
