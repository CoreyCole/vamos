package system

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"

	"github.com/CoreyCole/vamos/pkg/db"
)

const (
	snapshotInterval = 2 * time.Minute
	maxBootsToKeep   = 5
	maxSnapshotRows  = 50000
)

// readBootID reads the kernel boot ID from /proc/sys/kernel/random/boot_id.
// Falls back to a timestamp-based ID if unreadable (e.g., non-Linux).
func readBootID() string {
	data, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "unknown-" + time.Now().Format("20060102-150405")
	}
	return strings.TrimSpace(string(data))
}

// runSnapshotLoop captures a system snapshot every snapshotInterval.
func (s *Service) runSnapshotLoop() {
	s.captureSnapshot()

	ticker := time.NewTicker(snapshotInterval)
	defer ticker.Stop()

	for range ticker.C {
		s.captureSnapshot()
	}
}

// captureSnapshot collects system metrics and top processes, writing them to SQLite.
func (s *Service) captureSnapshot() {
	ctx := context.Background()

	var cpuPct float64
	if pcts, err := cpu.Percent(0, false); err == nil && len(pcts) > 0 {
		cpuPct = pcts[0]
	}

	var memUsed, memTotal, swapUsed, swapTotal int64
	var memPct float64
	if vm, err := mem.VirtualMemory(); err == nil {
		memUsed = int64(vm.Used)
		memTotal = int64(vm.Total)
		memPct = vm.UsedPercent
		swapTotal = int64(vm.SwapTotal)
		swapUsed = int64(vm.SwapTotal - vm.SwapFree)
	}

	var diskUsed, diskTotal int64
	if du, err := disk.Usage("/"); err == nil {
		diskUsed = int64(du.Used)
		diskTotal = int64(du.Total)
	}

	var l1, l5, l15 float64
	if loadAvg, err := load.Avg(); err == nil {
		l1 = loadAvg.Load1
		l5 = loadAvg.Load5
		l15 = loadAvg.Load15
	}

	snapshot, err := s.queries.InsertSystemSnapshot(ctx, db.InsertSystemSnapshotParams{
		BootID:         s.bootID,
		CapturedAt:     time.Now(),
		CpuPercent:     cpuPct,
		MemUsedBytes:   memUsed,
		MemTotalBytes:  memTotal,
		MemUsedPercent: memPct,
		SwapUsedBytes:  swapUsed,
		SwapTotalBytes: swapTotal,
		DiskUsedBytes:  diskUsed,
		DiskTotalBytes: diskTotal,
		LoadAvg1:       l1,
		LoadAvg5:       l5,
		LoadAvg15:      l15,
	})
	if err != nil {
		log.Printf("[system/snapshots] failed to insert snapshot: %v", err)
		return
	}

	procs := collectTopProcesses(10)
	for _, p := range procs {
		if err := s.queries.InsertSnapshotProcess(ctx, db.InsertSnapshotProcessParams{
			SnapshotID: snapshot.ID,
			Pid:        int64(p.PID),
			User:       p.User,
			MemMb:      p.MemMB,
			CpuPercent: p.CPUPct,
			Command:    p.Command,
		}); err != nil {
			log.Printf("[system/snapshots] failed to insert process for snapshot %d: %v", snapshot.ID, err)
		}
	}
}

// cleanupOnStartup removes old boot data, keeping the most recent maxBootsToKeep boots.
// Also enforces a hard row cap as a disk safety net.
func (s *Service) cleanupOnStartup() {
	ctx := context.Background()

	boots, err := s.queries.GetDistinctBootIDs(ctx)
	if err != nil {
		log.Printf("[system/snapshots] cleanup: failed to get boot IDs: %v", err)
		return
	}

	if len(boots) > maxBootsToKeep {
		keepIDs := make([]string, maxBootsToKeep)
		for i := 0; i < maxBootsToKeep; i++ {
			keepIDs[i] = boots[i].BootID
		}

		// Delete processes first (no cascade), then snapshots
		if err := s.queries.DeleteSnapshotProcessesByBootIDs(ctx, keepIDs); err != nil {
			log.Printf("[system/snapshots] cleanup: failed to delete old processes: %v", err)
			return
		}
		if err := s.queries.DeleteSnapshotsByExcludedBootIDs(ctx, keepIDs); err != nil {
			log.Printf("[system/snapshots] cleanup: failed to delete old snapshots: %v", err)
			return
		}

		deleted := len(boots) - maxBootsToKeep
		log.Printf("[system/snapshots] cleanup: removed data from %d old boot(s)", deleted)
	}

	// Hard cap safety net
	count, err := s.queries.GetSystemSnapshotCount(ctx)
	if err != nil {
		return
	}
	if count > maxSnapshotRows {
		excess := count - maxSnapshotRows
		if err := s.queries.DeleteOldestSnapshotProcesses(ctx, excess); err != nil {
			log.Printf("[system/snapshots] cleanup: failed to trim processes: %v", err)
			return
		}
		if err := s.queries.DeleteOldestSnapshots(ctx, excess); err != nil {
			log.Printf("[system/snapshots] cleanup: failed to trim snapshots: %v", err)
			return
		}
		log.Printf("[system/snapshots] cleanup: trimmed %d snapshots (hard cap)", excess)
	}
}
