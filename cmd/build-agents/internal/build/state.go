package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const StateVersion = 1

type StepName string

const (
	StepProto          StepName = "proto"
	StepSQLC           StepName = "sqlc"
	StepTempl          StepName = "templ"
	StepGo             StepName = "go"
	StepTailwind       StepName = "tailwind"
	StepTSWorker       StepName = "ts-worker"
	StepDatastarAssets StepName = "datastar-assets"
)

type State struct {
	Version         int                  `json:"version"`
	Steps           map[string]StepState `json:"steps"`
	PendingRestarts PendingRestartState  `json:"pending_restarts"`
	LastRun         RunSummary           `json:"last_run"`
}

type StepState struct {
	InputHash        string `json:"input_hash"`
	OutputHash       string `json:"output_hash"`
	RestartInputHash string `json:"restart_input_hash,omitempty"`
}

type PendingRestartState struct {
	Web      bool `json:"web"`
	TSWorker bool `json:"ts_worker"`
}

type RunSummary struct {
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Success    bool      `json:"success"`
}

type StateStore interface {
	Load(ctx context.Context) (State, error)
	Save(ctx context.Context, state State) error
	Clean(ctx context.Context) error
}

type FileStateStore struct{ path string }

func NewFileStateStore(path string) *FileStateStore { return &FileStateStore{path: path} }

func DefaultState(repoRoot string) State {
	return State{
		Version: StateVersion,
		Steps: map[string]StepState{
			string(StepProto):          {},
			string(StepSQLC):           {},
			string(StepTempl):          {},
			string(StepGo):             {},
			string(StepTailwind):       {},
			string(StepTSWorker):       {},
			string(StepDatastarAssets): {},
		},
		PendingRestarts: PendingRestartState{},
		LastRun:         RunSummary{},
	}
}

func (s *FileStateStore) Load(ctx context.Context) (State, error) {
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultState(""), nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read state: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("parse state %s: %w", s.path, err)
	}
	if state.Version != StateVersion {
		return State{}, fmt.Errorf("unsupported state version %d", state.Version)
	}
	if state.Steps == nil {
		state.Steps = DefaultState("").Steps
	}
	return state, nil
}

func (s *FileStateStore) Save(ctx context.Context, state State) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	state.Version = StateVersion
	if state.Steps == nil {
		state.Steps = DefaultState("").Steps
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("mkdir state dir: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp state: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp state: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}

func (s *FileStateStore) Clean(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(dir, 0o755)
	}
	if err != nil {
		return fmt.Errorf("read state dir: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "build.lock" || name == "build.lock.json" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("remove builder state %s: %w", name, err)
		}
	}
	return nil
}
