package qrspicmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func RunRecoverSummary(
	ctx context.Context,
	opts RecoverSummaryOptions,
	d deps,
	out io.Writer,
) error {
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	if strings.TrimSpace(opts.SessionFile) == "" {
		return errors.New("session-file is required")
	}
	out = ensureWriter(out)
	state, err := stateStore(d, "", time.Now).Load(opts.StateFile)
	if err != nil {
		return err
	}
	now := time.Now()
	if d.Clock != nil {
		now = d.Clock()
	}
	stage := strings.TrimSpace(opts.Stage)
	if stage == "" && state.ActiveChild != nil {
		stage = strings.TrimSpace(state.ActiveChild.Stage)
	}
	if stage == "" {
		stage = strings.TrimSpace(string(state.Workflow.CurrentNodeID))
	}
	if stage == "" {
		stage = "unknown"
	}
	childID := "unknown-child"
	if state.ActiveChild != nil && strings.TrimSpace(state.ActiveChild.ID) != "" {
		childID = strings.TrimSpace(state.ActiveChild.ID)
	}
	planDir := resolvedRecoveryPlanDir(state)
	if strings.TrimSpace(planDir) == "" {
		return errors.New("plan directory is required in manager state")
	}
	evidence, _, err := LatestAssistantTerminalEvidence(opts.SessionFile)
	if err != nil {
		return err
	}
	notePath := RecoverySummaryPath(planDir, stage, childID, now)
	promptPath := filepath.Join(
		filepath.Dir(opts.StateFile),
		"prompts",
		fmt.Sprintf("recover-summary-%s.md", timestampForRecoveryFile(now)),
	)
	req := RecoverySummaryRequest{
		StateFile:         opts.StateFile,
		PlanDir:           planDir,
		ImplementationCwd: state.ImplementationCwd,
		Stage:             stage,
		ChildID:           childID,
		SessionFile:       opts.SessionFile,
		Evidence:          evidence,
		LatestArtifact:    latestRecoveryArtifact(state),
		PromptPath:        promptPath,
		NotePath:          notePath,
	}
	if err := WriteRecoverySummaryPrompt(req, promptPath); err != nil {
		return err
	}
	if opts.DryRun {
		if err := writeDryRunRecoveryNote(req, notePath); err != nil {
			return err
		}
	} else {
		binary := strings.TrimSpace(opts.PiBinary)
		if binary == "" {
			binary = "pi"
		}
		result, err := commandRunner(d).Run(ctx, binary, "@"+promptPath)
		if err != nil {
			return fmt.Errorf(
				"run recovery summarizer %q failed: %w; prompt written at %s; rerun with --dry-run to create a placeholder note",
				binary,
				err,
				promptPath,
			)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf(
				"run recovery summarizer %q exited %d: %s; prompt written at %s",
				binary,
				result.ExitCode,
				strings.TrimSpace(result.Stderr),
				promptPath,
			)
		}
		if err := ensureRecoveryNoteWritten(notePath, promptPath, binary); err != nil {
			return err
		}
	}
	if strings.EqualFold(opts.Output, "json") ||
		strings.EqualFold(opts.Output, "ndjson") {
		return json.NewEncoder(out).Encode(req)
	}
	fmt.Fprintf(out, "recovery prompt: %s\n", promptPath)
	fmt.Fprintf(out, "recovery note: %s\n", notePath)
	fmt.Fprintf(out, "stage: %s\n", stage)
	fmt.Fprintf(out, "session: %s\n", opts.SessionFile)
	if evidence.EvidenceID != "" {
		fmt.Fprintf(out, "evidence id: %s\n", evidence.EvidenceID)
	}
	return nil
}

func ensureRecoveryNoteWritten(notePath, promptPath, binary string) error {
	if _, err := os.Stat(notePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return fmt.Errorf(
		"run recovery summarizer %q did not write recovery note %s; prompt written at %s",
		binary,
		notePath,
		promptPath,
	)
}

func WriteRecoverySummaryPrompt(req RecoverySummaryRequest, promptPath string) error {
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# q-manager recovery summarizer\n\n")
	b.WriteString(
		"You are a read-only recovery summarizer. Do not emit qrspi_result. Do not advance graph. Do not edit code.\n\n",
	)
	b.WriteString("Read:\n")
	b.WriteString("- Failed session: " + req.SessionFile + "\n")
	b.WriteString("- Plan memory: " + filepath.Join(req.PlanDir, "AGENTS.md") + "\n")
	b.WriteString("- Current stage artifacts as needed.\n")
	if strings.TrimSpace(req.LatestArtifact) != "" {
		b.WriteString("- Latest known artifact: " + req.LatestArtifact + "\n")
	}
	b.WriteString("\nWrite exactly: " + req.NotePath + "\n\n")
	b.WriteString("Include:\n")
	b.WriteString("- Last reliable completed work.\n")
	b.WriteString("- Terminal provider error evidence.\n")
	b.WriteString("- Same-stage relaunch instructions.\n")
	b.WriteString("- Commands/artifacts already checked.\n")
	b.WriteString("- Avoid repeating huge outputs blindly.\n\n")
	b.WriteString("Context:\n")
	b.WriteString("- State file: " + req.StateFile + "\n")
	b.WriteString("- Plan dir: " + req.PlanDir + "\n")
	if strings.TrimSpace(req.ImplementationCwd) != "" {
		b.WriteString("- Implementation cwd: " + req.ImplementationCwd + "\n")
	}
	b.WriteString("- Stage: " + req.Stage + "\n")
	b.WriteString("- Child: " + req.ChildID + "\n")
	if req.Evidence.EvidenceID != "" {
		b.WriteString("- Evidence ID: " + req.Evidence.EvidenceID + "\n")
	}
	if req.Evidence.SessionID != "" {
		b.WriteString("- Session ID: " + req.Evidence.SessionID + "\n")
	}
	if req.Evidence.Line > 0 {
		b.WriteString(fmt.Sprintf("- Evidence line: %d\n", req.Evidence.Line))
	}
	if req.Evidence.StopReason != "" {
		b.WriteString("- Stop reason: " + req.Evidence.StopReason + "\n")
	}
	if req.Evidence.ErrorMessage != "" {
		b.WriteString("- Provider error: " + req.Evidence.ErrorMessage + "\n")
	}
	if req.Evidence.ContextWindowError {
		b.WriteString("- Context-window error: true\n")
	}
	return os.WriteFile(promptPath, []byte(b.String()), 0o644)
}

func RecoverySummaryPath(planDir, stage, childID string, now time.Time) string {
	return filepath.Join(
		planDir,
		"context",
		"recovery",
		fmt.Sprintf(
			"%s_%s_%s_context-recovery.md",
			timestampForRecoveryPath(now),
			slugForRecoveryPath(stage),
			slugForRecoveryPath(childID),
		),
	)
}

func writeDryRunRecoveryNote(req RecoverySummaryRequest, notePath string) error {
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("stage: recovery-summary\n")
	b.WriteString("source: recover-summary --dry-run\n")
	b.WriteString("---\n\n")
	b.WriteString("# q-manager child context recovery note\n\n")
	b.WriteString(
		"Same-stage relaunch only. Do not emit qrspi_result. Do not advance graph. Do not edit code from this helper.\n\n",
	)
	b.WriteString("- Stage: " + req.Stage + "\n")
	b.WriteString("- Child: " + req.ChildID + "\n")
	b.WriteString("- Failed session: " + req.SessionFile + "\n")
	if req.Evidence.EvidenceID != "" {
		b.WriteString("- Evidence ID: " + req.Evidence.EvidenceID + "\n")
	}
	if req.Evidence.ErrorMessage != "" {
		b.WriteString("- Provider error: " + req.Evidence.ErrorMessage + "\n")
	}
	if req.LatestArtifact != "" {
		b.WriteString("- Latest known artifact: " + req.LatestArtifact + "\n")
	}
	b.WriteString(
		"\nNext child prompt: read this note, inspect the failed session tail and named artifacts, then relaunch the same graph node with focused context. Avoid repeating huge outputs blindly.\n",
	)
	return os.WriteFile(notePath, []byte(b.String()), 0o644)
}

func resolvedRecoveryPlanDir(state ManagerState) string {
	planDir := strings.TrimSpace(state.CanonicalPlanDir)
	if planDir == "" {
		return ""
	}
	if filepath.IsAbs(planDir) {
		return filepath.Clean(planDir)
	}
	for _, base := range []string{state.SourceCwd, state.ImplementationCwd} {
		if strings.TrimSpace(base) != "" {
			return filepath.Join(base, planDir)
		}
	}
	return filepath.Clean(planDir)
}

func latestRecoveryArtifact(state ManagerState) string {
	if state.ActiveChild != nil {
		if status := readPriorValidationStatus(
			state.ActiveChild.ValidationStatusPath,
		); status != nil &&
			strings.TrimSpace(status.Result.Artifact) != "" {
			return strings.TrimSpace(status.Result.Artifact)
		}
	}
	if state.LastActionCard != nil {
		for _, line := range state.LastActionCard.Evidence {
			if value, ok := strings.CutPrefix(strings.TrimSpace(line), "artifact: "); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func timestampForRecoveryPath(t time.Time) string {
	return t.Format("2006-01-02_15-04-05")
}

func timestampForRecoveryFile(t time.Time) string {
	return t.UTC().Format("20060102T150405.000000000Z")
}

var recoverySlugPattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func slugForRecoveryPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	value = recoverySlugPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-._")
	if value == "" {
		return "unknown"
	}
	return strings.ToLower(value)
}
