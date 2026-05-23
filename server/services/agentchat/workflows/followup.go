package workflows

import (
	"fmt"
	"path"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

func maybeEnterImplementationFollowup(
	decision wruntime.TransitionDecision,
	result wruntime.WorkflowResult,
) (wruntime.TransitionDecision, error) {
	if result.Outcome != wruntime.OutcomeNeedsFollowup {
		return decision, nil
	}

	nextState, err := enterImplementationFollowup(decision.State, result)
	if err != nil {
		return decision, err
	}
	decision.State = nextState
	decision.NextNodeID = qrspi.NodeQuestion
	decision.StartNext = true
	decision.WaitingHuman = false
	decision.StopReason = ""
	decision.Events = append(decision.Events, wruntime.Event{
		Type:   "workflow_followup_entered",
		NodeID: qrspi.NodeQuestion,
	})
	return decision, nil
}

func maybeExitImplementationFollowup(
	state wruntime.State,
	decision wruntime.TransitionDecision,
	result wruntime.WorkflowResult,
) wruntime.TransitionDecision {
	if result.SourceNodeID != qrspi.NodeReviewImplementation ||
		result.Outcome != wruntime.OutcomeReadyForHumanReview ||
		len(state.Followups) == 0 {
		return decision
	}

	nextState, parentReview := exitImplementationFollowup(decision.State)
	nextState.CurrentNodeID = parentReview
	nextState.PendingNextNodeID = parentReview
	nextState.Status = wruntime.WorkspaceStatusIdle
	nextState.HumanGate = nil
	decision.State = nextState
	decision.NextNodeID = parentReview
	decision.StartNext = true
	decision.WaitingHuman = false
	decision.StopReason = ""
	decision.Events = []wruntime.Event{{
		Type:   "workflow_followup_exited",
		NodeID: parentReview,
	}}
	return decision
}

func enterImplementationFollowup(
	state wruntime.State,
	result wruntime.WorkflowResult,
) (wruntime.State, error) {
	followupPlan := artifactByRoleResult(result, "followup-plan")
	if followupPlan == "" {
		followupPlan = artifactByRoleResult(result, "followup-questions")
	}
	if followupPlan == "" {
		return state, fmt.Errorf(
			"implementation follow-up requires followup-plan or followup-questions artifact",
		)
	}

	parentPlanDir, err := inferParentPlanDir(result.PrimaryArtifact)
	if err != nil {
		return state, err
	}
	followupPlanDir, err := inferImplementationReviewPlanDir(followupPlan)
	if err != nil {
		return state, err
	}

	state.Followups = append(state.Followups, wruntime.FollowupContext{
		ParentPlanDir:      parentPlanDir,
		FollowupPlanDir:    followupPlanDir,
		ParentReviewNodeID: result.SourceNodeID,
		ParentReviewPath:   strings.TrimSpace(result.PrimaryArtifact),
	})
	state.CurrentNodeID = qrspi.NodeQuestion
	state.PendingNextNodeID = qrspi.NodeQuestion
	state.Status = wruntime.WorkspaceStatusIdle
	state.HumanGate = nil
	return state, nil
}

func exitImplementationFollowup(state wruntime.State) (wruntime.State, wruntime.NodeID) {
	parent := qrspi.NodeReviewImplementation
	if len(state.Followups) == 0 {
		return state, parent
	}
	top := state.Followups[len(state.Followups)-1]
	if top.ParentReviewNodeID != "" {
		parent = top.ParentReviewNodeID
	}
	state.Followups = state.Followups[:len(state.Followups)-1]
	return state, parent
}

func artifactByRoleResult(result wruntime.WorkflowResult, roles ...string) string {
	for _, role := range roles {
		for _, artifact := range result.Artifacts {
			if strings.EqualFold(strings.TrimSpace(artifact.Role), role) {
				return strings.TrimSpace(artifact.Path)
			}
		}
	}
	return ""
}

func inferParentPlanDir(reviewPath string) (string, error) {
	reviewPath = cleanArtifactPath(reviewPath)
	beforeReviews, _, ok := strings.Cut(reviewPath, "/reviews/")
	if !ok || beforeReviews == "" {
		return "", fmt.Errorf(
			"implementation follow-up review path %q must be under a parent plan reviews directory",
			reviewPath,
		)
	}
	return beforeReviews, nil
}

func inferImplementationReviewPlanDir(artifactPath string) (string, error) {
	artifactPath = cleanArtifactPath(artifactPath)
	parts := strings.Split(artifactPath, "/")
	for i, part := range parts {
		if strings.HasSuffix(part, "_implementation-review") {
			return strings.Join(parts[:i+1], "/"), nil
		}
	}
	return "", fmt.Errorf(
		"implementation follow-up artifact %q must be under a *_implementation-review directory",
		artifactPath,
	)
}

func cleanArtifactPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.TrimPrefix(path.Clean(value), "./")
}
