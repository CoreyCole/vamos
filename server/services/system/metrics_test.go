package system

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"
)

const inactiveStatus = "inactive"

type errorReader struct {
	data string
	err  error
}

func (r *errorReader) Read(p []byte) (int, error) {
	if r.data != "" {
		n := copy(p, r.data)
		r.data = r.data[n:]
		return n, nil
	}
	return 0, r.err
}

func (r *errorReader) Close() error { return nil }

func stubCommands(t *testing.T, start func(string, []string) (commandOutput, error)) {
	t.Helper()
	oldFind, oldStart := findExecutable, startSystemCommand
	findExecutable = func(string) string { return "command" }
	startSystemCommand = start
	t.Cleanup(func() { findExecutable, startSystemCommand = oldFind, oldStart })
}

func TestReadServicePropertiesRetainsDataOnExitError(t *testing.T) {
	exitErr := errors.New("exit status 1")
	stubCommands(t, func(string, []string) (commandOutput, error) {
		return commandOutput{
			reader: io.NopCloser(
				strings.NewReader("ActiveState=" + inactiveStatus + "\nSubState=dead\n"),
			),
			wait: func() error { return exitErr },
		}, nil
	})

	props, err := readServiceProperties("unit")
	if !errors.Is(err, exitErr) || props["ActiveState"] != inactiveStatus ||
		props["SubState"] != "dead" {
		t.Fatalf("props=%v err=%v", props, err)
	}
}

func TestReadServicePropertiesRetainsDataOnScannerError(t *testing.T) {
	scanErr := errors.New("read failed")
	stubCommands(t, func(string, []string) (commandOutput, error) {
		return commandOutput{
			reader: &errorReader{data: "ActiveState=active\n", err: scanErr},
			wait:   func() error { return nil },
		}, nil
	})

	props, err := readServiceProperties("unit")
	if !errors.Is(err, scanErr) || props["ActiveState"] != "active" {
		t.Fatalf("props=%v err=%v", props, err)
	}
}

func TestCollectServicesRetainsRowsAfterIndependentFailure(t *testing.T) {
	oldServices := monitoredServices
	monitoredServices = []string{"good", "bad"}
	t.Cleanup(func() { monitoredServices = oldServices })
	failure := errors.New("start failed")
	stubCommands(t, func(_ string, args []string) (commandOutput, error) {
		if args[3] == "bad" {
			return commandOutput{}, failure
		}
		return commandOutput{
			reader: io.NopCloser(
				strings.NewReader("ActiveState=" + inactiveStatus + "\n"),
			),
			wait: func() error { return nil },
		}, nil
	})

	services, err := collectServices()
	if !errors.Is(err, failure) || len(services) != 2 ||
		services[0].Active != inactiveStatus ||
		services[1].Active != "unknown" {
		t.Fatalf("services=%+v err=%v", services, err)
	}
}

func TestCollectTopProcessesRetainsRowsOnExitError(t *testing.T) {
	exitErr := errors.New("exit status 1")
	stubCommands(t, func(string, []string) (commandOutput, error) {
		return commandOutput{
			reader: io.NopCloser(
				strings.NewReader(
					"USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND\nroot 42 1.5 0 0 2048 ? S 00:00 0:00 worker\n",
				),
			),
			wait: func() error { return exitErr },
		}, nil
	})

	procs, err := collectTopProcesses(10)
	if !errors.Is(err, exitErr) || len(procs) != 1 || procs[0].PID != 42 {
		t.Fatalf("procs=%+v err=%v", procs, err)
	}
}

func TestCollectorReportsMissingExecutable(t *testing.T) {
	oldFind := findExecutable
	findExecutable = func(string) string { return "" }
	t.Cleanup(func() { findExecutable = oldFind })

	if _, err := readServiceProperties("unit"); err == nil {
		t.Fatal("expected missing systemctl error")
	}
	if _, err := collectTopProcesses(10); err == nil {
		t.Fatal("expected missing ps error")
	}
}

func TestHandleHealthJSONDegradesWithServiceRows(t *testing.T) {
	oldServices := monitoredServices
	monitoredServices = []string{"unit"}
	t.Cleanup(func() { monitoredServices = oldServices })
	stubCommands(t, func(string, []string) (commandOutput, error) {
		return commandOutput{
			reader: io.NopCloser(
				strings.NewReader("ActiveState=" + inactiveStatus + "\n"),
			),
			wait: func() error {
				return errors.New("exit status 1")
			},
		}, nil
	})

	e := echo.New()
	rec := httptest.NewRecorder()
	ctx := e.NewContext(
		httptest.NewRequest(http.MethodGet, "/system/health", http.NoBody),
		rec,
	)
	service := &Service{startedAt: time.Now()}
	if err := service.HandleHealthJSON(ctx); err != nil {
		t.Fatal(err)
	}

	var health HealthJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if health.Status != "degraded" || len(health.Services) != 1 ||
		health.Services[0].Active != "inactive" {
		t.Fatalf("health=%+v", health)
	}
}

func TestDashboardPatchesPartialDataAfterCollectorFailure(t *testing.T) {
	oldServices := monitoredServices
	monitoredServices = []string{"unit"}
	t.Cleanup(func() { monitoredServices = oldServices })
	stubCommands(t, func(_ string, args []string) (commandOutput, error) {
		if args[0] == "systemctl" {
			return commandOutput{
				reader: io.NopCloser(
					strings.NewReader("ActiveState=" + inactiveStatus + "\n"),
				),
				wait: func() error {
					return errors.New("exit status 1")
				},
			}, nil
		}
		return commandOutput{
			reader: io.NopCloser(
				strings.NewReader(
					"USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND\nroot 42 1.5 0 0 2048 ? S 00:00 0:00 worker\n",
				),
			),
			wait: func() error {
				return errors.New("exit status 1")
			},
		}, nil
	})

	rec := httptest.NewRecorder()
	sse := datastar.NewSSE(
		rec,
		httptest.NewRequest(http.MethodGet, "/system/stream", http.NoBody),
	)
	service := &Service{}
	if err := service.sendServices(sse); err != nil {
		t.Fatal(err)
	}
	if err := service.sendProcesses(sse); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{"services-table", "inactive", "processes-table", "worker"} {
		if !strings.Contains(body, want) {
			t.Fatalf("SSE response missing %q: %s", want, body)
		}
	}
}
