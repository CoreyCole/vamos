package workspaces

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
)

type BundleStore interface {
	Paths(ws Workspace) WorkspaceRuntimePaths
	ReadStatus(ws Workspace) (RuntimeStatus, error)
	WriteStatus(ws Workspace, status RuntimeStatus) error
	ReadDesired(ws Workspace) (DesiredState, error)
	WriteDesired(ws Workspace, desired DesiredState) error
	ReadLifecycle(ws Workspace) (WorkspaceLifecycleState, error)
	WriteLifecycle(ws Workspace, state WorkspaceLifecycleState) error
	ReadWorkspaceEnv(ws Workspace) (WorkspaceEnv, error)
	WriteWorkspaceEnv(ws Workspace, env WorkspaceEnv) error
	ReadRuntimeEnvSnapshot(ws Workspace) (RuntimeEnvSnapshot, error)
	WriteRuntimeEnvSnapshot(ws Workspace, snapshot RuntimeEnvSnapshot) error
}

type FileBundleStore struct{}

func (FileBundleStore) Paths(ws Workspace) WorkspaceRuntimePaths {
	return RuntimePaths(ws.CheckoutPath, ws.MetadataDirName)
}

func (s FileBundleStore) ReadStatus(ws Workspace) (RuntimeStatus, error) {
	var status RuntimeStatus
	if err := readJSONFile(s.Paths(ws).StatusJSON, &status); err != nil {
		return RuntimeStatus{}, err
	}
	return status, nil
}

func (s FileBundleStore) WriteStatus(ws Workspace, status RuntimeStatus) error {
	return writeJSONFile(s.Paths(ws).StatusJSON, status, 0o644)
}

func (s FileBundleStore) ReadDesired(ws Workspace) (DesiredState, error) {
	var desired DesiredState
	if err := readJSONFile(s.Paths(ws).DesiredJSON, &desired); err != nil {
		return DesiredState{}, err
	}
	return desired, nil
}

func (s FileBundleStore) WriteDesired(ws Workspace, desired DesiredState) error {
	return writeJSONFile(s.Paths(ws).DesiredJSON, desired, 0o644)
}

func (s FileBundleStore) ReadLifecycle(ws Workspace) (WorkspaceLifecycleState, error) {
	var state WorkspaceLifecycleState
	if err := readJSONFile(s.Paths(ws).LifecycleJSON, &state); err != nil {
		return WorkspaceLifecycleState{}, err
	}
	return state, nil
}

func (s FileBundleStore) WriteLifecycle(
	ws Workspace,
	state WorkspaceLifecycleState,
) error {
	return writeJSONFile(s.Paths(ws).LifecycleJSON, state, 0o644)
}

func (s FileBundleStore) ReadWorkspaceEnv(ws Workspace) (WorkspaceEnv, error) {
	meta, err := ReadMetadata(s.Paths(ws).WorkspaceEnv)
	if err != nil {
		return WorkspaceEnv{}, err
	}
	return WorkspaceEnv{
		Slug:         meta.Slug,
		CheckoutPath: meta.CheckoutPath,
		ManagerURL:   meta.ManagerURL,
		RestartToken: meta.RestartToken,
		DatabasePath: meta.DatabasePath,
	}, nil
}

func (s FileBundleStore) WriteWorkspaceEnv(ws Workspace, env WorkspaceEnv) error {
	return WriteMetadata(s.Paths(ws).WorkspaceEnv, WorkspaceMetadata{
		Slug:         env.Slug,
		CheckoutPath: env.CheckoutPath,
		ManagerURL:   env.ManagerURL,
		RestartToken: env.RestartToken,
		DatabasePath: env.DatabasePath,
	})
}

func (s FileBundleStore) ReadRuntimeEnvSnapshot(ws Workspace) (RuntimeEnvSnapshot, error) {
	var snapshot RuntimeEnvSnapshot
	if err := readJSONFile(s.Paths(ws).RuntimeEnvSnapshot, &snapshot); err != nil {
		return RuntimeEnvSnapshot{}, err
	}
	return snapshot, nil
}

func (s FileBundleStore) WriteRuntimeEnvSnapshot(ws Workspace, snapshot RuntimeEnvSnapshot) error {
	return writeJSONFile(s.Paths(ws).RuntimeEnvSnapshot, snapshot, 0o644)
}

func readJSONFile(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func writeJSONFile(path string, value any, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
