package workspaces

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type SystemLocalProber struct{}

func NewSystemLocalProber() SystemLocalProber { return SystemLocalProber{} }

func (SystemLocalProber) PIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func (SystemLocalProber) PortOpen(addr string) bool {
	if strings.TrimSpace(addr) == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (SystemLocalProber) HTTPHost(
	ctx context.Context,
	addr, host, path string,
) (*http.Response, []byte, error) {
	if path == "" {
		path = "/"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+path, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Host = host
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	closeErr := resp.Body.Close()
	if readErr != nil {
		return resp, body, readErr
	}
	return resp, body, closeErr
}

func (d WorkspaceDiagnostics) RuntimeStatus() RuntimeStatus {
	if d.RuntimeState != nil {
		return *d.RuntimeState
	}
	status := RuntimeStatus{
		Status: d.Workspace.Status,
		Phase:  d.Workspace.Phase,
		Error:  d.LatestError,
		Logs:   bundleLogs(d.Workspace.Bundle),
		Ports:  d.Workspace.Ports,
		PIDs:   d.Workspace.PIDs,
		Build:  d.Workspace.BuildStatus,
	}
	if status.Ports == nil && d.Workspace.Port != 0 {
		status.Ports = map[BundleComponent]int{ComponentWeb: d.Workspace.Port}
	}
	if status.PIDs == nil && d.Workspace.PID != 0 {
		status.PIDs = map[BundleComponent]int{ComponentWeb: d.Workspace.PID}
	}
	if status.Logs == nil {
		status.Logs = map[BundleComponent]string{}
	}
	if status.Logs[ComponentWeb] == "" && d.LogPath != "" {
		status.Logs[ComponentWeb] = d.LogPath
	}
	return status
}

func BuildWorkspaceDiagnostics(
	ctx context.Context,
	manager Manager,
	tailer LogTailer,
	prober LocalProber,
	slug string,
	tailLines int,
) (WorkspaceDiagnostics, error) {
	if manager == nil {
		return WorkspaceDiagnostics{}, fmt.Errorf("workspace manager is not configured")
	}
	if err := manager.Refresh(ctx); err != nil {
		return WorkspaceDiagnostics{}, err
	}
	ws, ok := manager.Lookup(slug)
	if !ok {
		return WorkspaceDiagnostics{}, fmt.Errorf("unknown workspace %q", slug)
	}

	paths := ws.Bundle
	if paths.WorkspaceEnv == "" {
		paths = RuntimePaths(ws.CheckoutPath)
	}
	metadataPath := paths.WorkspaceEnv
	metadataRaw, _ := os.ReadFile(metadataPath)
	metadata, metadataErr := ReadMetadata(metadataPath)
	if metadataErr != nil {
		legacyPath := WorkspaceMetadataPath(ws.CheckoutPath)
		if legacyMetadata, legacyErr := ReadMetadata(legacyPath); legacyErr == nil {
			metadataPath = legacyPath
			metadataRaw, _ = os.ReadFile(legacyPath)
			metadata = legacyMetadata
			metadataErr = nil
		}
	}
	var metadataPtr *WorkspaceMetadata
	if metadataErr == nil {
		metadata.RestartToken = ""
		metadataPtr = &metadata
	}
	store := FileBundleStore{}
	var runtimeStatusPtr *RuntimeStatus
	runtimeStatus, runtimeStatusErr := store.ReadStatus(ws)
	if runtimeStatusErr == nil {
		runtimeStatusPtr = &runtimeStatus
	}
	var desiredStatePtr *DesiredState
	desiredState, desiredStateErr := store.ReadDesired(ws)
	if desiredStateErr == nil {
		desiredStatePtr = &desiredState
	}
	var runtimeEnvSnapshotPtr *RuntimeEnvSnapshot
	runtimeEnvSnapshot, runtimeEnvSnapshotErr := store.ReadRuntimeEnvSnapshot(ws)
	if runtimeEnvSnapshotErr == nil {
		runtimeEnvSnapshotPtr = &runtimeEnvSnapshot
	}
	var tsWorkerIdentityPtr *TSWorkerIdentityMarker
	tsWorkerIdentity, tsWorkerIdentityErr := ReadTSWorkerIdentityMarker(paths.TSReadyMarker)
	if tsWorkerIdentityErr == nil {
		tsWorkerIdentityPtr = &tsWorkerIdentity
	}

	logPath := ws.LogPath
	if logPath == "" && ws.StateDir != "" {
		logPath = filepath.Join(ws.StateDir, "agents-server.log")
	}
	logTail := ""
	if tailer != nil && logPath != "" {
		if tail, err := tailer.Tail(logPath, tailLines); err == nil {
			logTail = tail
		}
	}

	pidAlive := false
	portOpen := false
	if prober != nil {
		pidAlive = prober.PIDAlive(ws.PID)
		portOpen = prober.PortOpen(ws.LocalAddr())
	}

	managerURL := ""
	if metadataPtr != nil {
		managerURL = metadataPtr.ManagerURL
	}
	diagnostics := WorkspaceDiagnostics{
		Workspace:          ws,
		Metadata:           metadataPtr,
		MetadataRaw:        redactWorkspaceMetadataRaw(string(metadataRaw)),
		MetadataPath:       metadataPath,
		RuntimeState:       runtimeStatusPtr,
		DesiredState:       desiredStatePtr,
		RuntimeEnvSnapshot: runtimeEnvSnapshotPtr,
		TSWorkerIdentity:   tsWorkerIdentityPtr,
		PIDAlive:           pidAlive,
		PortOpen:           portOpen,
		LogPath:            logPath,
		LogTail:            logTail,
		ManagerURL:         managerURL,
		PublicURL:          ws.URL,
		LatestError:        ws.Error,
	}
	if runtimeStatusErr != nil {
		diagnostics.RuntimeStatusError = runtimeStatusErr.Error()
	}
	if desiredStateErr != nil {
		diagnostics.DesiredStateError = desiredStateErr.Error()
	}
	if runtimeEnvSnapshotErr != nil {
		diagnostics.RuntimeEnvSnapshotError = runtimeEnvSnapshotErr.Error()
	}
	if tsWorkerIdentityErr != nil {
		diagnostics.TSWorkerIdentityError = tsWorkerIdentityErr.Error()
	}
	return diagnostics, nil
}

func redactWorkspaceMetadataRaw(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		key, _, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(key) == "VAMOS_WORKSPACE_RESTART_TOKEN" {
			lines[i] = key + "='[redacted]'"
		}
	}
	return strings.Join(lines, "\n")
}
