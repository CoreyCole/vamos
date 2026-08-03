package qrspicmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	laneSchemaVersion      = 1
	maxLaneDiagnosticBytes = 128 * 1024
	laneTerminationGrace   = 2 * time.Second
	laneReapDeadline       = 5 * time.Second
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

type LaneProcess interface {
	PID() int
	Terminate(ctx context.Context, grace time.Duration) error
	Wait(ctx context.Context) (LaneExit, error)
}

type LaneProcessRunner interface {
	Start(ctx context.Context, command []string, cwd string) (LaneProcess, error)
}

type laneDependencies struct {
	LaneProcessRunner LaneProcessRunner
}

func writeJSONAtomically(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		_ = temporary.Close()

		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}

	return os.Rename(temporaryPath, path)
}

type LaneRunner interface {
	Start(ctx context.Context, spec LaneSpec) (LaneRecord, error)
	Wait(ctx context.Context, record LaneRecord) (LaneRecord, error)
}

type LaneCoordinator struct {
	Runner      LaneRunner
	MaxParallel int
}

// DetachedLaneRunner is deliberately separate from the interactive child runner.
// It owns only one local process and its diagnostic record; it has no manager or
// graph/result/wake dependencies.
type DetachedLaneRunner struct {
	ProcessRunner LaneProcessRunner
	mu            sync.Mutex
	processes     map[int]LaneProcess
}

func BuildLaneCommand(spec LaneSpec) []string {
	contract := "You are a non-interactive, read-only QRSPI review lane. Do not start agents, use QRSPI commands, create results, notify supervisors, or modify files except the assigned report. Write a substantive Markdown report only to: " + spec.ReportPath
	script := `set -o pipefail
: > "$OUTPUT_PATH"
{ pi --print --no-extensions --session-id "$SESSION_ID" --session-dir "$SESSION_DIR" --name "$SESSION_NAME" -- "$(cat "$PROMPT_FILE")\n\n$LANE_CONTRACT"; } > "$OUTPUT_PATH" 2>&1`
	return []string{
		"env",
		"PROMPT_FILE=" + spec.PromptFile,
		"OUTPUT_PATH=" + LaneDiagnosticOutputPath(spec),
		"SESSION_ID=" + spec.SessionID,
		"SESSION_DIR=" + spec.SessionDir,
		"SESSION_NAME=" + string(spec.Role),
		"LANE_CONTRACT=" + contract,
		"bash", "-lc", script,
	}
}

func (r *DetachedLaneRunner) Start(
	ctx context.Context,
	spec LaneSpec,
) (LaneRecord, error) {
	if err := ValidateLaneSpec(spec); err != nil {
		return LaneRecord{}, err
	}
	if r.ProcessRunner == nil {
		r.ProcessRunner = ExecLaneProcessRunner{}
	}
	if err := os.MkdirAll(LaneAttemptDir(spec), 0o755); err != nil {
		return LaneRecord{}, err
	}
	if err := os.MkdirAll(
		filepath.Dir(LaneDiagnosticOutputPath(spec)),
		0o755,
	); err != nil {
		return LaneRecord{}, err
	}
	record := LaneRecord{
		SchemaVersion: laneSchemaVersion,
		Spec:          spec,
		State:         LaneQueued,
		StatusPath:    LaneStatusPath(spec),
		EventsPath:    LaneEventsPath(spec),
		OutputPath:    LaneDiagnosticOutputPath(spec),
	}
	if err := WriteLaneRecord(LaneRecordPath(spec), record); err != nil {
		return LaneRecord{}, err
	}
	process, err := r.ProcessRunner.Start(ctx, BuildLaneCommand(spec), spec.Cwd)
	if err != nil {
		record.State, record.FinishedAt, record.ErrorTail = LaneFailed, time.Now().
			UTC(),
			boundedLaneDiagnostic(
				err.Error(),
			)
		return record, WriteLaneRecord(LaneRecordPath(spec), record)
	}
	record.State, record.PID, record.StartedAt = LaneRunning, process.PID(), time.Now().
		UTC()
	if err := WriteLaneRecord(LaneRecordPath(spec), record); err != nil {
		return LaneRecord{}, err
	}
	r.mu.Lock()
	if r.processes == nil {
		r.processes = make(map[int]LaneProcess)
	}
	r.processes[record.PID] = process
	r.mu.Unlock()
	return record, nil
}

func (r *DetachedLaneRunner) Wait(
	ctx context.Context,
	record LaneRecord,
) (LaneRecord, error) {
	if record.State != LaneRunning {
		return record, fmt.Errorf("lane %q is not running", record.Spec.ID)
	}
	r.mu.Lock()
	process := r.processes[record.PID]
	r.mu.Unlock()
	if process == nil {
		return record, fmt.Errorf("lane process %d is unavailable", record.PID)
	}
	waitCtx, cancel := context.WithTimeout(ctx, record.Spec.Timeout)
	exit, err := process.Wait(waitCtx)
	waitErr := waitCtx.Err()
	cancel()
	if waitErr != nil {
		terminationErr := process.Terminate(context.Background(), laneTerminationGrace)
		reapCtx, reapCancel := context.WithTimeout(context.Background(), laneReapDeadline)
		exit, err = process.Wait(reapCtx)
		reapCancel()
		err = errors.Join(waitErr, terminationErr, err)
	}
	record.FinishedAt = time.Now().UTC()
	record.ErrorTail = laneErrorTail(exit, err)
	switch {
	case err != nil:
		record.State = LaneFailed
	case exit.ExitCode != 0:
		record.State = LaneFailed
		err = fmt.Errorf("lane %q exited with status %d", record.Spec.ID, exit.ExitCode)
		record.ErrorTail = laneErrorTail(exit, err)
	default:
		if reportErr := ValidateLaneReport(record.Spec); reportErr != nil {
			err = reportErr
			record.State = LaneFailed
			record.ErrorTail = laneErrorTail(exit, err)
		} else {
			record.State = LaneSuccess
		}
	}
	if record.State == LaneSuccess {
		record.ErrorTail = ""
	}
	writeErr := WriteLaneRecord(LaneRecordPath(record.Spec), record)
	r.mu.Lock()
	delete(r.processes, record.PID)
	r.mu.Unlock()
	if writeErr != nil {
		return record, writeErr
	}
	return record, err
}

type execLaneProcess struct {
	cmd      *exec.Cmd
	waitOnce sync.Once
	done     chan laneWaitResult
}

type laneWaitResult struct {
	exit LaneExit
	err  error
}

func (p *execLaneProcess) PID() int { return p.cmd.Process.Pid }

func (p *execLaneProcess) Wait(ctx context.Context) (LaneExit, error) {
	p.waitOnce.Do(func() {
		go func() {
			err := p.cmd.Wait()
			if err == nil {
				p.done <- laneWaitResult{}
				return
			}
			if exit, ok := err.(*exec.ExitError); ok {
				p.done <- laneWaitResult{exit: LaneExit{ExitCode: exit.ExitCode()}}
				return
			}
			p.done <- laneWaitResult{err: err}
		}()
	})
	select {
	case result := <-p.done:
		p.done <- result
		return result.exit, result.err
	case <-ctx.Done():
		return LaneExit{}, ctx.Err()
	}
}

func (p *execLaneProcess) Terminate(ctx context.Context, grace time.Duration) error {
	return terminateLaneProcess(ctx, p, grace)
}

type ExecLaneProcessRunner struct{}

func (ExecLaneProcessRunner) Start(
	ctx context.Context,
	command []string,
	cwd string,
) (LaneProcess, error) {
	if len(command) == 0 {
		return nil, errors.New("lane command is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return startLaneProcess(command, cwd)
}

func laneErrorTail(exit LaneExit, err error) string {
	parts := []string{exit.ErrorTail}
	if err != nil {
		parts = append(parts, err.Error())
	}
	return boundedLaneDiagnostic(strings.Join(parts, "\n"))
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

// LaneDiagnosticOutputPath stores complete lane stdout/stderr separately from
// lane records and canonical review reports.
func LaneDiagnosticOutputPath(spec LaneSpec) string {
	return filepath.Join(
		spec.PlanDir,
		".vamos",
		"sessions",
		"pi",
		"subagents",
		spec.CoordinatorID,
		spec.ID,
		fmt.Sprintf("attempt-%d", spec.Attempt),
		"output.txt",
	)
}

func LaneFailureLogPath(planDir, coordinatorID string) string {
	return filepath.Join(
		planDir,
		".vamos",
		"qrspi",
		"lanes",
		coordinatorID,
		"failures.jsonl",
	)
}

type LaneFailureEvent struct {
	LaneID     string    `json:"laneId"`
	Attempt    int       `json:"attempt"`
	FailedAt   time.Time `json:"failedAt"`
	RecordPath string    `json:"recordPath"`
	OutputPath string    `json:"outputPath"`
	ErrorTail  string    `json:"errorTail"`
}

func AppendLaneFailure(path string, event LaneFailureEvent) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
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
	if spec.Attempt != 1 {
		return errors.New("lane attempt must be 1")
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
	if err := requireContained(spec.PlanDir, LaneDiagnosticOutputPath(spec)); err != nil {
		return fmt.Errorf("lane diagnostics: %w", err)
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
	if current, err := ReadLaneRecord(path); err == nil {
		if terminalLaneState(current.State) {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	record.ErrorTail = boundedLaneDiagnostic(record.ErrorTail)
	return writeJSONAtomically(path, record)
}

// InspectLaneRecord reads a diagnostic record only after proving it is a regular
// file within this plan's lane metadata tree. It has no transition authority.
func InspectLaneRecord(planDir, path string) (LaneRecord, error) {
	if !filepath.IsAbs(planDir) || !filepath.IsAbs(path) {
		return LaneRecord{}, errors.New("plan directory and record path must be absolute")
	}
	lanesRoot := filepath.Join(planDir, ".vamos", "qrspi", "lanes")
	if err := requireContained(lanesRoot, path); err != nil {
		return LaneRecord{}, fmt.Errorf("lane record path: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return LaneRecord{}, err
	}
	if !info.Mode().IsRegular() {
		return LaneRecord{}, errors.New("lane record must be a regular file")
	}
	record, err := ReadLaneRecord(path)
	if err != nil {
		return LaneRecord{}, err
	}
	if expected := LaneRecordPath(
		record.Spec,
	); filepath.Clean(
		path,
	) != filepath.Clean(
		expected,
	) {
		return LaneRecord{}, errors.New(
			"lane record path does not match its specification",
		)
	}
	if err := requireContained(planDir, LaneRecordPath(record.Spec)); err != nil {
		return LaneRecord{}, fmt.Errorf("lane record: %w", err)
	}
	return record, nil
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

// Run starts a validated set of parent-owned lanes with a bounded worker pool.
// Lane failures are durable unavailable evidence; independent peers continue.
func (c LaneCoordinator) Run(
	ctx context.Context,
	specs []LaneSpec,
) ([]LaneRecord, error) {
	if c.Runner == nil {
		return nil, errors.New("lane runner is required")
	}
	if err := ValidateLaneSpecs(specs); err != nil {
		return nil, err
	}
	if c.MaxParallel < 1 {
		return nil, errors.New("maximum lane parallelism must be positive")
	}
	if len(specs) == 0 {
		return nil, nil
	}
	type job struct {
		index int
		spec  LaneSpec
	}
	type completion struct {
		index  int
		spec   LaneSpec
		record LaneRecord
		err    error
	}
	jobs := make(chan job)
	completed := make(chan completion, c.MaxParallel)
	workers := c.MaxParallel
	if workers > len(specs) {
		workers = len(specs)
	}
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				record, err := c.Runner.Start(ctx, job.spec)
				if err == nil {
					record, err = c.Runner.Wait(ctx, record)
				}
				if err == nil && record.State == LaneSuccess {
					if err = ValidateLaneReport(job.spec); err != nil {
						record.State = LaneFailed
						record.FinishedAt = time.Now().UTC()
						record.ErrorTail = laneErrorTail(LaneExit{}, err)
					}
				}
				completed <- completion{job.index, job.spec, record, err}
			}
		}()
	}
	defer func() { close(jobs); wg.Wait() }()

	records := make([]LaneRecord, len(specs))
	next, active, finished := 0, 0, 0
	for finished < len(specs) {
		for active < workers && next < len(specs) {
			jobs <- job{next, specs[next]}
			next++
			active++
		}
		result := <-completed
		active--
		if result.err != nil || result.record.State != LaneSuccess {
			if result.record.Spec.ID == "" {
				return records, fmt.Errorf(
					"lane %q failed before recording: %w",
					result.spec.ID,
					result.err,
				)
			}
			if result.record.State != LaneFailed {
				result.record.State = LaneFailed
			}
			if result.record.FinishedAt.IsZero() {
				result.record.FinishedAt = time.Now().UTC()
			}
			if result.record.ErrorTail == "" {
				result.record.ErrorTail = laneErrorTail(LaneExit{}, result.err)
			}
			if err := AppendLaneFailure(
				LaneFailureLogPath(result.spec.PlanDir, result.spec.CoordinatorID),
				LaneFailureEvent{
					LaneID:     result.spec.ID,
					Attempt:    result.spec.Attempt,
					FailedAt:   result.record.FinishedAt,
					RecordPath: LaneRecordPath(result.spec),
					OutputPath: LaneDiagnosticOutputPath(result.spec),
					ErrorTail:  boundedLaneDiagnostic(result.record.ErrorTail),
				},
			); err != nil {
				return records, fmt.Errorf("append lane failure: %w", err)
			}
		}
		records[result.index] = result.record
		finished++
	}
	return records, nil
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
