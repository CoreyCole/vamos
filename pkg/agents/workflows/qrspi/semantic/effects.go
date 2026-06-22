package semantic

import (
	"fmt"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type EffectKind string

const (
	EffectUpdateExecutionCwd EffectKind = "update_execution_cwd"
	EffectEnterFollowup      EffectKind = "enter_followup"
	EffectExitFollowup       EffectKind = "exit_followup"
	EffectPersistResult      EffectKind = "persist_result"
	EffectStartNext          EffectKind = "start_next"
	EffectWaitHuman          EffectKind = "wait_human"
	EffectInvalidRetry       EffectKind = "invalid_retry"
	EffectInvalidExhausted   EffectKind = "invalid_exhausted"
)

type Effect struct {
	Kind    EffectKind      `json:"kind"`
	NodeID  wruntime.NodeID `json:"node_id,omitempty"`
	Path    string          `json:"path,omitempty"`
	Message string          `json:"message,omitempty"`
}

func DeriveEffects(before wruntime.State, result wruntime.WorkflowResult, decision wruntime.TransitionDecision, context Context) ([]Effect, error) {
	effects := []Effect{{Kind: EffectPersistResult, NodeID: result.SourceNodeID}}
	if path := ExecutionCwdFromWorkspaceResult(result); path != "" && result.SourceNodeID == qrspi.NodeWorkspace {
		effects = append(effects, Effect{Kind: EffectUpdateExecutionCwd, NodeID: result.SourceNodeID, Path: path})
	}
	if result.SourceNodeID == qrspi.NodeReviewImplementation && result.Outcome == wruntime.OutcomeNeedsFollowup {
		followup, err := implementationFollowupPath(result)
		if err != nil {
			return nil, err
		}
		effects = append(effects, Effect{Kind: EffectEnterFollowup, NodeID: qrspi.NodeQuestion, Path: followup})
	}
	if result.SourceNodeID == qrspi.NodeReviewImplementation && result.Outcome == wruntime.OutcomeReadyForHumanReview && len(before.Followups) > 0 {
		effects = append(effects, Effect{Kind: EffectExitFollowup, NodeID: qrspi.NodeReviewImplementation})
	}
	if decision.StartNext {
		effects = append(effects, Effect{Kind: EffectStartNext, NodeID: decision.NextNodeID})
	}
	if decision.WaitingHuman || result.Status == wruntime.StatusNeedsHuman {
		effects = append(effects, Effect{Kind: EffectWaitHuman, NodeID: decision.NextNodeID, Message: decision.StopReason})
	}
	return effects, nil
}

func implementationFollowupPath(result wruntime.WorkflowResult) (string, error) {
	followup := artifactByRole(result, "followup-plan", "followup-questions")
	if followup == "" {
		return "", fmt.Errorf("implementation follow-up requires followup-plan or followup-questions artifact")
	}
	return inferImplementationReviewPlanDir(followup)
}

func artifactByRole(result wruntime.WorkflowResult, roles ...string) string {
	for _, role := range roles {
		for _, artifact := range result.Artifacts {
			if strings.EqualFold(strings.TrimSpace(artifact.Role), role) {
				return strings.TrimSpace(artifact.Path)
			}
		}
	}
	return ""
}

func inferImplementationReviewPlanDir(artifactPath string) (string, error) {
	artifactPath = cleanArtifactPath(artifactPath)
	parts := strings.Split(artifactPath, "/")
	for i, part := range parts {
		if strings.HasSuffix(part, "_implementation-review") {
			return strings.Join(parts[:i+1], "/"), nil
		}
	}
	return "", fmt.Errorf("implementation follow-up artifact %q must be under a *_implementation-review directory", artifactPath)
}
