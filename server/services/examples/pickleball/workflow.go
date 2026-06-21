package pickleball

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/generatedgo"
	temporalmgr "github.com/CoreyCole/vamos/pkg/agents/temporal"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	WorkflowPickleballSelfModify    = "pickleball-self-modify"
	ActivityPickleballRunAIEdits    = "RunAIEdits"
	ActivityPickleballBuildSnapshot = "BuildAndSnapshot"
	generatedBuildIDTimestampLayout = "20060102-150405"
	maxPromptSummaryLength          = 96
)

type AIGenerateInput struct {
	SessionID     string
	Prompt        string
	WorkspacePath string
	History       []BuildSnapshot
}

type AIGenerator interface {
	ApplyPrompt(ctx context.Context, input AIGenerateInput) error
}

type SelfModifyWorkflowInput struct {
	SessionID string
	Prompt    string
	UserEmail string
}

type SelfModifyActivities struct {
	Service *Service
}

type temporalWorkflowStarter struct {
	client client.Client
}

type generatedRunner struct{}

func NewTemporalWorkflowStarter(c client.Client) WorkflowStarter {
	return temporalWorkflowStarter{client: c}
}

func (s temporalWorkflowStarter) StartPickleballSelfModify(ctx context.Context, req PromptRequest) (string, error) {
	if s.client == nil {
		return "", fmt.Errorf("temporal client is required")
	}
	now := time.Now().UTC()
	runID := fmt.Sprintf("%s-%d", now.Format(generatedBuildIDTimestampLayout), now.UnixNano())
	workflowID := fmt.Sprintf("%s:%s:%s", WorkflowPickleballSelfModify, cleanWorkflowIDPart(req.SessionID), runID)
	run, err := s.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:                    workflowID,
		TaskQueue:             temporalmgr.GoTaskQueue,
		WorkflowIDReusePolicy: enums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE_FAILED_ONLY,
	}, SelfModifyWorkflow, SelfModifyWorkflowInput{SessionID: req.SessionID, Prompt: req.Prompt, UserEmail: req.UserEmail})
	if err != nil {
		var alreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &alreadyStarted) {
			return "", nil
		}
		return "", fmt.Errorf("start pickleball workflow: %w", err)
	}
	return run.GetRunID(), nil
}

func SelfModifyWorkflow(ctx workflow.Context, input SelfModifyWorkflowInput) error {
	editCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Minute,
		HeartbeatTimeout:    time.Minute,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 1},
	})
	if err := workflow.ExecuteActivity(editCtx, ActivityPickleballRunAIEdits, input).Get(editCtx, nil); err != nil {
		return err
	}

	buildCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 1},
	})
	return workflow.ExecuteActivity(buildCtx, ActivityPickleballBuildSnapshot, input).Get(buildCtx, nil)
}

func (a *SelfModifyActivities) RunAIEdits(ctx context.Context, input SelfModifyWorkflowInput) error {
	if a == nil || a.Service == nil {
		return fmt.Errorf("pickleball service is required")
	}
	session, err := a.Service.store.LoadSession(ctx, input.SessionID)
	if err != nil {
		return err
	}
	if err := SeedOrUpdateGeneratedWorkspace(ctx, session, a.Service.opts.SeedBundleDir); err != nil {
		return err
	}
	history, err := a.Service.SnapshotHistoryForPrompt(ctx, session.ID)
	if err != nil {
		return err
	}
	generator := a.Service.opts.AIGenerator
	if generator == nil {
		generator = PromptPatchGenerator{}
	}
	return generator.ApplyPrompt(ctx, AIGenerateInput{SessionID: session.ID, Prompt: input.Prompt, WorkspacePath: session.WorkspacePath, History: history})
}

func (a *SelfModifyActivities) BuildAndSnapshot(ctx context.Context, input SelfModifyWorkflowInput) error {
	if a == nil || a.Service == nil {
		return fmt.Errorf("pickleball service is required")
	}
	svc := a.Service
	session, err := svc.markBuilding(ctx, input.SessionID)
	if err != nil {
		return err
	}
	buildID := newBuildID(input.Prompt)
	outputDir, err := os.MkdirTemp("", "vamos-pickleball-output-*")
	if err != nil {
		_ = svc.MarkFailed(ctx, session.ID, err, "")
		return fmt.Errorf("create generated output dir: %w", err)
	}
	defer os.RemoveAll(outputDir)

	runner := svc.runner
	if runner == nil {
		runner = generatedRunner{}
	}
	result, err := runner.BuildAndRun(ctx, BuildOneShotRunnerInput(session, buildID, outputDir))
	if err != nil {
		_ = svc.MarkFailed(ctx, session.ID, err, result.StdoutTail+result.StderrTail)
		return err
	}

	snapshotRel := pathJoinSlash(svc.opts.ExampleRoot, "sessions", session.ID, "snapshots", buildID)
	snapshotAbs := filepath.Join(svc.store.Root(), "sessions", session.ID, "snapshots", buildID)
	snapshotResult, err := generatedgo.CopySnapshot(generatedgo.SnapshotInput{
		SourceDir:   session.WorkspacePath,
		OutputDir:   outputDir,
		SnapshotDir: snapshotAbs,
		Allowlist:   []string{"app.html", "results.csv", "manifest.json"},
	})
	if err != nil {
		_ = svc.MarkFailed(ctx, session.ID, err, result.StdoutTail+result.StderrTail)
		return err
	}
	result.SourceHash = snapshotResult.SourceHash
	for key, hash := range snapshotResult.ArtifactHashes {
		result.ArtifactHashes[key] = hash
	}
	snapshot := BuildSnapshotFromRunner(session, result, snapshotRel)
	if err := svc.PromoteSnapshot(ctx, session.ID, snapshot); err != nil {
		_ = svc.MarkFailed(ctx, session.ID, err, result.StdoutTail+result.StderrTail)
		return err
	}
	return nil
}

func BuildAIPrompt(req PromptRequest, history []BuildSnapshot) string {
	var b strings.Builder
	b.WriteString("You are editing a generated Go bundle for the Vamos pickleball self-modifying app example.\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Edit only the generated bundle workspace.\n")
	b.WriteString("- Preserve one-shot behavior; do not start a server or use the network.\n")
	b.WriteString("- Write app.html, results.csv, and manifest.json to VAMOS_GENERATED_OUTPUT_DIR.\n")
	b.WriteString("- Keep generated HTML iframe-safe and mobile-friendly.\n")
	if len(history) > 0 {
		b.WriteString("\nRecent successful builds:\n")
		for i, snapshot := range history {
			if i >= 5 {
				break
			}
			fmt.Fprintf(&b, "- %s: %s (%s)\n", snapshot.BuildID, snapshot.PromptSummary, snapshot.HTMLThoughtsPath)
		}
	}
	b.WriteString("\nUser prompt:\n")
	b.WriteString(strings.TrimSpace(req.Prompt))
	b.WriteByte('\n')
	return b.String()
}

func SeedOrUpdateGeneratedWorkspace(ctx context.Context, session PickleballSession, seedDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(seedDir) == "" {
		seedDir = defaultSeedDir
	}
	if strings.TrimSpace(session.WorkspacePath) == "" {
		return fmt.Errorf("session workspace path is required")
	}
	return copyDirIfMissing(seedDir, session.WorkspacePath)
}

func BuildOneShotRunnerInput(session PickleballSession, buildID string, outputDir string) generatedgo.RunnerInput {
	return generatedgo.RunnerInput{
		WorkspaceDir:      session.WorkspacePath,
		OutputDir:         outputDir,
		ModulePath:        ".",
		CompileTimeout:    30 * time.Second,
		RunTimeout:        30 * time.Second,
		ArtifactAllowlist: []string{"app.html", "results.csv", "manifest.json"},
		EnvAllowlist: map[string]string{
			"VAMOS_GENERATED_BUILD_ID": buildID,
			"VAMOS_PARENT_BUILD_ID":    session.CurrentBuildID,
		},
	}
}

func BuildSnapshotFromRunner(session PickleballSession, result generatedgo.RunnerResult, snapshotPath string) BuildSnapshot {
	snapshotPath = cleanRelativePath(snapshotPath)
	htmlPath := pathJoinSlash(snapshotPath, result.Manifest.Artifacts.HTML)
	csvPath := pathJoinSlash(snapshotPath, result.Manifest.Artifacts.CSV)
	return BuildSnapshot{
		BuildID:          result.Manifest.BuildID,
		ParentBuildID:    result.Manifest.ParentBuildID,
		PromptSummary:    result.Manifest.PromptSummary,
		Mode:             string(result.Manifest.Mode),
		Status:           string(result.Status),
		SnapshotPath:     snapshotPath,
		ManifestPath:     pathJoinSlash(snapshotPath, "manifest.json"),
		HTMLThoughtsPath: htmlPath,
		CSVThoughtsPath:  csvPath,
		SourceHash:       result.SourceHash,
		HTMLHash:         result.ArtifactHashes["app.html"],
		CSVHash:          result.ArtifactHashes["results.csv"],
		CreatedAt:        time.Now().UTC(),
	}
}

func (g generatedRunner) BuildAndRun(ctx context.Context, input generatedgo.RunnerInput) (generatedgo.RunnerResult, error) {
	return generatedgo.BuildAndRun(ctx, input)
}

func (s *Service) markBuilding(ctx context.Context, sessionID string) (PickleballSession, error) {
	session, err := s.store.LoadSession(ctx, sessionID)
	if err != nil {
		return PickleballSession{}, err
	}
	session.State = AppStateBuilding
	session.ErrorMessage = ""
	session.LogTail = ""
	if err := s.store.SaveSession(ctx, session); err != nil {
		return PickleballSession{}, err
	}
	s.notify(session.ID)
	return session, nil
}

func pathJoinSlash(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(cleanRelativePath(part), "/")
		if part != "" {
			clean = append(clean, part)
		}
	}
	return filepath.ToSlash(filepath.Join(clean...))
}

func newBuildID(prompt string) string {
	slug := strings.ToLower(regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(strings.TrimSpace(prompt), "-"))
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "build"
	}
	if len(slug) > 32 {
		slug = strings.Trim(slug[:32], "-")
	}
	return fmt.Sprintf("%s_%s", time.Now().UTC().Format(generatedBuildIDTimestampLayout), slug)
}

func cleanWorkflowIDPart(value string) string {
	value = regexp.MustCompile(`[^a-zA-Z0-9_-]+`).ReplaceAllString(strings.TrimSpace(value), "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "default"
	}
	return value
}

func promptSummary(prompt string) string {
	prompt = strings.Join(strings.Fields(prompt), " ")
	if prompt == "" {
		return "Prompted pickleball app update"
	}
	if len(prompt) > maxPromptSummaryLength {
		return strings.TrimSpace(prompt[:maxPromptSummaryLength-1]) + "…"
	}
	return prompt
}

// PromptPatchGenerator is the built-in local edit adapter for the reusable demo.
// Hosts can replace it with an Agent Chat/Pi-backed generator through Options.
type PromptPatchGenerator struct{}

func (PromptPatchGenerator) ApplyPrompt(ctx context.Context, input AIGenerateInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	mainPath := filepath.Join(input.WorkspacePath, "main.go")
	data, err := os.ReadFile(mainPath)
	if err != nil {
		return fmt.Errorf("read generated bundle: %w", err)
	}
	source := string(data)
	source = replaceFirstStringField(source, "PromptSummary", promptSummary(input.Prompt))
	source = replaceFirstStringField(source, "Reason", reasonForPrompt(input.Prompt))
	if strings.Contains(strings.ToLower(input.Prompt), "color") {
		source = strings.ReplaceAll(source, "#0f766e", "#7c3aed")
		source = strings.ReplaceAll(source, "#14b8a6", "#f97316")
	}
	if err := os.WriteFile(mainPath, []byte(source), 0o644); err != nil {
		return fmt.Errorf("write generated bundle: %w", err)
	}
	return nil
}

func replaceFirstStringField(source, field, value string) string {
	re := regexp.MustCompile(`(` + regexp.QuoteMeta(field) + `\s*:\s*)"[^"\\]*(?:\\.[^"\\]*)*"(\s*,?)`)
	loc := re.FindStringSubmatchIndex(source)
	if loc == nil {
		return source
	}
	return source[:loc[0]] + source[loc[2]:loc[3]] + fmt.Sprintf("%q", value) + source[loc[4]:loc[5]] + source[loc[1]:]
}

func reasonForPrompt(prompt string) string {
	lower := strings.ToLower(prompt)
	switch {
	case strings.Contains(lower, "partner"):
		return "Prompt update: prefer fresh partner pairings while keeping games close."
	case strings.Contains(lower, "skill"):
		return "Prompt update: show skill totals clearly while balancing each court."
	case strings.Contains(lower, "csv"):
		return "Prompt update: keep CSV-friendly matchup explanations for review."
	default:
		return "Prompt update: generated bundle changed from the latest user request."
	}
}
