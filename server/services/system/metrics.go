package system

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"

	"github.com/CoreyCole/vamos/pkg/db"
)

// collectSystemSignals gathers all scalar system metrics.
func collectSystemSignals() SystemSignals {
	var s SystemSignals

	// System uptime (from host, not process startedAt)
	if uptime, err := host.Uptime(); err == nil {
		s.Uptime = formatDuration(time.Duration(uptime) * time.Second)
	}

	// Memory
	if vm, err := mem.VirtualMemory(); err == nil {
		s.MemTotal = humanize.Bytes(vm.Total)
		s.MemUsed = humanize.Bytes(vm.Used)
		s.MemUsedPercent = fmt.Sprintf("%.1f%%", vm.UsedPercent)
		s.SwapTotal = humanize.Bytes(vm.SwapTotal)
		s.SwapUsed = humanize.Bytes(vm.SwapTotal - vm.SwapFree)
		if vm.SwapTotal > 0 {
			swapUsedPct := float64(vm.SwapTotal-vm.SwapFree) / float64(vm.SwapTotal) * 100
			s.SwapUsedPercent = fmt.Sprintf("%.1f%%", swapUsedPct)
		} else {
			s.SwapUsedPercent = "N/A"
		}
	}

	// CPU
	if cpuPct, err := cpu.Percent(0, false); err == nil && len(cpuPct) > 0 {
		s.CpuPercent = fmt.Sprintf("%.1f%%", cpuPct[0])
	}

	// Disk
	if du, err := disk.Usage("/"); err == nil {
		s.DiskTotal = humanize.Bytes(du.Total)
		s.DiskUsed = humanize.Bytes(du.Used)
		s.DiskUsedPercent = fmt.Sprintf("%.1f%%", du.UsedPercent)
	}

	// Load average
	if loadAvg, err := load.Avg(); err == nil {
		s.LoadAvg = fmt.Sprintf(
			"%.2f / %.2f / %.2f",
			loadAvg.Load1,
			loadAvg.Load5,
			loadAvg.Load15,
		)
	}

	return s
}

// monitoredServices is the list of systemd user services to monitor.
var monitoredServices = []string{
	"cn-agents",
	"cn-agents-ts-worker",
	"cn-temporal",
}

// collectServices queries systemd for service statuses.
func collectServices() []ServiceInfo {
	services := make([]ServiceInfo, 0, len(monitoredServices))
	for _, name := range monitoredServices {
		info := ServiceInfo{Name: name, Active: "unknown", Sub: "unknown"}

		// Read properties from systemctl
		props := readServiceProperties(name)
		if active, ok := props["ActiveState"]; ok {
			info.Active = active
		}
		if sub, ok := props["SubState"]; ok {
			info.Sub = sub
		}
		if since, ok := props["StateChangeTimestamp"]; ok {
			info.Since = formatTimestamp(since)
		}
		if memStr, ok := props["MemoryCurrent"]; ok {
			if memBytes, err := strconv.ParseUint(
				memStr,
				10,
				64,
			); err == nil &&
				memBytes < 1<<62 {
				info.Memory = humanize.Bytes(memBytes)
			}
		}

		services = append(services, info)
	}
	return services
}

// readServiceProperties reads systemctl --user show properties.
func readServiceProperties(name string) map[string]string {
	props := make(map[string]string)

	// Use /proc/self to find systemctl path, fallback to common locations
	cmd := findExecutable("systemctl")
	if cmd == "" {
		return props
	}

	f, err := os.Open("/dev/null")
	if err != nil {
		return props
	}
	defer f.Close()

	// Build the command manually using os/exec
	r, w, err := os.Pipe()
	if err != nil {
		return props
	}

	proc, err := os.StartProcess(cmd, []string{
		"systemctl", "--user", "show", name,
		"--property=ActiveState,SubState,StateChangeTimestamp,MemoryCurrent",
	}, &os.ProcAttr{
		Files: []*os.File{f, w, f},
	})
	if err != nil {
		r.Close()
		w.Close()
		return props
	}
	w.Close()

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if k, v, ok := strings.Cut(line, "="); ok {
			props[k] = v
		}
	}
	r.Close()
	proc.Wait()

	return props
}

// findExecutable looks for an executable in common paths.
func findExecutable(name string) string {
	paths := []string{
		"/usr/bin/" + name,
		"/bin/" + name,
		"/usr/sbin/" + name,
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// collectTopProcesses returns the top N processes by memory usage.
func collectTopProcesses(n int) []ProcessInfo {
	cmd := findExecutable("ps")
	if cmd == "" {
		return nil
	}

	f, err := os.Open("/dev/null")
	if err != nil {
		return nil
	}
	defer f.Close()

	r, w, err := os.Pipe()
	if err != nil {
		return nil
	}

	proc, err := os.StartProcess(cmd, []string{
		"ps", "aux", "--sort=-rss",
	}, &os.ProcAttr{
		Files: []*os.File{f, w, f},
	})
	if err != nil {
		r.Close()
		w.Close()
		return nil
	}
	w.Close()

	var procs []ProcessInfo
	scanner := bufio.NewScanner(r)
	scanner.Scan() // skip header

	for scanner.Scan() && len(procs) < n {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 11 {
			continue
		}

		pid, _ := strconv.Atoi(fields[1])
		cpuPct, _ := strconv.ParseFloat(fields[2], 64)
		rssKB, _ := strconv.ParseFloat(fields[5], 64)
		command := strings.Join(fields[10:], " ")

		// Truncate long commands
		if len(command) > 60 {
			command = command[:57] + "..."
		}

		procs = append(procs, ProcessInfo{
			PID:     pid,
			User:    fields[0],
			MemMB:   rssKB / 1024,
			CPUPct:  cpuPct,
			Command: command,
		})
	}
	r.Close()
	proc.Wait()

	return procs
}

// collectCrashes scans /var/crash for crash report files.
func collectCrashes() []CrashReport {
	matches, err := filepath.Glob("/var/crash/*.crash")
	if err != nil {
		return nil
	}

	var crashes []CrashReport
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		// Extract program name from filename (e.g., _usr_bin_wezterm-gui.1000.crash)
		base := filepath.Base(path)
		program := base
		if strings.HasPrefix(base, "_") {
			parts := strings.Split(strings.TrimPrefix(base, "_"), ".")
			if len(parts) > 0 {
				program = strings.ReplaceAll(parts[0], "_", "/")
			}
		}

		crashes = append(crashes, CrashReport{
			Filename: base,
			Program:  program,
			SizeMB:   float64(info.Size()) / (1024 * 1024),
			Date:     info.ModTime(),
		})
	}
	return crashes
}

// formatDuration formats a duration into a human-readable string.
func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

// formatTimestamp parses systemd timestamps into a relative time string.
func formatTimestamp(ts string) string {
	// systemd timestamps look like: "Mon 2025-01-20 14:30:00 EST"
	// Try common formats
	formats := []string{
		"Mon 2006-01-02 15:04:05 MST",
		"Mon 2006-01-02 15:04:05 -0700",
		time.RFC3339,
	}
	for _, fmt := range formats {
		if t, err := time.Parse(fmt, ts); err == nil {
			return humanize.Time(t)
		}
	}
	if ts != "" {
		return ts
	}
	return "unknown"
}

// sqliteTimestampFormats are the formats SQLite may produce for CAST(datetime AS TEXT).
// Accept both driver-specific and generic SQLite datetime text formats.
var sqliteTimestampFormats = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05-07:00",
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
}

// parseSQLiteTimestamp parses a SQLite datetime string into a time.Time.
func parseSQLiteTimestamp(s string) (time.Time, bool) {
	for _, f := range sqliteTimestampFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// toBootSessions converts sqlc rows into display-ready BootSession values.
func toBootSessions(rows []db.GetDistinctBootIDsRow) []BootSession {
	sessions := make([]BootSession, 0, len(rows))
	for _, r := range rows {
		s := BootSession{
			BootID:        r.BootID,
			FirstSeen:     r.FirstSeen,
			LastSeen:      r.LastSeen,
			SnapshotCount: r.SnapshotCount,
		}

		first, okFirst := parseSQLiteTimestamp(r.FirstSeen)
		last, okLast := parseSQLiteTimestamp(r.LastSeen)

		if okFirst {
			s.FirstSeen = first.Format("Jan 02 15:04")
		}
		if okLast {
			s.LastSeen = last.Format("Jan 02 15:04")
		}
		if okFirst && okLast {
			s.Uptime = formatDuration(last.Sub(first))
		} else {
			s.Uptime = "--"
		}

		sessions = append(sessions, s)
	}
	return sessions
}
