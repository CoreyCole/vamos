package workspaces

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"
	"unicode"
)

func workspaceCardTitle(ws Workspace) string {
	if ws.IsMain || ws.Slug == mainWorkspaceSlug {
		return "Main"
	}
	_, titleSlug, ok := workspaceSlugTimestamp(ws.Slug)
	if !ok {
		if strings.TrimSpace(ws.DisplayName) != "" {
			return workspaceTitleCase(ws.DisplayName)
		}
		titleSlug = ws.Slug
	}
	return workspaceTitleCase(strings.ReplaceAll(titleSlug, "-", " "))
}

func workspaceViewTitle(view ImplWorkspaceView) string {
	if view.IsMain {
		return "Main"
	}
	if strings.TrimSpace(view.Row.DisplayName) != "" {
		return workspaceTitleCase(view.Row.DisplayName)
	}
	if strings.TrimSpace(view.Runtime.Workspace.DisplayName) != "" {
		return workspaceCardTitle(view.Runtime.Workspace)
	}
	return workspaceTitleCase(strings.ReplaceAll(workspaceViewSlug(view), "-", " "))
}

func workspaceViewSlug(view ImplWorkspaceView) string {
	if view.HasRuntime && strings.TrimSpace(view.Runtime.Workspace.Slug) != "" {
		return strings.TrimSpace(view.Runtime.Workspace.Slug)
	}
	if strings.TrimSpace(view.Row.WorkspaceSlug) != "" {
		return strings.TrimSpace(view.Row.WorkspaceSlug)
	}
	return strings.TrimSpace(view.Runtime.Workspace.Slug)
}

func canActOnImplWorkspace(view ImplWorkspaceView) bool {
	return view.Row.Status == string(ImplWorkspaceStatusActive) &&
		strings.TrimSpace(workspaceViewCheckoutPath(view)) != "" &&
		strings.TrimSpace(view.Runtime.Workspace.Slug) != ""
}

func isHistoricalImplWorkspaceView(
	view ImplWorkspaceView,
	protected map[string]ReleaseLaneWorkspace,
) bool {
	slug := workspaceViewSlug(view)
	if lane, ok := protected[slug]; ok && lane.Protected {
		return false
	}
	if view.IsMain || slug == mainWorkspaceSlug {
		return false
	}
	if view.Row.Status == string(ImplWorkspaceStatusMerged) ||
		view.Row.Status == string(ImplWorkspaceStatusCleanedUp) {
		return true
	}
	if view.HasRuntime && strings.TrimSpace(view.Runtime.Workspace.Slug) != "" {
		return false
	}
	checkout := strings.TrimSpace(workspaceViewCheckoutPath(view))
	if checkout == "" {
		return true
	}
	return !view.HasRuntime || strings.TrimSpace(view.Runtime.Workspace.Slug) == ""
}

func filterHistoricalImplWorkspaceViews(
	views []ImplWorkspaceView,
	showHistorical bool,
	protected ...map[string]ReleaseLaneWorkspace,
) []ImplWorkspaceView {
	if showHistorical {
		return views
	}
	protectedBySlug := map[string]ReleaseLaneWorkspace{}
	if len(protected) > 0 && protected[0] != nil {
		protectedBySlug = protected[0]
	}
	out := make([]ImplWorkspaceView, 0, len(views))
	for _, view := range views {
		filteredChildren := filterHistoricalImplWorkspaceViews(
			view.Children,
			showHistorical,
			protectedBySlug,
		)
		view.Children = filteredChildren
		if isHistoricalImplWorkspaceView(view, protectedBySlug) {
			out = append(out, filteredChildren...)
			continue
		}
		out = append(out, view)
	}
	return out
}

func workspaceViewCheckoutPath(view ImplWorkspaceView) string {
	if strings.TrimSpace(view.Row.CheckoutPath) != "" {
		return view.Row.CheckoutPath
	}
	return view.Runtime.Workspace.CheckoutPath
}

func workspacePlanBindingLabel(view ImplWorkspaceView) string {
	if view.Row.PlanDirRel.Valid && strings.TrimSpace(view.Row.PlanDirRel.String) != "" {
		return view.Row.PlanDirRel.String
	}
	if view.Row.PlanDir.Valid {
		return strings.TrimSpace(view.Row.PlanDir.String)
	}
	return ""
}

func workspaceViewActivity(view ImplWorkspaceView) string {
	if view.Row.UpdatedAt.IsZero() {
		return ""
	}
	return view.Row.UpdatedAt.Format("Jan 2, 2006 · 3:04 PM")
}

func workspaceImplStatusBadge(view ImplWorkspaceView) string {
	switch view.Row.Status {
	case string(ImplWorkspaceStatusMerged):
		return "Merged"
	case string(ImplWorkspaceStatusCleanedUp):
		return "Cleaned up"
	default:
		return workspaceTransitionLabel(view.Runtime)
	}
}

func workspaceCardTimestamp(ws Workspace) string {
	stamp, _, ok := workspaceSlugTimestamp(ws.Slug)
	if !ok {
		return ""
	}
	return stamp.Format("Jan 2, 2006 · 3:04 PM")
}

func workspaceTrunkBranch(ws Workspace) string {
	if strings.TrimSpace(ws.Stack.TrunkBranch) != "" {
		return ws.Stack.TrunkBranch
	}
	if strings.TrimSpace(ws.Stack.BaseBranch) != "" {
		return ws.Stack.BaseBranch
	}
	return ""
}

func workspaceBottomBranch(ws Workspace) string {
	return strings.TrimSpace(ws.Stack.BottomBranch)
}

func workspaceTopBranch(ws Workspace) string {
	return strings.TrimSpace(ws.Stack.TopBranch)
}

func workspaceActionIndicator(slug, action string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(slug)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strings.TrimSpace(action)))
	return fmt.Sprintf("_workspaceAction%x", h.Sum32())
}

func workspaceActionIndicatorSignal(slug, action string) string {
	return "$" + workspaceActionIndicator(slug, action)
}

func releaseActionLabel(action ReleaseActionView) string {
	if strings.TrimSpace(action.Label) != "" {
		return action.Label
	}
	return workspaceTitleCase(strings.ReplaceAll(string(action.FlowID), "_", " "))
}

func releaseDisabledReason(action ReleaseActionView) string {
	if !action.Disabled {
		return ""
	}
	if strings.TrimSpace(action.DisabledReason) != "" {
		return action.DisabledReason
	}
	return "release action unavailable"
}

func workspaceQRSPIBadge(view ImplWorkspaceView) string {
	return workspaceWorkflowStageLabel(view.Workflow)
}

func workspaceRuntimeLabel(view ImplWorkspaceView) string {
	if isHistoricalImplWorkspaceView(view, nil) {
		return workspaceImplStatusBadge(view)
	}
	return workspaceTransitionLabel(view.Runtime)
}

func workspaceBranchLabel(view ImplWorkspaceView) string {
	if branch := workspaceTopBranch(view.Runtime.Workspace); branch != "" {
		return branch
	}
	if branch := strings.TrimSpace(view.Runtime.Workspace.Branch); branch != "" {
		return branch
	}
	if view.Row.TopBranch.Valid && strings.TrimSpace(view.Row.TopBranch.String) != "" {
		return strings.TrimSpace(view.Row.TopBranch.String)
	}
	if view.Row.Branch.Valid && strings.TrimSpace(view.Row.Branch.String) != "" {
		return strings.TrimSpace(view.Row.Branch.String)
	}
	return "—"
}

func workspaceCommit(view ImplWorkspaceView) string {
	if commit := strings.TrimSpace(view.Runtime.Workspace.Commit); commit != "" {
		return commit
	}
	if view.Row.CommitHash.Valid {
		return strings.TrimSpace(view.Row.CommitHash.String)
	}
	return ""
}

func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) > 7 {
		return commit[:7]
	}
	if commit == "" {
		return "—"
	}
	return commit
}

func workspaceViewLabel(view ImplWorkspaceView) string {
	return workspaceViewTitle(view)
}

func workspaceReleaseSummary(view ImplWorkspaceView) string {
	if len(view.ReleaseActions) == 0 {
		return "No release action"
	}
	for _, action := range view.ReleaseActions {
		if !action.Disabled {
			return releaseActionLabel(action)
		}
	}
	return releaseDisabledReason(view.ReleaseActions[0])
}

func releaseQueueSummary(panel ReleasePanelModel) string {
	active := len(panel.Queue.Active)
	pending := len(panel.Queue.Pending)
	history := len(panel.History)
	if active == 0 && pending == 0 && history == 0 {
		return "empty"
	}
	return fmt.Sprintf("%d active · %d pending · %d history", active, pending, history)
}

func releaseLaneStatusLabel(lane ReleaseLaneView) string {
	status := strings.TrimSpace(string(lane.Workspace.Status))
	if status == "" {
		return "unknown"
	}
	return status
}

func workspaceWorkflowStageLabel(summary WorkspaceWorkflowSummary) string {
	stage := strings.TrimSpace(summary.Stage)
	status := strings.TrimSpace(summary.Status)
	if stage == "" && status == "" {
		return "No plan"
	}
	if summary.WaitingHuman || strings.Contains(stage, "human") {
		return "Human review"
	}
	if status == "done" || status == "complete" {
		return "Done"
	}
	switch stage {
	case "question", "research", "design", "design-product", "outline", "plan", "review-outline", "review-plan":
		return "Plan"
	case "workspace":
		return "Workspace"
	case "implement":
		return "Implementing"
	case "review-implementation":
		return "Implementation review"
	case "verify":
		return "Verify"
	case "done":
		return "Done"
	case "":
		return "Unknown"
	default:
		return workspaceTitleCase(strings.ReplaceAll(stage, "-", " "))
	}
}

func workspaceCanStart(snap WorkspaceLifecycleSnapshot) bool {
	ws := snap.Workspace
	if ws.IsMain {
		return false
	}
	switch normalizedObservedState(snap) {
	case WorkspaceObservedStopped, WorkspaceObservedFailed, WorkspaceObservedCrashed:
		return true
	default:
		return false
	}
}

func workspaceCanStop(snap WorkspaceLifecycleSnapshot) bool {
	return !snap.Workspace.IsMain &&
		normalizedObservedState(snap) == WorkspaceObservedRunning
}

func workspaceCanRestart(snap WorkspaceLifecycleSnapshot) bool {
	return !snap.Workspace.IsMain &&
		normalizedObservedState(snap) == WorkspaceObservedRunning
}

func workspaceTransitioning(snap WorkspaceLifecycleSnapshot) bool {
	switch normalizedObservedState(snap) {
	case WorkspaceObservedStarting, WorkspaceObservedStopping:
		return true
	default:
		return false
	}
}

func workspaceTransitionLabel(snap WorkspaceLifecycleSnapshot) string {
	return string(normalizedObservedState(snap))
}

func normalizedObservedState(snap WorkspaceLifecycleSnapshot) WorkspaceObservedState {
	if snap.ObservedState == "" {
		return observedFromStatus(snap.Workspace.Status)
	}
	return snap.ObservedState
}

func workspaceSlugTimestamp(slug string) (time.Time, string, bool) {
	const timestampPartCount = 6

	parts := strings.Split(slug, "-")
	if len(parts) < timestampPartCount+1 {
		return time.Time{}, slug, false
	}
	candidate := strings.Join(parts[:timestampPartCount], "-")
	stamp, err := time.Parse("2006-01-02-15-04-05", candidate)
	if err != nil {
		return time.Time{}, slug, false
	}
	return stamp, strings.Join(parts[timestampPartCount:], "-"), true
}

func workspaceTitleCase(value string) string {
	words := strings.Fields(strings.ReplaceAll(value, "-", " "))
	for i, word := range words {
		words[i] = workspaceTitleWord(word)
	}
	return strings.Join(words, " ")
}

func workspaceTitleWord(word string) string {
	lower := strings.ToLower(strings.TrimSpace(word))
	switch lower {
	case "api",
		"cli",
		"css",
		"db",
		"dns",
		"go",
		"html",
		"http",
		"https",
		"id",
		"js",
		"json",
		"llm",
		"mpa",
		"oauth",
		"pid",
		"qrspi",
		"sse",
		"sql",
		"sqlc",
		"tls",
		"ts",
		"ui",
		"url",
		"ux":
		return strings.ToUpper(lower)
	case "cn":
		return "CN"
	case "agentchat":
		return "Agent Chat"
	case "datastar":
		return "Datastar"
	case "github":
		return "GitHub"
	case "temporal":
		return "Temporal"
	case "vamos":
		return "Vamos"
	}

	runes := []rune(lower)
	if len(runes) == 0 {
		return ""
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
