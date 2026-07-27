//go:build darwin

package qrspicmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExecLaneProcessRunnerTerminatesDescendant(t *testing.T) {
	tempDir := t.TempDir()
	pidFile := filepath.Join(tempDir, "pids")
	process, err := (ExecLaneProcessRunner{}).Start(
		context.Background(),
		[]string{
			"env",
			"PID_FILE=" + pidFile,
			"bash",
			"-c",
			`sleep 30 & child=$!; printf '%s %s\n' "$$" "$child" > "$PID_FILE"; wait "$child"`,
		},
		tempDir,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Kill(-process.PID(), syscall.SIGKILL) }()

	parentPID, childPID := readLaneFixturePIDs(t, pidFile)
	if parentPID != process.PID() {
		t.Fatalf("fixture parent PID = %d, process PID = %d", parentPID, process.PID())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := process.Terminate(ctx, 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if _, err := process.Wait(ctx); err != nil {
		t.Fatalf("direct child was not reaped: %v", err)
	}
	waitForLaneProcessExit(t, childPID)
}

func readLaneFixturePIDs(t *testing.T, path string) (int, int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			fields := strings.Fields(string(data))
			if len(fields) != 2 {
				t.Fatalf("fixture PIDs = %q", data)
			}
			parent, parentErr := strconv.Atoi(fields[0])
			child, childErr := strconv.Atoi(fields[1])
			if parentErr != nil || childErr != nil {
				t.Fatalf("fixture PIDs = %q: %v %v", data, parentErr, childErr)
			}
			return parent, child
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("lane fixture did not publish PIDs")
	return 0, 0
}

func waitForLaneProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(fmt.Sprintf("lane descendant %d remains live", pid))
}
