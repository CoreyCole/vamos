package workspaces

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRuntimePathsUseCheckoutLocalBundle(t *testing.T) {
	checkout := t.TempDir()
	paths := RuntimePaths(checkout)
	for name, path := range map[string]string{
		"root":            paths.Root,
		"run":             paths.RunDir,
		"log":             paths.LogDir,
		"state":           paths.StateDir,
		"workspace env":   paths.WorkspaceEnv,
		"desired json":    paths.DesiredJSON,
		"status json":     paths.StatusJSON,
		"lifecycle json":  paths.LifecycleJSON,
		"ports json":      paths.PortsJSON,
		"web pid":         paths.WebPID,
		"temporal pid":    paths.TemporalPID,
		"ts worker pid":   paths.TSWorkerPID,
		"web log":         paths.WebLog,
		"temporal log":    paths.TemporalLog,
		"ts worker log":   paths.TSWorkerLog,
		"build log":       paths.BuildLog,
		"agents db":       paths.AgentsDB,
		"temporal db":     paths.TemporalDB,
		"openclaw dir":    paths.OpenClawDir,
		"ts ready marker": paths.TSReadyMarker,
	} {
		if !isSubpath(filepath.Join(checkout, ".vamos"), path) {
			t.Fatalf("%s path %q is not under checkout .vamos", name, path)
		}
	}
}

func TestEnsureRuntimeDirs(t *testing.T) {
	paths := RuntimePaths(t.TempDir())
	if err := EnsureRuntimeDirs(paths); err != nil {
		t.Fatalf("EnsureRuntimeDirs: %v", err)
	}
	for _, dir := range []string{paths.RunDir, paths.LogDir, paths.StateDir, paths.OpenClawDir} {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			t.Fatalf("dir %q stat=%v err=%v", dir, st, err)
		}
	}
}

func TestFileBundleStoreRoundTrips(t *testing.T) {
	ws := Workspace{CheckoutPath: t.TempDir()}
	store := FileBundleStore{}
	status := RuntimeStatus{
		Status: StatusStarting,
		Phase:  PhaseStartingWeb,
		Error:  "waiting",
		Logs:   map[BundleComponent]string{ComponentWeb: "web.log"},
		Ports:  map[BundleComponent]int{ComponentWeb: 4201},
		PIDs:   map[BundleComponent]int{ComponentWeb: 1234},
		Build:  BuildStatus{Error: "boom", LogPath: "build.log"},
	}
	if err := store.WriteStatus(ws, status); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}
	gotStatus, err := store.ReadStatus(ws)
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	if gotStatus.Status != status.Status || gotStatus.Phase != status.Phase ||
		gotStatus.Error != status.Error ||
		gotStatus.Ports[ComponentWeb] != 4201 ||
		gotStatus.PIDs[ComponentWeb] != 1234 ||
		gotStatus.Build.Error != "boom" {
		t.Fatalf("status = %#v", gotStatus)
	}

	desired := DesiredState{Desired: StatusRunning}
	if err := store.WriteDesired(ws, desired); err != nil {
		t.Fatalf("WriteDesired: %v", err)
	}
	gotDesired, err := store.ReadDesired(ws)
	if err != nil {
		t.Fatalf("ReadDesired: %v", err)
	}
	if gotDesired != desired {
		t.Fatalf("desired = %#v want %#v", gotDesired, desired)
	}

	lifecycle := WorkspaceLifecycleState{
		DesiredState:        WorkspaceDesiredRunning,
		ObservedState:       WorkspaceObservedStarting,
		TransitionKind:      WorkspaceTransitionStart,
		TransitionID:        "transition-1",
		TransitionStartedAt: time.Date(2026, 5, 15, 1, 2, 3, 0, time.UTC),
		TransitionUpdatedAt: time.Date(2026, 5, 15, 1, 2, 4, 0, time.UTC),
	}
	if err := store.WriteLifecycle(ws, lifecycle); err != nil {
		t.Fatalf("WriteLifecycle: %v", err)
	}
	gotLifecycle, err := store.ReadLifecycle(ws)
	if err != nil {
		t.Fatalf("ReadLifecycle: %v", err)
	}
	if gotLifecycle.DesiredState != lifecycle.DesiredState ||
		gotLifecycle.ObservedState != lifecycle.ObservedState ||
		gotLifecycle.TransitionKind != lifecycle.TransitionKind ||
		gotLifecycle.TransitionID != lifecycle.TransitionID {
		t.Fatalf("lifecycle = %#v want %#v", gotLifecycle, lifecycle)
	}

	env := WorkspaceEnv{
		Slug:         "feature",
		CheckoutPath: ws.CheckoutPath,
		ManagerURL:   "https://main.cn-agents.test",
		RestartToken: "tok'en",
	}
	if err := store.WriteWorkspaceEnv(ws, env); err != nil {
		t.Fatalf("WriteWorkspaceEnv: %v", err)
	}
	gotEnv, err := store.ReadWorkspaceEnv(ws)
	if err != nil {
		t.Fatalf("ReadWorkspaceEnv: %v", err)
	}
	if gotEnv != env {
		t.Fatalf("env = %#v want %#v", gotEnv, env)
	}
}

func TestFileBundleStoreRoundTripsLifecycle(t *testing.T) {
	ws := Workspace{CheckoutPath: t.TempDir()}
	store := FileBundleStore{}
	state := WorkspaceLifecycleState{
		DesiredState:        WorkspaceDesiredRunning,
		ObservedState:       WorkspaceObservedStarting,
		TransitionKind:      WorkspaceTransitionStart,
		TransitionID:        "transition-1",
		TransitionStartedAt: time.Date(2026, 5, 15, 1, 2, 3, 0, time.UTC),
		TransitionUpdatedAt: time.Date(2026, 5, 15, 1, 2, 4, 0, time.UTC),
	}
	if err := store.WriteLifecycle(ws, state); err != nil {
		t.Fatalf("WriteLifecycle: %v", err)
	}
	got, err := store.ReadLifecycle(ws)
	if err != nil {
		t.Fatalf("ReadLifecycle: %v", err)
	}
	if got.DesiredState != state.DesiredState ||
		got.ObservedState != state.ObservedState ||
		got.TransitionID != state.TransitionID {
		t.Fatalf("lifecycle = %#v want %#v", got, state)
	}
}

func TestReadRuntimeStatusCorruptJSONReturnsError(t *testing.T) {
	ws := Workspace{CheckoutPath: t.TempDir()}
	paths := RuntimePaths(ws.CheckoutPath)
	if err := EnsureRuntimeDirs(paths); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.StatusJSON, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := FileBundleStore{}.ReadStatus(ws)
	if err == nil {
		t.Fatal("ReadStatus err = nil, want corrupt JSON error")
	}
}

func TestFileBundleStoreMissingStatusReturnsNotExist(t *testing.T) {
	_, err := FileBundleStore{}.ReadStatus(Workspace{CheckoutPath: t.TempDir()})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err=%v, want os.ErrNotExist", err)
	}
}

func isSubpath(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "" && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
