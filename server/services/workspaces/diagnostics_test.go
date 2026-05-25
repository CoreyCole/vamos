package workspaces

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileLogTailerReturnsLastNLines(t *testing.T) {
	t.Parallel()

	logPath := filepath.Join(t.TempDir(), "agents-server.log")
	writeTestFile(t, logPath, "line1\nline2\nline3\nline4\nline5\n")
	tail, err := NewFileLogTailer().Tail(logPath, 2)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if tail != "line4\nline5" {
		t.Fatalf("tail = %q, want line4\\nline5", tail)
	}
}

func TestBuildWorkspaceDiagnostics(t *testing.T) {
	t.Parallel()

	checkout := t.TempDir()
	stateDir := t.TempDir()
	logPath := filepath.Join(stateDir, "agents-server.log")
	writeTestFile(t, logPath, "old\nnew\n")
	if err := WriteMetadata(WorkspaceMetadataPath(checkout), WorkspaceMetadata{
		Slug:         "demo",
		CheckoutPath: checkout,
		ManagerURL:   "https://main.cn-agents.test",
		RestartToken: "secret-restart-token",
		PID:          123,
		Port:         4321,
	}); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}
	ws := Workspace{
		Slug:         "demo",
		CheckoutPath: checkout,
		URL:          "https://demo.cn-agents.test/",
		PID:          123,
		Port:         4321,
		StateDir:     stateDir,
		LogPath:      logPath,
		Ports: map[BundleComponent]int{
			ComponentWeb:      4321,
			ComponentTemporal: 7233,
		},
		PIDs: map[BundleComponent]int{
			ComponentWeb:      123,
			ComponentTSWorker: 456,
		},
	}
	if err := (FileBundleStore{}).WriteRuntimeEnvSnapshot(ws, BuildRuntimeEnvSnapshot(ws, RuntimeConfig{}, ws.Ports, ws.PIDs, time.Unix(100, 0))); err != nil {
		t.Fatalf("WriteRuntimeEnvSnapshot: %v", err)
	}
	writeTSWorkerIdentityMarkerForTest(t, ws)
	manager := &diagnosticsFakeManager{workspaces: map[string]Workspace{"demo": ws}}
	prober := diagnosticsFakeProber{pidAlive: true, portOpen: true}
	diagnostics, err := BuildWorkspaceDiagnostics(
		context.Background(),
		manager,
		NewFileLogTailer(),
		prober,
		"demo",
		1,
	)
	if err != nil {
		t.Fatalf("BuildWorkspaceDiagnostics: %v", err)
	}
	if diagnostics.Workspace.Slug != "demo" || diagnostics.Metadata == nil ||
		diagnostics.Metadata.Slug != "demo" {
		t.Fatalf(
			"diagnostics workspace/metadata = %#v %#v",
			diagnostics.Workspace,
			diagnostics.Metadata,
		)
	}
	if diagnostics.MetadataRaw == "" || diagnostics.LogTail != "new" {
		t.Fatalf(
			"metadata raw/log tail = %q/%q",
			diagnostics.MetadataRaw,
			diagnostics.LogTail,
		)
	}
	if diagnostics.Metadata.RestartToken != "" ||
		strings.Contains(diagnostics.MetadataRaw, "secret-restart-token") {
		t.Fatalf(
			"metadata leaked restart token: metadata=%#v raw=%q",
			diagnostics.Metadata,
			diagnostics.MetadataRaw,
		)
	}
	if !diagnostics.PIDAlive || !diagnostics.PortOpen {
		t.Fatalf(
			"pid/port = %v/%v, want true/true",
			diagnostics.PIDAlive,
			diagnostics.PortOpen,
		)
	}
	if diagnostics.RuntimeEnvSnapshot == nil || diagnostics.TSWorkerIdentity == nil {
		t.Fatalf("proof diagnostics snapshot=%#v identity=%#v errors=%q/%q", diagnostics.RuntimeEnvSnapshot, diagnostics.TSWorkerIdentity, diagnostics.RuntimeEnvSnapshotError, diagnostics.TSWorkerIdentityError)
	}
	if diagnostics.PublicURL != "https://demo.cn-agents.test/" ||
		diagnostics.ManagerURL != "https://main.cn-agents.test" {
		t.Fatalf(
			"urls = public %q manager %q",
			diagnostics.PublicURL,
			diagnostics.ManagerURL,
		)
	}
}

func TestBuildWorkspaceDiagnosticsReportsProofReadErrors(t *testing.T) {
	t.Parallel()

	checkout := t.TempDir()
	if err := WriteMetadata(WorkspaceMetadataPath(checkout), WorkspaceMetadata{
		Slug:         "demo",
		CheckoutPath: checkout,
		ManagerURL:   "https://main.test",
	}); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}
	paths := RuntimePaths(checkout)
	writeTestFile(t, paths.RuntimeEnvSnapshot, "{bad json")
	writeTestFile(t, paths.TSReadyMarker, "{bad json")
	manager := &diagnosticsFakeManager{workspaces: map[string]Workspace{
		"demo": {Slug: "demo", CheckoutPath: checkout, Bundle: paths},
	}}

	diagnostics, err := BuildWorkspaceDiagnostics(context.Background(), manager, nil, nil, "demo", 0)
	if err != nil {
		t.Fatalf("BuildWorkspaceDiagnostics: %v", err)
	}
	if diagnostics.RuntimeEnvSnapshot != nil || diagnostics.RuntimeEnvSnapshotError == "" {
		t.Fatalf("runtime snapshot proof = %#v error %q", diagnostics.RuntimeEnvSnapshot, diagnostics.RuntimeEnvSnapshotError)
	}
	if diagnostics.TSWorkerIdentity != nil || diagnostics.TSWorkerIdentityError == "" {
		t.Fatalf("ts identity proof = %#v error %q", diagnostics.TSWorkerIdentity, diagnostics.TSWorkerIdentityError)
	}
}

func TestWorkspaceDiagnosticsRuntimeStatusIncludesBundleFacts(t *testing.T) {
	t.Parallel()

	paths := RuntimePaths(t.TempDir())
	diagnostics := WorkspaceDiagnostics{
		Workspace: Workspace{
			Status: StatusRunning,
			Bundle: paths,
			Ports: map[BundleComponent]int{
				ComponentWeb:      4101,
				ComponentTemporal: 7233,
			},
			PIDs: map[BundleComponent]int{
				ComponentWeb:      111,
				ComponentTSWorker: 333,
			},
			BuildStatus: BuildStatus{Error: "templ failed", LogPath: paths.BuildLog},
		},
	}
	runtimeStatus := diagnostics.RuntimeStatus()
	if runtimeStatus.Status != StatusRunning ||
		runtimeStatus.Build.LogPath != paths.BuildLog ||
		runtimeStatus.Ports[ComponentTemporal] != 7233 ||
		runtimeStatus.PIDs[ComponentTSWorker] != 333 ||
		runtimeStatus.Logs[ComponentTSWorker] != paths.TSWorkerLog {
		t.Fatalf("runtime status = %#v", runtimeStatus)
	}
}

func TestBuildWorkspaceDiagnosticsUnknownSlug(t *testing.T) {
	t.Parallel()

	manager := &diagnosticsFakeManager{workspaces: map[string]Workspace{}}
	if _, err := BuildWorkspaceDiagnostics(
		context.Background(),
		manager,
		nil,
		nil,
		"missing",
		0,
	); err == nil {
		t.Fatal("BuildWorkspaceDiagnostics error = nil, want unknown workspace")
	}
}

type diagnosticsFakeManager struct {
	workspaces map[string]Workspace
}

func (m *diagnosticsFakeManager) Refresh(context.Context) error { return nil }
func (m *diagnosticsFakeManager) List() []Workspace {
	items := make([]Workspace, 0, len(m.workspaces))
	for _, ws := range m.workspaces {
		items = append(items, ws)
	}
	return items
}

func (m *diagnosticsFakeManager) Lookup(slug string) (Workspace, bool) {
	ws, ok := m.workspaces[slug]
	return ws, ok
}

func (m *diagnosticsFakeManager) LookupHost(
	string,
) (Workspace, bool) {
	return Workspace{}, false
}

func (m *diagnosticsFakeManager) Start(context.Context, string) (Workspace, error) {
	return Workspace{}, nil
}

func (m *diagnosticsFakeManager) Stop(context.Context, string) (Workspace, error) {
	return Workspace{}, nil
}

func (m *diagnosticsFakeManager) Restart(context.Context, string) (Workspace, error) {
	return Workspace{}, nil
}

type diagnosticsFakeProber struct {
	pidAlive bool
	portOpen bool
}

func (p diagnosticsFakeProber) PIDAlive(int) bool    { return p.pidAlive }
func (p diagnosticsFakeProber) PortOpen(string) bool { return p.portOpen }

func (p diagnosticsFakeProber) HTTPHost(
	context.Context,
	string,
	string,
	string,
) (*http.Response, []byte, error) {
	return nil, nil, nil
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
