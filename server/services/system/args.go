package system

import "time"

// SystemSignals holds scalar system metrics pushed via MarshalAndPatchSignals.
// JSON keys match the data-signals declaration in page_system.templ.
type SystemSignals struct {
	MemTotal        string `json:"memTotal,omitempty"`
	MemUsed         string `json:"memUsed,omitempty"`
	MemUsedPercent  string `json:"memUsedPercent,omitempty"`
	SwapTotal       string `json:"swapTotal,omitempty"`
	SwapUsed        string `json:"swapUsed,omitempty"`
	SwapUsedPercent string `json:"swapUsedPercent,omitempty"`
	CpuPercent      string `json:"cpuPercent,omitempty"`
	DiskTotal       string `json:"diskTotal,omitempty"`
	DiskUsed        string `json:"diskUsed,omitempty"`
	DiskUsedPercent string `json:"diskUsedPercent,omitempty"`
	Uptime          string `json:"uptime,omitempty"`
	LoadAvg         string `json:"loadAvg,omitempty"`
}

// DashboardArgs holds the args for the full page render.
type DashboardArgs struct {
	UserEmail    string
	CurrentTheme string
}

// ServiceInfo holds systemd service status.
type ServiceInfo struct {
	Name   string
	Active string // "active", "inactive", "failed"
	Sub    string // "running", "dead", "failed"
	Since  string
	Memory string
}

// ProcessInfo holds a top process entry.
type ProcessInfo struct {
	PID     int
	User    string
	MemMB   float64
	CPUPct  float64
	Command string
}

// CrashReport holds info about a crash dump file.
type CrashReport struct {
	Filename string
	Program  string
	SizeMB   float64
	Date     time.Time
}

// BootSession is a display-ready view of a boot session for the history UI.
type BootSession struct {
	BootID        string
	FirstSeen     string // formatted timestamp
	LastSeen      string // formatted timestamp
	Uptime        string // human-friendly duration
	SnapshotCount int64
}

// HealthJSON is the response for the /system/health endpoint.
type HealthJSON struct {
	Status   string        `json:"status"`
	Uptime   string        `json:"uptime"`
	Memory   HealthMemory  `json:"memory"`
	Swap     HealthSwap    `json:"swap"`
	CPU      float64       `json:"cpu_percent"`
	Disk     HealthDisk    `json:"disk"`
	LoadAvg  [3]float64    `json:"load_avg"`
	Services []ServiceInfo `json:"services"`
}

// HealthMemory is the memory section of the health JSON.
type HealthMemory struct {
	TotalBytes   uint64  `json:"total_bytes"`
	UsedBytes    uint64  `json:"used_bytes"`
	UsedPercent  float64 `json:"used_percent"`
}

// HealthSwap is the swap section of the health JSON.
type HealthSwap struct {
	TotalBytes  uint64  `json:"total_bytes"`
	UsedBytes   uint64  `json:"used_bytes"`
	UsedPercent float64 `json:"used_percent"`
}

// HealthDisk is the disk section of the health JSON.
type HealthDisk struct {
	TotalBytes  uint64  `json:"total_bytes"`
	UsedBytes   uint64  `json:"used_bytes"`
	UsedPercent float64 `json:"used_percent"`
}
