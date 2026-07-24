package qrspicmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

const resultRecordVersion = 1

type ResultRecord struct {
	Version               int              `yaml:"version"`
	ID                    string           `yaml:"id"`
	ManagerRunID          string           `yaml:"manager_run_id"`
	SourceChildID         string           `yaml:"source_child_id"`
	SourceChildGeneration int              `yaml:"source_child_generation"`
	Node                  string           `yaml:"node"`
	State                 string           `yaml:"state"`
	Outcome               string           `yaml:"outcome,omitempty"`
	CreatedAt             time.Time        `yaml:"created_at"`
	Session               SessionReference `yaml:"session"`
	Summary               qrspi.Summary    `yaml:"summary"`
	Artifacts             []qrspi.Artifact `yaml:"artifacts,omitempty"`
}

type SessionReference struct {
	ID   string `yaml:"id"`
	Path string `yaml:"path"`
}

type ResultRecordRef struct {
	ID   string
	Path string
}

type SessionMigration struct {
	SessionDir       string
	LegacySessionDir string
	Migrated         bool
}

type ResultRecordNotFoundError struct{ Path string }

func (e ResultRecordNotFoundError) Error() string { return "result record not found: " + e.Path }

type ResultRecordPartialError struct {
	Path string
	Err  error
}

func (e ResultRecordPartialError) Error() string {
	return fmt.Sprintf("result record is partial or unreadable: %s: %v", e.Path, e.Err)
}
func (e ResultRecordPartialError) Unwrap() error { return e.Err }

type ResultRecordInvalidError struct {
	Path string
	Err  error
}

func (e ResultRecordInvalidError) Error() string {
	return fmt.Sprintf("invalid result record %s: %v", e.Path, e.Err)
}
func (e ResultRecordInvalidError) Unwrap() error { return e.Err }

func PlanMetadataDir(planDir string) string {
	return filepath.Join(filepath.Clean(planDir), ".vamos")
}

func PlanSessionDir(planDir string) string {
	return filepath.Join(PlanMetadataDir(planDir), "sessions", "pi")
}

func PlanResultDir(planDir string) string {
	return filepath.Join(PlanMetadataDir(planDir), "qrspi")
}

// MigratePlanSessions moves the old plan-local session directory only when the
// new target does not exist. Existing metadata always wins: leaving the legacy
// directory in place makes recovery deterministic and avoids silent data loss.
func MigratePlanSessions(planDir string) (SessionMigration, error) {
	planDir = filepath.Clean(planDir)
	target := PlanSessionDir(planDir)
	legacy := filepath.Join(planDir, ".sessions", "pi")

	if info, err := os.Stat(target); err == nil {
		if !info.IsDir() {
			return SessionMigration{}, fmt.Errorf(
				"plan session path is not a directory: %s",
				target,
			)
		}
		return SessionMigration{SessionDir: target, LegacySessionDir: legacy}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return SessionMigration{}, err
	}

	if _, err := os.Stat(legacy); err == nil {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return SessionMigration{}, err
		}
		if err := os.Rename(legacy, target); err != nil {
			return SessionMigration{}, fmt.Errorf("migrate legacy plan sessions: %w", err)
		}
		return SessionMigration{
			SessionDir:       target,
			LegacySessionDir: legacy,
			Migrated:         true,
		}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return SessionMigration{}, err
	}

	return SessionMigration{SessionDir: target, LegacySessionDir: legacy}, nil
}

func planSessionDirs(planDir string) ([]string, error) {
	migration, err := MigratePlanSessions(planDir)
	if err != nil {
		return nil, err
	}
	dirs := []string{migration.SessionDir}
	if !migration.Migrated {
		if info, err := os.Stat(migration.LegacySessionDir); err == nil && info.IsDir() {
			dirs = append(dirs, migration.LegacySessionDir)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return dirs, nil
}

func ThoughtsRelativePath(planDir, path string) (string, error) {
	planDir, err := filepath.EvalSymlinks(planDir)
	if err != nil {
		return "", fmt.Errorf("resolve plan directory: %w", err)
	}
	thoughtsRoot, ok := thoughtsRootForPlan(planDir)
	if !ok {
		return "", fmt.Errorf("plan directory is not inside thoughts: %s", planDir)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	if !pathWithin(thoughtsRoot, path) {
		return "", fmt.Errorf("path is outside thoughts: %s", path)
	}
	rel, err := filepath.Rel(filepath.Dir(thoughtsRoot), path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func ResolveThoughtsReference(planDir, ref string) (string, error) {
	ref = filepath.Clean(strings.TrimSpace(ref))
	if ref == "." || filepath.IsAbs(ref) ||
		(ref != "thoughts" && !strings.HasPrefix(ref, "thoughts"+string(filepath.Separator))) {
		return "", fmt.Errorf("reference must be thoughts-relative: %q", ref)
	}
	planDir, err := filepath.EvalSymlinks(planDir)
	if err != nil {
		return "", fmt.Errorf("resolve plan directory: %w", err)
	}
	thoughtsRoot, ok := thoughtsRootForPlan(planDir)
	if !ok {
		return "", fmt.Errorf("plan directory is not inside thoughts: %s", planDir)
	}
	candidate := filepath.Join(filepath.Dir(thoughtsRoot), ref)
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	if !pathWithin(planDir, resolved) {
		return "", fmt.Errorf("reference escapes current plan: %s", ref)
	}
	return resolved, nil
}

// WriteResultRecord publishes a completed immutable template without replacing
// an existing record. Linking a synced temporary file gives no-replace atomic
// publication on the plan filesystem, then both file and directory are synced.
func WriteResultRecord(path string, record ResultRecord) error {
	if err := validateRecordShape(record); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".result-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	data, err := yaml.Marshal(record)
	if err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Link(tmpPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("result record already exists: %s", path)
		}
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync result record directory: %w", err)
	}
	return nil
}

func ReadResultRecord(path string) (ResultRecord, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return ResultRecord{}, ResultRecordNotFoundError{Path: path}
	}
	if err != nil {
		return ResultRecord{}, ResultRecordPartialError{Path: path, Err: err}
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var record ResultRecord
	if err := decoder.Decode(&record); err != nil {
		var typeErr *yaml.TypeError
		if errors.As(err, &typeErr) && strings.Contains(err.Error(), "field ") {
			return ResultRecord{}, ResultRecordInvalidError{Path: path, Err: err}
		}
		return ResultRecord{}, ResultRecordPartialError{Path: path, Err: err}
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ResultRecord{}, ResultRecordInvalidError{
				Path: path,
				Err:  errors.New("multiple YAML documents"),
			}
		}
		return ResultRecord{}, ResultRecordPartialError{Path: path, Err: err}
	}
	if err := validateRecordShape(record); err != nil {
		return ResultRecord{}, ResultRecordInvalidError{Path: path, Err: err}
	}
	return record, nil
}

// ValidateResultRecord verifies the immutable fields embedded in a record. Call
// ValidateResultRecordAt when the storage path is also available; a record does
// not carry its own path so it cannot otherwise prove that binding.
func ValidateResultRecord(state ManagerState, record ResultRecord) error {
	child := state.ActiveChild
	if child == nil {
		return ResultRecordInvalidError{Err: errors.New("no active child")}
	}
	if err := validateRecordShape(record); err != nil {
		return ResultRecordInvalidError{Err: err}
	}
	for _, check := range []struct{ name, got, want string }{
		{"manager run", record.ManagerRunID, state.ManagerRunID},
		{"child", record.SourceChildID, child.ID},
		{"node", record.Node, string(state.Workflow.CurrentNodeID)},
		{"session id", record.Session.ID, child.SessionID},
		{"result id", record.ID, child.ResultID},
	} {
		if strings.TrimSpace(check.want) == "" || check.got != check.want {
			return ResultRecordInvalidError{
				Err: fmt.Errorf("%s does not match active child", check.name),
			}
		}
	}
	if record.SourceChildGeneration != child.Generation {
		return ResultRecordInvalidError{
			Err: errors.New("child generation does not match active child"),
		}
	}
	if strings.TrimSpace(child.ResultPath) == "" {
		return ResultRecordInvalidError{
			Err: errors.New("active child has no result path binding"),
		}
	}
	expectedSession := strings.TrimSpace(child.SessionPath)
	if expectedSession == "" {
		return ResultRecordInvalidError{
			Err: errors.New("active child has no durable session path"),
		}
	}
	sessionRef, err := ThoughtsRelativePath(state.CanonicalPlanDir, expectedSession)
	if err != nil || sessionRef != record.Session.Path {
		return ResultRecordInvalidError{
			Err: errors.New("session path does not match active child"),
		}
	}
	if _, err := ResolveThoughtsReference(
		state.CanonicalPlanDir,
		record.Session.Path,
	); err != nil {
		return ResultRecordInvalidError{
			Err: fmt.Errorf("invalid session reference: %w", err),
		}
	}
	if strings.TrimSpace(record.Summary.PlanGoal) == "" ||
		strings.TrimSpace(record.Summary.StageCompleted) == "" ||
		strings.TrimSpace(record.Summary.KeyDecisions) == "" {
		return ResultRecordInvalidError{Err: errors.New("result summary is incomplete")}
	}
	for _, artifact := range record.Artifacts {
		if _, err := ResolveThoughtsReference(
			state.CanonicalPlanDir,
			artifact.Path,
		); err != nil {
			return ResultRecordInvalidError{
				Err: fmt.Errorf("invalid artifact reference: %w", err),
			}
		}
	}
	return nil
}

func ValidateResultRecordAt(
	state ManagerState,
	ref ResultRecordRef,
	record ResultRecord,
) error {
	if err := ValidateResultRecord(state, record); err != nil {
		return err
	}
	if ref.ID != record.ID {
		return ResultRecordInvalidError{
			Err: errors.New("result reference ID does not match record"),
		}
	}
	child := state.ActiveChild
	if child == nil || strings.TrimSpace(child.ResultPath) == "" {
		return ResultRecordInvalidError{
			Err: errors.New("active child has no result path binding"),
		}
	}
	path := strings.TrimSpace(ref.Path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(state.CanonicalPlanDir, path)
	}
	if filepath.Clean(path) != filepath.Clean(child.ResultPath) {
		return ResultRecordInvalidError{
			Err: errors.New("result path does not match active child"),
		}
	}
	return nil
}

func validateRecordShape(record ResultRecord) error {
	if record.Version != resultRecordVersion {
		return fmt.Errorf("unsupported result record version %d", record.Version)
	}
	for _, field := range []struct{ name, value string }{
		{"id", record.ID},
		{"manager_run_id", record.ManagerRunID},
		{"source_child_id", record.SourceChildID},
		{"node", record.Node},
		{"state", record.State},
		{"session.id", record.Session.ID},
		{"session.path", record.Session.Path},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required", field.name)
		}
	}
	if record.SourceChildGeneration < 1 {
		return errors.New("source_child_generation must be positive")
	}
	if record.CreatedAt.IsZero() {
		return errors.New("created_at is required")
	}
	if len(record.Summary.PlanGoal) > 500 || len(record.Summary.StageCompleted) > 500 ||
		len(record.Summary.KeyDecisions) > 500 {
		return errors.New("summary fields must be concise")
	}
	return nil
}
