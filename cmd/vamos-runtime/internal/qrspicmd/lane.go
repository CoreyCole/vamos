package qrspicmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	laneSchemaVersion      = 1
	maxLaneDiagnosticBytes = 128 * 1024
)

type LaneRole string

const (
	LaneRoleReviewScout LaneRole = "qrspi-review-scout"
	LaneRoleReviewer    LaneRole = "qrspi-reviewer"
)

type LaneState string

const (
	LaneQueued  LaneState = "queued"
	LaneRunning LaneState = "running"
	LaneSuccess LaneState = "success"
	LaneFailed  LaneState = "failed"
)

type LaneSpec struct {
	ID             string        `json:"id"`
	CoordinatorID  string        `json:"coordinatorId"`
	CoordinatorGen int           `json:"coordinatorGeneration"`
	Role           LaneRole      `json:"role"`
	PromptFile     string        `json:"promptFile"`
	Cwd            string        `json:"cwd"`
	PlanDir        string        `json:"planDir"`
	ReviewDir      string        `json:"reviewDir"`
	ReportPath     string        `json:"reportPath"`
	SessionID      string        `json:"sessionId"`
	SessionDir     string        `json:"sessionDir"`
	Attempt        int           `json:"attempt"`
	Timeout        time.Duration `json:"timeout"`
}

type LaneRecord struct {
	SchemaVersion int       `json:"schemaVersion"`
	Spec          LaneSpec  `json:"spec"`
	State         LaneState `json:"state"`
	PID           int       `json:"pid,omitempty"`
	StartedAt     time.Time `json:"startedAt,omitempty"`
	FinishedAt    time.Time `json:"finishedAt,omitempty"`
	StatusPath    string    `json:"statusPath"`
	EventsPath    string    `json:"eventsPath"`
	OutputPath    string    `json:"outputPath"`
	ErrorTail     string    `json:"errorTail,omitempty"`
}

type LaneExit struct {
	ExitCode  int
	ErrorTail string
}

type LaneProcess interface{ PID() int }

type LaneProcessRunner interface {
	Start(ctx context.Context, command []string, cwd string) (LaneProcess, error)
	Wait(ctx context.Context, process LaneProcess) (LaneExit, error)
	Cancel(ctx context.Context, process LaneProcess) error
}

type LaneRunner interface {
	Start(ctx context.Context, spec LaneSpec) (LaneRecord, error)
	Wait(ctx context.Context, record LaneRecord) (LaneRecord, error)
}

type LaneCoordinator struct {
	Runner      LaneRunner
	MaxParallel int
}

func LaneAttemptDir(spec LaneSpec) string {
	return filepath.Join(
		spec.PlanDir,
		".vamos",
		"qrspi",
		"lanes",
		spec.CoordinatorID,
		spec.ID,
		fmt.Sprintf("attempt-%d", spec.Attempt),
	)
}

func LaneRecordPath(
	spec LaneSpec,
) string {
	return filepath.Join(LaneAttemptDir(spec), "record.json")
}

func LaneStatusPath(
	spec LaneSpec,
) string {
	return filepath.Join(LaneAttemptDir(spec), "status.json")
}

func LaneEventsPath(
	spec LaneSpec,
) string {
	return filepath.Join(LaneAttemptDir(spec), "events.jsonl")
}

func LaneOutputPath(
	spec LaneSpec,
) string {
	return filepath.Join(LaneAttemptDir(spec), "output.txt")
}

func ValidateLaneSpec(spec LaneSpec) error {
	for field, value := range map[string]string{
		"lane ID": spec.ID, "coordinator ID": spec.CoordinatorID, "prompt file": spec.PromptFile,
		"cwd": spec.Cwd, "plan directory": spec.PlanDir, "review directory": spec.ReviewDir,
		"report path": spec.ReportPath, "session ID": spec.SessionID, "session directory": spec.SessionDir,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", field)
		}
	}
	for _, value := range []string{spec.ID, spec.CoordinatorID, spec.SessionID} {
		if unsafeLaneID(value) {
			return fmt.Errorf("unsafe lane identity %q", value)
		}
	}
	if spec.CoordinatorGen < 1 {
		return errors.New("coordinator generation must be positive")
	}
	if spec.Role != LaneRoleReviewScout && spec.Role != LaneRoleReviewer {
		return fmt.Errorf("unknown lane role %q", spec.Role)
	}
	if spec.Attempt < 1 || spec.Attempt > 2 {
		return errors.New("lane attempt must be 1 or 2")
	}
	if spec.Timeout <= 0 {
		return errors.New("lane timeout must be positive")
	}
	for field, value := range map[string]string{"prompt file": spec.PromptFile, "cwd": spec.Cwd, "plan directory": spec.PlanDir, "review directory": spec.ReviewDir, "report path": spec.ReportPath, "session directory": spec.SessionDir} {
		if !filepath.IsAbs(value) {
			return fmt.Errorf("%s must be absolute", field)
		}
	}
	if err := requireContained(spec.ReviewDir, spec.ReportPath); err != nil {
		return fmt.Errorf("report path: %w", err)
	}
	if err := requireContained(spec.PlanDir, LaneAttemptDir(spec)); err != nil {
		return fmt.Errorf("lane metadata: %w", err)
	}
	return nil
}

func ValidateLaneSpecs(specs []LaneSpec) error {
	reports := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if err := ValidateLaneSpec(spec); err != nil {
			return err
		}
		report, err := canonicalOrAbsolute(spec.ReportPath)
		if err != nil {
			return err
		}
		if _, exists := reports[report]; exists {
			return fmt.Errorf("duplicate lane report target %q", spec.ReportPath)
		}
		reports[report] = struct{}{}
	}
	return nil
}

func ValidateLaneReport(spec LaneSpec) error {
	if err := ValidateLaneSpec(spec); err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Ext(spec.ReportPath), ".md") {
		return errors.New("lane report must be a Markdown file")
	}
	if err := requireContained(spec.ReviewDir, spec.ReportPath); err != nil {
		return fmt.Errorf("report path: %w", err)
	}
	info, err := os.Stat(spec.ReportPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("lane report must be a regular file")
	}
	data, err := os.ReadFile(spec.ReportPath)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return errors.New("lane report must be substantive")
	}
	return nil
}

func WriteLaneRecord(path string, record LaneRecord) error {
	if record.SchemaVersion == 0 {
		record.SchemaVersion = laneSchemaVersion
	}
	if err := ValidateLaneSpec(record.Spec); err != nil {
		return err
	}
	if path == "" {
		return errors.New("lane record path is required")
	}
	if err := requireContained(record.Spec.PlanDir, path); err != nil {
		return fmt.Errorf("record path: %w", err)
	}
	if current, err := ReadLaneRecord(
		path,
	); err == nil && terminalLaneState(current.State) &&
		!terminalLaneState(record.State) {
		return nil
	} else if err != nil &&
		!errors.Is(err, os.ErrNotExist) {
		return err
	}
	record.ErrorTail = boundedLaneDiagnostic(record.ErrorTail)
	return writeJSONAtomically(path, record)
}

func ReadLaneRecord(path string) (LaneRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LaneRecord{}, err
	}
	var record LaneRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return LaneRecord{}, fmt.Errorf("decode lane record: %w", err)
	}
	if record.SchemaVersion != laneSchemaVersion {
		return LaneRecord{}, fmt.Errorf(
			"unsupported lane record schema %d",
			record.SchemaVersion,
		)
	}
	if err := ValidateLaneSpec(record.Spec); err != nil {
		return LaneRecord{}, err
	}
	if record.State != LaneQueued && record.State != LaneRunning &&
		record.State != LaneSuccess &&
		record.State != LaneFailed {
		return LaneRecord{}, fmt.Errorf("unknown lane state %q", record.State)
	}
	return record, nil
}

func unsafeLaneID(value string) bool {
	return value == "." || value == ".." || strings.ContainsAny(value, `/\\`) ||
		strings.Contains(value, "..")
}

func terminalLaneState(
	state LaneState,
) bool {
	return state == LaneSuccess || state == LaneFailed
}

func boundedLaneDiagnostic(value string) string {
	if len(value) <= maxLaneDiagnosticBytes {
		return value
	}
	return value[len(value)-maxLaneDiagnosticBytes:]
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func canonicalOrAbsolute(path string) (string, error) {
	canonical, err := canonicalPath(path)
	if err == nil {
		return canonical, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return filepath.Abs(path)
}

func requireContained(root, target string) error {
	rootPath, err := canonicalPath(root)
	if err != nil {
		return err
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absoluteTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	// Reject lexical escapes first, then resolve the deepest existing ancestor.
	// This permits metadata directories that do not exist yet without accepting
	// an existing symlink that leaves the selected root.
	rel, err := filepath.Rel(absoluteRoot, absoluteTarget)
	if err != nil || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(rel) {
		return errors.New("must remain contained")
	}
	ancestor := absoluteTarget
	var suffix []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(ancestor)
		if resolveErr == nil {
			resolvedTarget := filepath.Join(
				append([]string{resolved}, reversePathParts(suffix)...)...)
			resolvedRel, relErr := filepath.Rel(rootPath, resolvedTarget)
			if relErr != nil || resolvedRel == ".." ||
				strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) ||
				filepath.IsAbs(resolvedRel) {
				return errors.New("must remain contained")
			}
			return nil
		}
		if !errors.Is(resolveErr, os.ErrNotExist) {
			return resolveErr
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return resolveErr
		}
		suffix = append(suffix, filepath.Base(ancestor))
		ancestor = parent
	}
}

func reversePathParts(parts []string) []string {
	for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
		parts[left], parts[right] = parts[right], parts[left]
	}
	return parts
}
