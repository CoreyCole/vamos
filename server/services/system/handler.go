package system

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/theme"
)

// HandleDashboard renders the system health dashboard page.
func (s *Service) HandleDashboard(c echo.Context, themeService *theme.Service) error {
	userEmail, _ := c.Get("user_email").(string)

	currentTheme := "dark"
	if themeService != nil {
		currentTheme = themeService.GetCurrentThemeMode(c)
	}

	return Dashboard(DashboardArgs{
		UserEmail:    userEmail,
		CurrentTheme: currentTheme,
	}).Render(c.Request().Context(), c.Response().Writer)
}

// HandleStream is the long-lived SSE endpoint for live dashboard updates.
func (s *Service) HandleStream(c echo.Context) error {
	sse := datastar.NewSSE(c.Response().Writer, c.Request())

	systemTicker := time.NewTicker(2 * time.Second)
	servicesTicker := time.NewTicker(5 * time.Second)
	defer systemTicker.Stop()
	defer servicesTicker.Stop()

	// Fire all immediately on connection
	s.sendSystemMetrics(sse)
	s.sendServices(sse)
	s.sendProcesses(sse)
	s.sendCrashes(sse)
	s.sendBootList(sse)

	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case <-systemTicker.C:
			s.sendSystemMetrics(sse)
		case <-servicesTicker.C:
			s.sendServices(sse)
			s.sendProcesses(sse)
			s.sendCrashes(sse)
		}
	}
}

// HandleHealthJSON returns system health as JSON for programmatic access.
func (s *Service) HandleHealthJSON(c echo.Context) error {
	health := HealthJSON{
		Status: "ok",
		Uptime: formatDuration(time.Since(s.startedAt)),
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		health.Memory = HealthMemory{
			TotalBytes:  vm.Total,
			UsedBytes:   vm.Used,
			UsedPercent: vm.UsedPercent,
		}
		health.Swap = HealthSwap{
			TotalBytes: vm.SwapTotal,
			UsedBytes:  vm.SwapTotal - vm.SwapFree,
		}
		if vm.SwapTotal > 0 {
			health.Swap.UsedPercent = float64(vm.SwapTotal-vm.SwapFree) / float64(vm.SwapTotal) * 100
		}
	}

	if cpuPct, err := cpu.Percent(0, false); err == nil && len(cpuPct) > 0 {
		health.CPU = cpuPct[0]
	}

	if du, err := disk.Usage("/"); err == nil {
		health.Disk = HealthDisk{
			TotalBytes:  du.Total,
			UsedBytes:   du.Used,
			UsedPercent: du.UsedPercent,
		}
	}

	if loadAvg, err := load.Avg(); err == nil {
		health.LoadAvg = [3]float64{loadAvg.Load1, loadAvg.Load5, loadAvg.Load15}
	}

	health.Services = collectServices()

	return c.JSON(http.StatusOK, health)
}

// sendSystemMetrics pushes scalar system metrics via MarshalAndPatchSignals.
func (s *Service) sendSystemMetrics(sse *datastar.ServerSentEventGenerator) {
	signals := collectSystemSignals()
	sse.MarshalAndPatchSignals(signals)
}

// sendServices pushes the services table fragment via PatchElementTempl.
func (s *Service) sendServices(sse *datastar.ServerSentEventGenerator) {
	services := collectServices()
	sse.PatchElementTempl(
		ServicesTable(services),
		datastar.WithSelectorID("services-table"),
		datastar.WithModeInner(),
	)
}

// sendProcesses pushes the process table fragment via PatchElementTempl.
func (s *Service) sendProcesses(sse *datastar.ServerSentEventGenerator) {
	procs := collectTopProcesses(10)
	sse.PatchElementTempl(
		ProcessesTable(procs),
		datastar.WithSelectorID("processes-table"),
		datastar.WithModeInner(),
	)
}

// sendCrashes pushes the crash reports fragment via PatchElementTempl.
func (s *Service) sendCrashes(sse *datastar.ServerSentEventGenerator) {
	crashes := collectCrashes()
	sse.PatchElementTempl(
		CrashesCard(crashes),
		datastar.WithSelectorID("crashes-section"),
		datastar.WithModeInner(),
	)
}

// sendBootList pushes the metrics history boot list via PatchElementTempl.
func (s *Service) sendBootList(sse *datastar.ServerSentEventGenerator) {
	boots, err := s.queries.GetDistinctBootIDs(context.Background())
	if err != nil {
		return
	}
	sse.PatchElementTempl(
		HistoryBootList(toBootSessions(boots), s.bootID),
		datastar.WithSelectorID("history-content"),
		datastar.WithModeInner(),
	)
}

// HandleHistory is a POST handler that returns snapshots for a selected boot session.
func (s *Service) HandleHistory(c echo.Context) error {
	bootID := c.FormValue("bootId")
	sse := datastar.NewSSE(c.Response().Writer, c.Request())

	if bootID == "" {
		// Return boot list
		boots, _ := s.queries.GetDistinctBootIDs(c.Request().Context())
		return sse.PatchElementTempl(
			HistoryBootList(toBootSessions(boots), s.bootID),
			datastar.WithSelectorID("history-content"),
			datastar.WithModeInner(),
		)
	}

	// Return snapshots for the selected boot (collapsed, no expansion)
	snapshots, _ := s.queries.GetSnapshotsByBootID(c.Request().Context(), bootID)
	return sse.PatchElementTempl(
		HistorySnapshots(bootID, s.bootID, snapshots, 0, nil),
		datastar.WithSelectorID("history-content"),
		datastar.WithModeInner(),
	)
}

// HandleSnapshotDetail is a POST handler that re-renders the snapshot table with inline process expansion.
func (s *Service) HandleSnapshotDetail(c echo.Context) error {
	bootID := c.FormValue("bootId")
	if bootID == "" {
		return c.String(http.StatusBadRequest, "missing boot ID")
	}

	snapshotIDStr := c.FormValue("snapshotId")
	snapshotID, err := strconv.ParseInt(snapshotIDStr, 10, 64)
	if err != nil {
		return c.String(http.StatusBadRequest, "invalid snapshot ID")
	}

	snapshots, _ := s.queries.GetSnapshotsByBootID(c.Request().Context(), bootID)

	var processes []db.SystemSnapshotProcess
	if snapshotID > 0 {
		processes, _ = s.queries.GetSnapshotProcesses(c.Request().Context(), snapshotID)
	}

	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return sse.PatchElementTempl(
		HistorySnapshots(bootID, s.bootID, snapshots, snapshotID, processes),
		datastar.WithSelectorID("history-content"),
		datastar.WithModeInner(),
	)
}
