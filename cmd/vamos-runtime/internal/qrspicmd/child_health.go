package qrspicmd

import (
	"context"
	"fmt"
	"os"
	"strings"
)

func InspectActiveChildHealth(ctx context.Context, state ManagerState, stateFile string, d deps) (ActiveChildHealth, error) {
	if state.ActiveChild == nil {
		return ActiveChildHealth{Status: ActiveChildUnknown, Evidence: []string{"no active child"}}, nil
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
		SafeCommand: fmt.Sprintf("vamos qrspi repair-state --state-file %s --clear-failed-child --relaunch", stateFile),
	}

	status, _ := ReadChildStatus(child.StatusPath)
	if status != nil {
		health.ExitCode = &status.ExitCode
		health.Evidence = append(health.Evidence, fmt.Sprintf("exitCode: %d", status.ExitCode))
	}
	if data, err := os.ReadFile(child.OutputPath); err == nil {
		health.OutputTail = FilterChildOutputTail(data, 8)
	}

	hasResult, sessionPath, resultErr := ChildHasQRSPIResult(state)
	if hasResult {
		health.Status = ActiveChildFinishedNeedsValidation
		health.SessionPath = sessionPath
		health.Evidence = append(health.Evidence, "session has qrspi_result")
		return health, nil
	}

	paneOK := false
	if strings.TrimSpace(child.TmuxPaneID) != "" {
		ok, err := tmuxClient(d).PaneExists(ctx, TmuxPane{ID: child.TmuxPaneID})
		paneOK = ok && err == nil
		if !paneOK {
			health.Evidence = append(health.Evidence, "pane missing")
		}
	}
	if status != nil && status.ExitCode != 0 && HasDoneMarker(child.DonePath) && !hasResult {
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
	health.Evidence = append(health.Evidence, "pane exists and no terminal failure status")
	return health, nil
}

func HasDoneMarker(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func ChildHasQRSPIResult(state ManagerState) (bool, string, error) {
	text, parseCtx, err := ReadChildResultText(state, ResultSourceOptions{})
	if err != nil {
		return false, "", err
	}
	return strings.Contains(text, "qrspi_result"), parseCtx.SessionPath, nil
}

func IsTerminalFailedChild(health ActiveChildHealth) bool {
	return health.Status == ActiveChildLaunchFailed
}
