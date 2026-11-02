package engine

import (
	"fmt"
	"time"
)

type Snapshot struct {
	At         time.Time   `json:"at"`
	Sys        System      `json:"sys"`
	Procs      []Proc      `json:"procs"`
	Ports      []Port      `json:"ports"`
	Containers []Container `json:"containers"`
	GPUs       []GPU       `json:"gpus,omitempty"`
	Artifacts  []Artifact  `json:"artifacts"`
	DockerErr  string      `json:"docker_err,omitempty"`
	ScanState  ScanState   `json:"scan_state"`
}

type System struct {
	Hostname string        `json:"hostname"`
	OS       string        `json:"os"`
	Kernel   string        `json:"kernel"`
	Uptime   time.Duration `json:"uptime_ns"`
	NumCPU   int           `json:"num_cpu"`
	CPUModel string        `json:"cpu_model"`

	CPUPercent   float64   `json:"cpu_percent"`
	CorePercents []float64 `json:"core_percents"`
	Load1        float64   `json:"load_1"`
	Load5        float64   `json:"load_5"`
	Load15       float64   `json:"load_15"`

	MemUsed    uint64  `json:"mem_used"`
	MemTotal   uint64  `json:"mem_total"`
	MemAvail   uint64  `json:"mem_avail"`
	MemPercent float64 `json:"mem_percent"`
	SwapUsed   uint64  `json:"swap_used"`
	SwapTotal  uint64  `json:"swap_total"`

	DiskReadBps  uint64 `json:"disk_read_bps"`
	DiskWriteBps uint64 `json:"disk_write_bps"`
	NetRecvBps   uint64 `json:"net_recv_bps"`
	NetSentBps   uint64 `json:"net_sent_bps"`

	ProcTotal int    `json:"proc_total"`
	FDAlloc   uint64 `json:"fd_alloc"`
	FDMax     uint64 `json:"fd_max"`
	FDPerProc int    `json:"fd_per_proc"` // avg open fds across readable procs
	FDCount   int    `json:"fd_count"`    // fds readable without root

	OrphanCount int    `json:"orphan_count"`
	OrphanRSS   uint64 `json:"orphan_rss"`
}

type Proc struct {
	Pid        int32   `json:"pid"`
	PPid       int32   `json:"ppid"`
	UID        uint32  `json:"uid"`
	User       string  `json:"user"`
	Name       string  `json:"name"`
	CmdLine    string  `json:"cmdline"`
	State      string  `json:"state"`
	RSS        uint64  `json:"rss"`
	CPUPercent float64 `json:"cpu_percent"`
	AgeSec     float64 `json:"age_sec"`
	Threads    int     `json:"threads"`
	Orphan     bool    `json:"orphan"`
}

type Port struct {
	Proto     string     `json:"proto"` // tcp | tcp6
	Addr      string     `json:"addr"`  // bind address
	Port      uint16     `json:"port"`
	State     string     `json:"state"`
	Pid       int32      `json:"pid"`
	ProcName  string     `json:"proc_name"`
	Monitored bool       `json:"monitored"`
	HTTP      *HTTPProbe `json:"http,omitempty"`
}

type HTTPProbe struct {
	OK        bool      `json:"ok"`
	Status    int       `json:"status"`
	LatencyMs float64   `json:"latency_ms"`
	Scheme    string    `json:"scheme"`
	CheckedAt time.Time `json:"checked_at"`
}

type Container struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Image      string  `json:"image"`
	State      string  `json:"state"`
	Status     string  `json:"status"`
	CPUPercent float64 `json:"cpu_percent"`
	MemUsage   uint64  `json:"mem_usage"`
	MemLimit   uint64  `json:"mem_limit"`
	NetRx      uint64  `json:"net_rx"`
	NetTx      uint64  `json:"net_tx"`
	Pids       int     `json:"pids"`
}

type Artifact struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Size    uint64 `json:"size"`
	Files   int    `json:"files"`
	Project string `json:"project"`
}

type GPU struct {
	Vendor      string  `json:"vendor"` // nvidia | amdgpu | i915
	Name        string  `json:"name"`
	UtilPercent float64 `json:"util_percent"`
	MemUsed     uint64  `json:"mem_used"`
	MemTotal    uint64  `json:"mem_total"`
	TempC       float64 `json:"temp_c"`
	HasTemp     bool    `json:"has_temp"`
}

type ScanState struct {
	Scanning   bool      `json:"scanning"`
	DirsSeen   int       `json:"dirs_seen"`
	LastDoneAt time.Time `json:"last_done_at"`
	Error      string    `json:"error,omitempty"`
}

type Event struct {
	Level string // ok | warn | error
	Text  string
}

// Human formatting helpers shared by engine + UI.

func Bytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func Bps(n uint64) string { return Bytes(n) + "/s" }

func Seconds(s float64) string {
	d := time.Duration(s) * time.Second
	return Dur(d)
}

func Dur(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%dm", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%dh%02dm", h, int(d.Minutes())%60)
	default:
		dd := int(d.Hours()) / 24
		hh := int(d.Hours()) % 24
		return fmt.Sprintf("%dd%dh", dd, hh)
	}
}
