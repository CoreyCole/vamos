package qrspicmd

import (
	"context"
	"fmt"
	"os"
	"strings"
)

func InspectActiveChildHealth(
	ctx context.Context,
	state ManagerState,
	stateFile string,
	d deps,
) (ActiveChildHealth, error) {
	if state.ActiveChild == nil {
		return ActiveChildHealth{
			Status:   ActiveChildUnknown,
			Evidence: []string{"no active child"},
		}, nil
	}
	child := state.ActiveChild
	health := ActiveChildHealth{
		Status:      ActiveChildUnknown,
		ChildID:     child.ID,
		Stage:       child.Stage,
		PaneID:      child.TmuxPaneID,
		OutputPath:  child.OutputPath,
		StatusPath:  child.StatusPath,
		DonePath:    child.DonePath,
		SessionDir:  child.SessionDir,
		SessionPath: child.SessionPath,
		SafeCommand: fmt.Sprintf(
			"vamos qrspi repair-state --state-file %s --clear-failed-child --relaunch",
			stateFile,
		),
	}

	status, _ := ReadChildStatus(child.StatusPath)
	if status != nil {
		health.ExitCode = &status.ExitCode
		health.Evidence = append(
			health.Evidence,
			fmt.Sprintf("exitCode: %d", status.ExitCode),
		)
	}
	if data, err := os.ReadFile(child.OutputPath); err == nil {
		health.OutputTail = FilterChildOutputTail(data, 8)
	}

	if strings.TrimSpace(health.SessionPath) == "" {
		if resolved, err := resolveActiveChildSessionPath(child); err == nil {
			health.SessionPath = resolved
		}
	}
	if strings.TrimSpace(health.SessionPath) != "" {
		if evidence, ok, err := LatestAssistantTerminalEvidence(
			health.SessionPath,
		); err == nil && ok &&
			evidence.ContextWindowError {
			health.Status = ActiveChildProviderContextError
			health.TerminalEvidence = &evidence
			health.Evidence = append(
				health.Evidence,
				providerContextEvidenceLines(evidence)...)
			health.SafeCommand = providerContextRecoverySafeCommand(stateFile)
			return health, nil
		}
	}

	hasResult, sessionPath, resultErr := ChildHasQRSPIResult(state)
	if hasResult {
		health.Status = ActiveChildFinishedNeedsValidation
		health.SessionPath = sessionPath
		health.Evidence = append(health.Evidence, "session has qrspi_result")
		return health, nil
	}
	if strings.TrimSpace(sessionPath) != "" {
		health.SessionPath = sessionPath
	}
	sessionText := ""
	if strings.TrimSpace(health.SessionPath) != "" {
		if text, err := ExtractLastAssistantTextFromSession(
			health.SessionPath,
		); err == nil {
			sessionText = text
		}
	}

	paneOK := false
	if strings.TrimSpace(child.TmuxPaneID) != "" {
		ok, err := tmuxClient(d).PaneExists(ctx, TmuxPane{ID: child.TmuxPaneID})
		paneOK = ok && err == nil
		if !paneOK {
			health.Evidence = append(health.Evidence, "pane missing")
		}
	}
	if status != nil && HasDoneMarker(child.DonePath) && !hasResult &&
		HasChildContextExhaustionEvidence(health, sessionText) {
		health.Status = ActiveChildContextExhausted
		if resultErr != nil {
			health.Evidence = append(health.Evidence, resultErr.Error())
		}
		if strings.TrimSpace(sessionText) != "" {
			health.Evidence = append(
				health.Evidence,
				"session has context-limit/no-result evidence",
			)
		}
		health.SafeCommand = fmt.Sprintf(
			"pi --resume %s # then run /compact only if this is the exhausted child session",
			firstNonEmpty(health.SessionPath, child.SessionID),
		)
		return health, nil
	}
	if status != nil && status.ExitCode != 0 && HasDoneMarker(child.DonePath) &&
		!hasResult {
		health.Status = ActiveChildLaunchFailed
		if resultErr != nil {
			health.Evidence = append(health.Evidence, resultErr.Error())
		}
		return health, nil
	}
	if status != nil && status.ExitCode == 0 && HasDoneMarker(child.DonePath) {
		health.Status = ActiveChildFinishedNeedsValidation
		return health, nil
	}
	if !paneOK && strings.TrimSpace(child.TmuxPaneID) != "" {
		health.Status = ActiveChildPaneMissing
		return health, nil
	}
	health.Status = ActiveChildRunning
	health.Evidence = append(
		health.Evidence,
		"pane exists and no terminal failure status",
	)
	return health, nil
}

func HasDoneMarker(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func LatestTerminalEvidenceForActiveChild(
	state ManagerState,
) (AssistantTerminalEvidence, bool, error) {
	if state.ActiveChild == nil {
		return AssistantTerminalEvidence{}, false, nil
	}
	path, err := resolveActiveChildSessionPath(state.ActiveChild)
	if err != nil {
		return AssistantTerminalEvidence{}, false, err
	}
	return LatestAssistantTerminalEvidence(path)
}

func ChildHasQRSPIResult(state ManagerState) (bool, string, error) {
	text, parseCtx, err := ReadChildResultText(state, ResultSourceOptions{})
	if err != nil {
		return false, "", err
	}
	return strings.Contains(text, "qrspi_result"), parseCtx.SessionPath, nil
}

func HasChildContextExhaustionEvidence(
	health ActiveChildHealth,
	sessionText string,
) bool {
	lines := append([]string{}, health.Evidence...)
	lines = append(lines, health.OutputTail...)
	if strings.TrimSpace(sessionText) != "" {
		lines = append(lines, sessionText)
	}
	needles := []string{
		"context length",
		"context window",
		"context_length_exceeded",
		"maximum context",
		"context limit",
		"input exceeds",
		"compaction failed",
	}
	for _, line := range lines {
		text := strings.ToLower(line)
		for _, needle := range needles {
			if strings.Contains(text, needle) {
				return true
			}
		}
	}
	return false
}

func resolveActiveChildSessionPath(child *ChildRunRef) (string, error) {
	if child == nil {
		return "", fmt.Errorf("no active child")
	}
	if strings.TrimSpace(child.SessionPath) != "" {
		return strings.TrimSpace(child.SessionPath), nil
	}
	return ResolveSessionPath(child.SessionDir, child.SessionID, child.Cwd)
}

func providerContextEvidenceLines(e AssistantTerminalEvidence) []string {
	lines := []string{
		fmt.Sprintf("provider stopReason: %s", e.StopReason),
		fmt.Sprintf("provider error: %s", e.ErrorMessage),
		fmt.Sprintf("evidence id: %s", e.EvidenceID),
	}
	if e.SessionPath != "" {
		lines = append(lines, fmt.Sprintf("session: %s", e.SessionPath))
	}
	if e.SessionID != "" {
		lines = append(lines, fmt.Sprintf("session id: %s", e.SessionID))
	}
	if e.Line > 0 {
		lines = append(lines, fmt.Sprintf("line: %d", e.Line))
	}
	if e.Timestamp != "" {
		lines = append(lines, fmt.Sprintf("timestamp: %s", e.Timestamp))
	}
	return lines
}

func providerContextRecoverySafeCommand(stateFile string) string {
	return fmt.Sprintf(
		"vamos qrspi inspect --state-file %s --sessions --latest",
		stateFile,
	)
}

func providerContextRecoveryContinueCommand(stateFile string) string {
	return fmt.Sprintf(
		"vamos qrspi recover-manual --state-file %s --mode latest-session --continue",
		stateFile,
	)
}

func providerContextRecoverySummaryCommand(
	stateFile string,
	evidence AssistantTerminalEvidence,
) string {
	if strings.TrimSpace(evidence.SessionPath) == "" {
		return ""
	}
	return fmt.Sprintf(
		"vamos qrspi recover-summary --state-file %s --session-file %s",
		stateFile,
		evidence.SessionPath,
	)
}

func IsTerminalFailedChild(health ActiveChildHealth) bool {
	return health.Status == ActiveChildLaunchFailed
}

func IsTerminalProviderContextError(health ActiveChildHealth) bool {
	return health.Status == ActiveChildProviderContextError ||
		(health.TerminalEvidence != nil && health.TerminalEvidence.ContextWindowError)
}

func IsRecoverableNoResultChild(health ActiveChildHealth) bool {
	return health.Status == ActiveChildContextExhausted ||
		IsTerminalProviderContextError(health)
}
