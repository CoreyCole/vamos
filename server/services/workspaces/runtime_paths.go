package workspaces

import (
	"os"
	"path/filepath"
	"strings"
)

func RuntimePaths(checkoutPath string, metadataDirName ...string) WorkspaceRuntimePaths {
	name := defaultMetadataDirName
	if len(metadataDirName) > 0 && strings.TrimSpace(metadataDirName[0]) != "" {
		name = strings.TrimSpace(metadataDirName[0])
	}
	root := filepath.Join(checkoutPath, name)
	runDir := filepath.Join(root, "run")
	logDir := filepath.Join(root, "log")
	stateDir := filepath.Join(root, "state")
	return WorkspaceRuntimePaths{
		Root:          root,
		RunDir:        runDir,
		LogDir:        logDir,
		StateDir:      stateDir,
		WorkspaceEnv:  filepath.Join(runDir, "workspace.env"),
		DesiredJSON:   filepath.Join(runDir, "desired.json"),
		StatusJSON:    filepath.Join(runDir, "status.json"),
		LifecycleJSON: filepath.Join(runDir, "lifecycle.json"),
		PortsJSON:     filepath.Join(runDir, "ports.json"),
		WebPID:        filepath.Join(runDir, "web.pid"),
		TemporalPID:   filepath.Join(runDir, "temporal.pid"),
		TSWorkerPID:   filepath.Join(runDir, "ts-worker.pid"),
		WebLog:        filepath.Join(logDir, "web.log"),
		TemporalLog:   filepath.Join(logDir, "temporal.log"),
		TSWorkerLog:   filepath.Join(logDir, "ts-worker.log"),
		BuildLog:      filepath.Join(logDir, "build.log"),
		AgentsDB:      filepath.Join(stateDir, "agents.db"),
		TemporalDB:    filepath.Join(stateDir, "temporal.db"),
		OpenClawDir:   filepath.Join(stateDir, "openclaw"),
		TSReadyMarker: filepath.Join(runDir, "ts-worker.ready"),
	}
}

func EnsureRuntimeDirs(paths WorkspaceRuntimePaths) error {
	for _, dir := range []string{paths.RunDir, paths.LogDir, paths.StateDir, paths.OpenClawDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}
