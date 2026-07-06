package qrspicmd

import (
	"context"
	"fmt"
	"strings"
)

type ManagerPaneAdoptionCommand string

const (
	ManagerPaneAdoptionStartNext    ManagerPaneAdoptionCommand = "start-next"
	ManagerPaneAdoptionContinue     ManagerPaneAdoptionCommand = "continue"
	ManagerPaneAdoptionManagerReady ManagerPaneAdoptionCommand = "manager-ready"
)

type ManagerPaneAdoptionOptions struct {
	StateFile    string
	Command      ManagerPaneAdoptionCommand
	ExplicitPane string
	CurrentPane  string
}

type PaneLiveness struct {
	PaneID  string
	Checked bool
	Exists  bool
	Error   string
}

type ManagerPaneAdoptionResult struct {
	State       ManagerState
	Changed     bool
	AdoptedPane string
	Reason      string
	Evidence    []string
	ActionCard  *ManagerActionCard
}

func ResolveManagerPaneAdoption(
	ctx context.Context,
	state ManagerState,
	opts ManagerPaneAdoptionOptions,
	d deps,
) (ManagerPaneAdoptionResult, error) {
	opts.StateFile = strings.TrimSpace(opts.StateFile)
	opts.ExplicitPane = strings.TrimSpace(opts.ExplicitPane)
	opts.CurrentPane = strings.TrimSpace(opts.CurrentPane)

	result := ManagerPaneAdoptionResult{State: state}
	stored := strings.TrimSpace(state.ManagerPaneID)
	delivery := strings.TrimSpace(state.Delivery.ManagerPaneID)
	selected := firstNonEmpty(delivery, stored)
	selectedLive := managerPaneLiveness(ctx, selected, d)
	storedLive := managerPaneLiveness(ctx, stored, d)
	deliveryLive := managerPaneLiveness(ctx, delivery, d)
	currentLive := managerPaneLiveness(ctx, opts.CurrentPane, d)
	result.Evidence = managerPaneEvidence(
		state,
		opts,
		storedLive,
		deliveryLive,
		currentLive,
	)

	if opts.ExplicitPane != "" {
		result.State.ManagerPaneID = opts.ExplicitPane
		if shouldRebindDeliveryPane(state, opts.ExplicitPane, true, selectedLive) {
			result.State.Delivery.ManagerPaneID = opts.ExplicitPane
		}
		result.Changed = result.State.ManagerPaneID != state.ManagerPaneID ||
			result.State.Delivery.ManagerPaneID != state.Delivery.ManagerPaneID
		result.AdoptedPane = opts.ExplicitPane
		result.Reason = "explicit_manager_pane"
		result.Evidence = append(result.Evidence, "adoption: explicit --manager-pane")
		return result, nil
	}

	if opts.CurrentPane == "" {
		result.Reason = "no_current_manager_pane"
		return result, nil
	}
	if currentLive.Checked && !currentLive.Exists {
		result.Reason = "current_manager_pane_unavailable"
		result.Evidence = append(
			result.Evidence,
			"adoption: current TMUX_PANE unavailable; not adopted",
		)
		return result, nil
	}
	if selected != "" && selected == opts.CurrentPane {
		if state.ManagerPaneID == "" {
			result.State.ManagerPaneID = opts.CurrentPane
			result.Changed = true
		}
		if shouldRebindDeliveryPane(state, opts.CurrentPane, false, selectedLive) &&
			state.Delivery.ManagerPaneID != opts.CurrentPane {
			result.State.Delivery.ManagerPaneID = opts.CurrentPane
			result.Changed = true
		}
		result.AdoptedPane = opts.CurrentPane
		result.Reason = "current_matches_manager_pane"
		return result, nil
	}
	canAutoAdopt := managerPaneAutoAdoptionAllowed(state, selectedLive)
	if selected != "" && selectedLive.Exists && !canAutoAdopt {
		result.ActionCard = buildManagerPaneActionCard(
			state,
			opts,
			result.Evidence,
			ActionManagerPaneAdoptionRequired,
		)
		result.Reason = "live_manager_pane_conflict"
		return result, nil
	}
	if canAutoAdopt {
		result.State.ManagerPaneID = opts.CurrentPane
		if shouldRebindDeliveryPane(state, opts.CurrentPane, false, selectedLive) {
			result.State.Delivery.ManagerPaneID = opts.CurrentPane
		}
		result.Changed = result.State.ManagerPaneID != state.ManagerPaneID ||
			result.State.Delivery.ManagerPaneID != state.Delivery.ManagerPaneID
		result.AdoptedPane = opts.CurrentPane
		result.Reason = "safe_current_pane_adoption"
		result.Evidence = append(result.Evidence, "adoption: safe current TMUX_PANE")
		return result, nil
	}
	return result, nil
}

func managerPaneAutoAdoptionAllowed(state ManagerState, selected PaneLiveness) bool {
	if strings.EqualFold(state.Delivery.Status, "compacting") ||
		state.Delivery.QueuedWake != nil {
		return true
	}
	selectedPane := strings.TrimSpace(selected.PaneID)
	if selectedPane == "" {
		return true
	}
	if !selected.Checked || !selected.Exists {
		return true
	}
	return false
}

func shouldRebindDeliveryPane(
	state ManagerState,
	adoptedPane string,
	explicit bool,
	selected PaneLiveness,
) bool {
	if strings.TrimSpace(adoptedPane) == "" {
		return false
	}
	if explicit {
		return true
	}
	if strings.TrimSpace(state.Delivery.ManagerPaneID) == "" {
		return true
	}
	if strings.EqualFold(state.Delivery.Status, "compacting") ||
		state.Delivery.QueuedWake != nil {
		return true
	}
	return selected.Checked &&
		strings.TrimSpace(
			selected.PaneID,
		) == strings.TrimSpace(
			state.Delivery.ManagerPaneID,
		) &&
		!selected.Exists
}

func managerPaneLiveness(ctx context.Context, pane string, d deps) PaneLiveness {
	pane = strings.TrimSpace(pane)
	live := PaneLiveness{PaneID: pane}
	if pane == "" {
		return live
	}
	live.Checked = true
	ok, err := tmuxClient(d).PaneExists(ctx, TmuxPane{ID: pane})
	live.Exists = ok && err == nil
	if err != nil {
		live.Error = err.Error()
	}
	return live
}

func buildManagerPaneActionCard(
	state ManagerState,
	opts ManagerPaneAdoptionOptions,
	evidence []string,
	kind string,
) *ManagerActionCard {
	summary := "q-manager parent pane adoption needs explicit operator intent"
	recommended := "rerun from the intended parent tmux pane with --manager-pane"
	if kind == ActionManagerPaneUnavailable {
		summary = "q-manager manager pane unavailable; wake queued for explicit parent recovery"
		recommended = "run manager-ready from the intended parent tmux pane"
	}
	return &ManagerActionCard{
		Kind:              kind,
		Severity:          "warning",
		Summary:           summary,
		Evidence:          evidence,
		RecommendedAction: recommended,
		SafeCommand:       managerPaneSafeCommand(opts),
		ContinueCommand:   continueCommand(opts.StateFile),
		RequiresHuman:     false,
	}
}

func managerPaneSafeCommand(opts ManagerPaneAdoptionOptions) string {
	stateFile := strings.TrimSpace(opts.StateFile)
	switch opts.Command {
	case ManagerPaneAdoptionStartNext:
		return fmt.Sprintf(
			"vamos qrspi start-next --state-file %s --manager-pane \"$TMUX_PANE\"",
			stateFile,
		)
	case ManagerPaneAdoptionManagerReady:
		return fmt.Sprintf(
			"vamos qrspi manager-ready --state-file %s --manager-pane \"$TMUX_PANE\"",
			stateFile,
		)
	default:
		return fmt.Sprintf(
			"vamos qrspi continue --state-file %s --manager-pane \"$TMUX_PANE\"",
			stateFile,
		)
	}
}

func managerPaneEvidence(
	state ManagerState,
	opts ManagerPaneAdoptionOptions,
	stored, delivery, current PaneLiveness,
) []string {
	evidence := []string{
		fmt.Sprintf("state file: %s", opts.StateFile),
		fmt.Sprintf("command: %s", opts.Command),
		fmt.Sprintf(
			"stored manager pane: %s",
			firstNonEmpty(state.ManagerPaneID, "<empty>"),
		),
		fmt.Sprintf(
			"delivery manager pane: %s",
			firstNonEmpty(state.Delivery.ManagerPaneID, "<empty>"),
		),
		fmt.Sprintf("current TMUX_PANE: %s", firstNonEmpty(opts.CurrentPane, "<empty>")),
		fmt.Sprintf(
			"delivery status: %s",
			firstNonEmpty(state.Delivery.Status, "<empty>"),
		),
		fmt.Sprintf("queued wake: %t", state.Delivery.QueuedWake != nil),
	}
	for _, live := range []PaneLiveness{stored, delivery, current} {
		if !live.Checked {
			continue
		}
		line := fmt.Sprintf("pane liveness: %s exists=%t", live.PaneID, live.Exists)
		if live.Error != "" {
			line += " error=" + live.Error
		}
		evidence = append(evidence, line)
	}
	return evidence
}
