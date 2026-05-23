package workflows

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
)

func TestQRSPIEndToEndPlanningPath(t *testing.T) {
	svc, store, runner := newQRSPIIntegrationHarness(t, nil)

	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeQuestion,
		"complete",
		"questions/runtime.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodeResearch)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeResearch,
		"complete",
		"research/runtime.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodeDesign)
	completeQRSPIIntegrationNode(t, svc, store, qrspi.NodeDesign, "complete", "design.md")
	assertCurrentNode(t, store, runner, qrspi.NodeReviewDesign)

	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeReviewDesign,
		"complete",
		"reviews/design-review/review.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodeOutline)

	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeOutline,
		"complete",
		"outline.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodeReviewOutline)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeReviewOutline,
		"complete",
		"reviews/outline-review/review.md",
	)
	assertWaitingHuman(
		t,
		store,
		qrspi.NodeHumanReviewOutline,
		qrspi.NodeReviewOutline,
		"reviews/outline-review/review.md",
	)
	advanceQRSPIIntegrationGate(t, svc, store, runner, qrspi.NodeHumanReviewOutline)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeHumanReviewOutline,
		"complete",
		"outline.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodePlan)

	completeQRSPIIntegrationNode(t, svc, store, qrspi.NodePlan, "complete", "plan.md")
	assertCurrentNode(t, store, runner, qrspi.NodeReviewPlan)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeReviewPlan,
		"complete",
		"reviews/plan-review/review.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodeWorkspace)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeWorkspace,
		"complete",
		"plan.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodeImplement)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeImplement,
		"complete",
		"handoffs/implement-handoff.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodeReviewImplementation)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeReviewImplementation,
		"complete",
		"reviews/implementation-review/review.md",
	)
	assertWaitingHuman(
		t,
		store,
		qrspi.NodeHumanReviewImplementation,
		qrspi.NodeReviewImplementation,
		"reviews/implementation-review/review.md",
	)
	advanceQRSPIIntegrationGate(
		t,
		svc,
		store,
		runner,
		qrspi.NodeHumanReviewImplementation,
	)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeHumanReviewImplementation,
		"complete",
		"done.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodeDone)
	completeQRSPIIntegrationNode(t, svc, store, qrspi.NodeDone, "done", "done.md")
	if store.state.Status != wruntime.WorkspaceStatusDone {
		t.Fatalf("status = %q, want done", store.state.Status)
	}
}

func TestQRSPIUpdatedPolicyAffectsNextCompletionTransition(t *testing.T) {
	policy := mustQRSPIIntegrationPolicy(t, qrspi.DefaultPolicy())
	svc, store, runner := newQRSPIIntegrationHarness(t, policy)
	moveQRSPIIntegrationState(t, store, qrspi.NodeDesign)
	store.state.Policy = mustQRSPIIntegrationPolicy(t, qrspi.Policy{
		AutoMode:                false,
		EnablePlanReviews:       false,
		InvalidResultRetryLimit: 1,
	})

	completeQRSPIIntegrationNode(t, svc, store, qrspi.NodeDesign, "complete", "design.md")

	assertCurrentNode(t, store, runner, qrspi.NodeOutline)
}

func TestQRSPIUpdatedPolicyWhileWaitingHumanDoesNotReselectCurrentGate(t *testing.T) {
	policy := mustQRSPIIntegrationPolicy(t, qrspi.Policy{
		AutoMode:                false,
		EnablePlanReviews:       true,
		InvalidResultRetryLimit: 1,
	})
	svc, store, runner := newQRSPIIntegrationHarness(t, policy)
	moveQRSPIIntegrationState(t, store, qrspi.NodeReviewOutline)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeReviewOutline,
		"complete",
		"reviews/outline-review/review.md",
	)
	assertWaitingHuman(
		t,
		store,
		qrspi.NodeHumanReviewOutline,
		qrspi.NodeReviewOutline,
		"reviews/outline-review/review.md",
	)

	store.state.Policy = mustQRSPIIntegrationPolicy(t, qrspi.Policy{
		AutoMode:                true,
		EnablePlanReviews:       false,
		InvalidResultRetryLimit: 1,
	})

	advanceQRSPIIntegrationGate(t, svc, store, runner, qrspi.NodeHumanReviewOutline)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeHumanReviewOutline,
		"complete",
		"outline.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodePlan)
}

func TestQRSPIPlanReviewsDisabledPath(t *testing.T) {
	policy := mustQRSPIIntegrationPolicy(
		t,
		qrspi.Policy{
			AutoMode:                false,
			EnablePlanReviews:       false,
			InvalidResultRetryLimit: 1,
		},
	)
	svc, store, runner := newQRSPIIntegrationHarness(t, policy)

	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeQuestion,
		"complete",
		"questions/runtime.md",
	)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeResearch,
		"complete",
		"research/runtime.md",
	)
	completeQRSPIIntegrationNode(t, svc, store, qrspi.NodeDesign, "complete", "design.md")
	assertCurrentNode(t, store, runner, qrspi.NodeOutline)

	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeOutline,
		"complete",
		"outline.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodePlan)

	completeQRSPIIntegrationNode(t, svc, store, qrspi.NodePlan, "complete", "plan.md")
	assertCurrentNode(t, store, runner, qrspi.NodeWorkspace)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeWorkspace,
		"complete",
		"plan.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodeImplement)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeImplement,
		"complete",
		"handoffs/implement-handoff.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodeReviewImplementation)
}

func TestQRSPIStopsOnNeedsHumanBlockedError(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   wruntime.WorkspaceStatus
	}{
		{status: "needs_human", want: wruntime.WorkspaceStatusWaitingHuman},
		{status: "blocked", want: wruntime.WorkspaceStatusBlocked},
		{status: "error", want: wruntime.WorkspaceStatusError},
	} {
		t.Run(tc.status, func(t *testing.T) {
			svc, store, runner := newQRSPIIntegrationHarness(t, nil)
			completeQRSPIIntegrationNode(
				t,
				svc,
				store,
				qrspi.NodeQuestion,
				tc.status,
				"questions/runtime.md",
			)
			if store.state.Status != tc.want {
				t.Fatalf("status = %q, want %q", store.state.Status, tc.want)
			}
			if len(runner.starts) != 0 {
				t.Fatalf("starts = %#v, want none", runner.starts)
			}
		})
	}
}

func TestQRSPIHumanReviewLoadsAutomatedReviewContext(t *testing.T) {
	svc, store, runner := newQRSPIIntegrationHarness(t, nil)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeQuestion,
		"complete",
		"questions/runtime.md",
	)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeResearch,
		"complete",
		"research/runtime.md",
	)
	completeQRSPIIntegrationNode(t, svc, store, qrspi.NodeDesign, "complete", "design.md")
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeReviewDesign,
		"complete",
		"reviews/design-review/review.md",
	)

	assertCurrentNode(t, store, runner, qrspi.NodeOutline)
}

func TestQRSPIHumanReviewImplementationBeforeDone(t *testing.T) {
	svc, store, runner := newQRSPIIntegrationHarness(t, nil)
	moveQRSPIIntegrationState(t, store, qrspi.NodeImplement)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeImplement,
		"complete",
		"handoffs/implement-handoff.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodeReviewImplementation)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeReviewImplementation,
		"complete",
		"reviews/implementation-review/review.md",
	)
	assertWaitingHuman(
		t,
		store,
		qrspi.NodeHumanReviewImplementation,
		qrspi.NodeReviewImplementation,
		"reviews/implementation-review/review.md",
	)
	advanceQRSPIIntegrationGate(
		t,
		svc,
		store,
		runner,
		qrspi.NodeHumanReviewImplementation,
	)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeHumanReviewImplementation,
		"complete",
		"done.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodeDone)
}

func TestQRSPIReviewImplementationAlwaysRuns(t *testing.T) {
	policy := mustQRSPIIntegrationPolicy(
		t,
		qrspi.Policy{
			AutoMode:                false,
			EnablePlanReviews:       false,
			InvalidResultRetryLimit: 1,
		},
	)
	svc, store, runner := newQRSPIIntegrationHarness(t, policy)
	moveQRSPIIntegrationState(t, store, qrspi.NodeImplement)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeImplement,
		"complete",
		"handoffs/implement-handoff.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodeReviewImplementation)
	if store.state.Nodes[qrspi.NodeReviewImplementation].Status == wruntime.NodeStatusBypassed {
		t.Fatalf("review-implementation was bypassed")
	}
}

func TestQRSPIAutoModeSkipsAutoApprovablePlanningGateOnly(t *testing.T) {
	policy := mustQRSPIIntegrationPolicy(
		t,
		qrspi.Policy{
			AutoMode:                true,
			EnablePlanReviews:       true,
			InvalidResultRetryLimit: 1,
		},
	)
	svc, store, runner := newQRSPIIntegrationHarness(t, policy)
	moveQRSPIIntegrationState(t, store, qrspi.NodeReviewOutline)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeReviewOutline,
		"complete",
		"reviews/outline-review/review.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodePlan)
	if store.state.Nodes[qrspi.NodeHumanReviewOutline].Status != wruntime.NodeStatusBypassed {
		t.Fatalf(
			"human-review-outline state = %#v, want bypassed",
			store.state.Nodes[qrspi.NodeHumanReviewOutline],
		)
	}

	moveQRSPIIntegrationState(t, store, qrspi.NodeReviewImplementation)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeReviewImplementation,
		"complete",
		"reviews/implementation-review/review.md",
	)
	assertWaitingHuman(
		t,
		store,
		qrspi.NodeHumanReviewImplementation,
		qrspi.NodeReviewImplementation,
		"reviews/implementation-review/review.md",
	)
	if store.state.Nodes[qrspi.NodeHumanReviewImplementation].Status == wruntime.NodeStatusBypassed {
		t.Fatalf(
			"human-review-implementation state = %#v, want not bypassed",
			store.state.Nodes[qrspi.NodeHumanReviewImplementation],
		)
	}
}

func TestQRSPIWorkspaceStageRunsBeforeImplement(t *testing.T) {
	svc, store, runner := newQRSPIIntegrationHarness(t, nil)
	workspaceDir := t.TempDir()
	store.planningCwd = "/tmp/planning-checkout"
	moveQRSPIIntegrationState(t, store, qrspi.NodeReviewPlan)
	completeQRSPIIntegrationNode(
		t,
		svc,
		store,
		qrspi.NodeReviewPlan,
		"complete",
		"reviews/plan-review/review.md",
	)
	assertCurrentNode(t, store, runner, qrspi.NodeWorkspace)

	store.run = qRSPIIntegrationRun(qrspi.NodeWorkspace)
	store.assistantText = qRSPIIntegrationResultXMLWithWorkspace(
		qrspi.NodeWorkspace,
		"complete",
		"plan.md",
		workspaceDir,
	)
	if err := svc.OnRunComplete(
		t.Context(),
		conversation.RunResult{
			RunID:       store.run.ID,
			ThreadID:    store.run.ThreadID,
			HeadEntryID: "assistant-1",
		},
	); err != nil {
		t.Fatalf("OnRunComplete(workspace) error = %v", err)
	}
	assertCurrentNode(t, store, runner, qrspi.NodeImplement)
	if store.state.ExecutionCwd != workspaceDir {
		t.Fatalf("ExecutionCwd = %q, want %q", store.state.ExecutionCwd, workspaceDir)
	}
	if got := runner.starts[len(runner.starts)-1].Cwd; got != workspaceDir {
		t.Fatalf("implement start cwd = %q, want %q", got, workspaceDir)
	}
}

func TestQRSPIImplementationReviewFollowupUsesReviewDirContext(t *testing.T) {
	svc, store, runner := newQRSPIIntegrationHarness(t, nil)
	moveQRSPIIntegrationState(t, store, qrspi.NodeReviewImplementation)
	parentReview := "thoughts/creative-mode-agent/plans/2026-05-13_17-19-50_qrspi-auto-mode-review-looping/reviews/2026-05-14_19-00-00_qrspi-auto-mode-review-looping_implementation-review/review.md"
	followupPlan := "thoughts/creative-mode-agent/plans/2026-05-13_17-19-50_qrspi-auto-mode-review-looping/reviews/2026-05-14_19-00-00_qrspi-auto-mode-review-looping_implementation-review/plan.md"
	store.run = qRSPIIntegrationRun(qrspi.NodeReviewImplementation)
	store.assistantText = qRSPIIntegrationResultXMLWithArtifacts(
		qrspi.NodeReviewImplementation,
		"complete",
		wruntime.OutcomeNeedsFollowup,
		parentReview,
		"/q-question "+followupPlan,
		wruntime.ArtifactRef{Role: "followup-plan", Path: followupPlan},
	)
	if err := svc.OnRunComplete(
		context.Background(),
		conversation.RunResult{
			RunID:       store.run.ID,
			ThreadID:    store.run.ThreadID,
			HeadEntryID: "assistant-1",
		},
	); err != nil {
		t.Fatalf("OnRunComplete() error = %v", err)
	}
	assertCurrentNode(t, store, runner, qrspi.NodeQuestion)
	if len(store.state.Followups) != 1 {
		t.Fatalf("followups = %#v, want one active followup", store.state.Followups)
	}
	if got := store.state.Followups[0].FollowupPlanDir; got != strings.TrimSuffix(
		followupPlan,
		"/plan.md",
	) {
		t.Fatalf(
			"followup plan dir = %q, want %q",
			got,
			strings.TrimSuffix(followupPlan, "/plan.md"),
		)
	}
	assertLatestPromptContains(
		t,
		runner,
		"Implementation-review follow-up context is active",
		"Follow-up plan dir: "+strings.TrimSuffix(followupPlan, "/plan.md"),
		"Parent implementation review: "+parentReview,
	)
	if !hasWorkflowEvent(store.events, "workflow_followup_entered") {
		t.Fatalf("events = %#v, want followup entered", store.events)
	}
}

func TestQRSPIImplementationReviewFollowupReturnsToParentReview(t *testing.T) {
	svc, store, runner := newQRSPIIntegrationHarness(t, nil)
	parentReview := "thoughts/creative-mode-agent/plans/2026-05-13_17-19-50_qrspi-auto-mode-review-looping/reviews/2026-05-14_19-00-00_qrspi-auto-mode-review-looping_implementation-review/review.md"
	followupDir := "thoughts/creative-mode-agent/plans/2026-05-13_17-19-50_qrspi-auto-mode-review-looping/reviews/2026-05-14_19-00-00_qrspi-auto-mode-review-looping_implementation-review"
	store.state.Followups = []wruntime.FollowupContext{
		{
			ParentPlanDir:      "thoughts/creative-mode-agent/plans/2026-05-13_17-19-50_qrspi-auto-mode-review-looping",
			FollowupPlanDir:    followupDir,
			ParentReviewNodeID: qrspi.NodeReviewImplementation,
			ParentReviewPath:   parentReview,
		},
	}
	moveQRSPIIntegrationState(t, store, qrspi.NodeReviewImplementation)
	store.run = qRSPIIntegrationRun(qrspi.NodeReviewImplementation)
	store.assistantText = qRSPIIntegrationResultXMLWithArtifacts(
		qrspi.NodeReviewImplementation,
		"complete",
		wruntime.OutcomeReadyForHumanReview,
		followupDir+"/reviews/2026-05-14_20-00-00_followup_implementation-review/review.md",
		"/q-review "+parentReview,
	)
	if err := svc.OnRunComplete(
		context.Background(),
		conversation.RunResult{
			RunID:       store.run.ID,
			ThreadID:    store.run.ThreadID,
			HeadEntryID: "assistant-1",
		},
	); err != nil {
		t.Fatalf("OnRunComplete() error = %v", err)
	}
	assertCurrentNode(t, store, runner, qrspi.NodeReviewImplementation)
	if len(store.state.Followups) != 0 {
		t.Fatalf("followups = %#v, want popped", store.state.Followups)
	}
	if store.state.HumanGate != nil ||
		store.state.Status == wruntime.WorkspaceStatusWaitingHuman {
		t.Fatalf(
			"state = %#v, want returned to parent review without waiting human",
			store.state,
		)
	}
	if !hasWorkflowEvent(store.events, "workflow_followup_exited") {
		t.Fatalf("events = %#v, want followup exited", store.events)
	}
	if hasWorkflowEvent(store.events, "workflow_waiting_human") {
		t.Fatalf("events = %#v, want no stale waiting-human event", store.events)
	}
}

func TestQRSPIOutcomeRequiredArtifactsDoNotAdvance(t *testing.T) {
	for _, tc := range []struct {
		name     string
		node     wruntime.NodeID
		outcome  wruntime.ResultOutcome
		artifact string
		wantErr  string
	}{
		{
			name:     "review research questions",
			node:     qrspi.NodeReviewOutline,
			outcome:  wruntime.OutcomeNeedsReviewResearch,
			artifact: "reviews/outline-review/review.md",
			wantErr:  "requires followup questions artifact",
		},
		{
			name:     "implementation followup plan",
			node:     qrspi.NodeReviewImplementation,
			outcome:  wruntime.OutcomeNeedsFollowup,
			artifact: "reviews/implementation-review/review.md",
			wantErr:  "requires followup plan or questions artifact",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, runner := newQRSPIIntegrationHarness(t, nil)
			moveQRSPIIntegrationState(t, store, tc.node)
			store.run = qRSPIIntegrationRun(tc.node)
			store.assistantText = qRSPIIntegrationResultXMLWithArtifacts(
				tc.node,
				"complete",
				tc.outcome,
				tc.artifact,
				"/q-next "+tc.artifact,
			)

			err := svc.OnRunComplete(
				context.Background(),
				conversation.RunResult{
					RunID:       store.run.ID,
					ThreadID:    store.run.ThreadID,
					HeadEntryID: "assistant-1",
				},
			)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("OnRunComplete() error = %v, want %q", err, tc.wantErr)
			}
			if store.state.CurrentNodeID != tc.node || len(runner.starts) != 0 {
				t.Fatalf(
					"state/starts advanced unexpectedly: state=%#v starts=%#v",
					store.state,
					runner.starts,
				)
			}
		})
	}
}

func TestQRSPIArtifactMissingDoesNotAdvance(t *testing.T) {
	svc, store, runner := newQRSPIIntegrationHarness(t, nil)
	store.artifactExists = false
	store.run = qRSPIIntegrationRun(qrspi.NodeQuestion)
	store.assistantText = qRSPIIntegrationResultXML(
		qrspi.NodeQuestion,
		"complete",
		"missing.md",
	)

	err := svc.OnRunComplete(
		context.Background(),
		conversation.RunResult{
			RunID:       store.run.ID,
			ThreadID:    store.run.ThreadID,
			HeadEntryID: "assistant-1",
		},
	)
	if err == nil || !strings.Contains(err.Error(), "artifact") {
		t.Fatalf("OnRunComplete() error = %v, want missing artifact", err)
	}
	if store.state.CurrentNodeID != qrspi.NodeQuestion || len(runner.starts) != 0 {
		t.Fatalf(
			"state/starts advanced unexpectedly: state=%#v starts=%#v",
			store.state,
			runner.starts,
		)
	}
}

func TestQRSPIPlanningReviewQuestionsRoutesToResearchForReview(t *testing.T) {
	svc, store, runner := newQRSPIIntegrationHarness(t, nil)
	moveQRSPIIntegrationState(t, store, qrspi.NodeReviewOutline)
	store.artifactExistence = map[string]bool{
		"reviews/outline-review/review.md":             true,
		"reviews/outline-review/questions/followup.md": true,
	}
	store.run = qRSPIIntegrationRun(qrspi.NodeReviewOutline)
	store.assistantText = qRSPIIntegrationResultXMLWithArtifacts(
		qrspi.NodeReviewOutline,
		"complete",
		wruntime.OutcomeNeedsReviewResearch,
		"reviews/outline-review/review.md",
		"/skill:q-research-for-review reviews/outline-review/questions/followup.md",
		wruntime.ArtifactRef{
			Role: "followup-questions",
			Path: "reviews/outline-review/questions/followup.md",
		},
	)
	if err := svc.OnRunComplete(
		context.Background(),
		conversation.RunResult{
			RunID:       store.run.ID,
			ThreadID:    store.run.ThreadID,
			HeadEntryID: "assistant-1",
		},
	); err != nil {
		t.Fatalf("OnRunComplete() error = %v", err)
	}
	assertCurrentNode(t, store, runner, qrspi.NodeResearchForReviewOutline)
}

func TestQRSPIPlanningReviewResearchAddressLoops(t *testing.T) {
	for _, tc := range []struct {
		name         string
		reviewNode   wruntime.NodeID
		researchNode wruntime.NodeID
		addressNode  wruntime.NodeID
		readyOutcome wruntime.ResultOutcome
	}{
		{
			name:         "design",
			reviewNode:   qrspi.NodeReviewDesign,
			researchNode: qrspi.NodeResearchForReviewDesign,
			addressNode:  qrspi.NodeAddressReviewResearchDesign,
			readyOutcome: wruntime.OutcomeReadyForOutline,
		},
		{
			name:         "outline",
			reviewNode:   qrspi.NodeReviewOutline,
			researchNode: qrspi.NodeResearchForReviewOutline,
			addressNode:  qrspi.NodeAddressReviewResearchOutline,
			readyOutcome: wruntime.OutcomeReadyForHumanReview,
		},
		{
			name:         "plan",
			reviewNode:   qrspi.NodeReviewPlan,
			researchNode: qrspi.NodeResearchForReviewPlan,
			addressNode:  qrspi.NodeAddressReviewResearchPlan,
			readyOutcome: wruntime.OutcomeReadyForWorkspace,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, runner := newQRSPIIntegrationHarness(t, nil)
			moveQRSPIIntegrationState(t, store, tc.reviewNode)
			reviewPath := fmt.Sprintf("reviews/%s-review/review.md", tc.name)
			questionsPath := fmt.Sprintf(
				"reviews/%s-review/questions/followup.md",
				tc.name,
			)
			researchPath := fmt.Sprintf("reviews/%s-review/research/followup.md", tc.name)

			store.run = qRSPIIntegrationRun(tc.reviewNode)
			store.assistantText = qRSPIIntegrationResultXMLWithArtifacts(
				tc.reviewNode,
				"complete",
				wruntime.OutcomeNeedsReviewResearch,
				reviewPath,
				"/skill:q-research-for-review "+questionsPath,
				wruntime.ArtifactRef{Role: "followup-questions", Path: questionsPath},
			)
			if err := svc.OnRunComplete(
				context.Background(),
				conversation.RunResult{
					RunID:       store.run.ID,
					ThreadID:    store.run.ThreadID,
					HeadEntryID: "assistant-1",
				},
			); err != nil {
				t.Fatalf("OnRunComplete(review) error = %v", err)
			}
			assertCurrentNode(t, store, runner, tc.researchNode)
			assertLatestPromptContains(
				t,
				runner,
				"q-research-for-review",
				questionsPath,
				reviewPath,
			)

			store.run = qRSPIIntegrationRun(tc.researchNode)
			store.assistantText = qRSPIIntegrationResultXMLWithArtifacts(
				tc.researchNode,
				"complete",
				wruntime.OutcomeComplete,
				researchPath,
				"/skill:q-address-review-research "+reviewPath+" "+researchPath,
				wruntime.ArtifactRef{Role: "review", Path: reviewPath},
				wruntime.ArtifactRef{Role: "questions", Path: questionsPath},
			)
			if err := svc.OnRunComplete(
				context.Background(),
				conversation.RunResult{
					RunID:       store.run.ID,
					ThreadID:    store.run.ThreadID,
					HeadEntryID: "assistant-2",
				},
			); err != nil {
				t.Fatalf("OnRunComplete(research) error = %v", err)
			}
			assertCurrentNode(t, store, runner, tc.addressNode)
			assertLatestPromptContains(
				t,
				runner,
				"q-address-review-research",
				reviewPath,
				researchPath,
			)

			store.run = qRSPIIntegrationRun(tc.addressNode)
			store.assistantText = qRSPIIntegrationResultXMLWithArtifacts(
				tc.addressNode,
				"complete",
				wruntime.OutcomeComplete,
				reviewPath,
				"/q-review "+reviewPath,
			)
			if err := svc.OnRunComplete(
				context.Background(),
				conversation.RunResult{
					RunID:       store.run.ID,
					ThreadID:    store.run.ThreadID,
					HeadEntryID: "assistant-3",
				},
			); err != nil {
				t.Fatalf("OnRunComplete(address) error = %v", err)
			}
			assertCurrentNode(t, store, runner, tc.reviewNode)

			store.run = qRSPIIntegrationRun(tc.reviewNode)
			store.assistantText = qRSPIIntegrationResultXMLWithArtifacts(
				tc.reviewNode,
				"complete",
				tc.readyOutcome,
				reviewPath,
				"/q-next "+reviewPath,
			)
			if err := svc.OnRunComplete(
				context.Background(),
				conversation.RunResult{
					RunID:       store.run.ID,
					ThreadID:    store.run.ThreadID,
					HeadEntryID: "assistant-4",
				},
			); err != nil {
				t.Fatalf("OnRunComplete(final review) error = %v", err)
			}
		})
	}
}

func TestQRSPIValidatesRelatedArtifacts(t *testing.T) {
	svc, store, runner := newQRSPIIntegrationHarness(t, nil)
	moveQRSPIIntegrationState(t, store, qrspi.NodeReviewOutline)
	store.artifactExistence = map[string]bool{
		"reviews/outline-review/review.md":             true,
		"reviews/outline-review/questions/followup.md": false,
	}
	store.run = qRSPIIntegrationRun(qrspi.NodeReviewOutline)
	store.assistantText = qRSPIIntegrationResultXMLWithArtifacts(
		qrspi.NodeReviewOutline,
		"complete",
		wruntime.OutcomeNeedsReviewResearch,
		"reviews/outline-review/review.md",
		"/skill:q-research-for-review reviews/outline-review/questions/followup.md",
		wruntime.ArtifactRef{
			Role: "followup-questions",
			Path: "reviews/outline-review/questions/followup.md",
		},
	)
	err := svc.OnRunComplete(
		context.Background(),
		conversation.RunResult{
			RunID:       store.run.ID,
			ThreadID:    store.run.ThreadID,
			HeadEntryID: "assistant-1",
		},
	)
	if err == nil || !strings.Contains(err.Error(), "followup.md") {
		t.Fatalf("OnRunComplete() error = %v, want missing related artifact", err)
	}
	if store.state.CurrentNodeID != qrspi.NodeReviewOutline || len(runner.starts) != 0 {
		t.Fatalf(
			"state/starts advanced unexpectedly: state=%#v starts=%#v",
			store.state,
			runner.starts,
		)
	}
}

func TestQRSPIDisplayNextMismatchDoesNotDriveTransition(t *testing.T) {
	svc, store, runner := newQRSPIIntegrationHarness(t, nil)
	store.run = qRSPIIntegrationRun(qrspi.NodeQuestion)
	store.assistantText = qRSPIIntegrationResultXMLWithArtifacts(
		qrspi.NodeQuestion,
		"complete",
		wruntime.OutcomeComplete,
		"questions/runtime.md",
		"/q-implement plan.md",
	)
	if err := svc.OnRunComplete(
		context.Background(),
		conversation.RunResult{
			RunID:       store.run.ID,
			ThreadID:    store.run.ThreadID,
			HeadEntryID: "assistant-1",
		},
	); err != nil {
		t.Fatalf("OnRunComplete() error = %v", err)
	}
	assertCurrentNode(t, store, runner, qrspi.NodeResearch)
	if !hasWorkflowEvent(store.events, "workflow_display_next_mismatch") {
		t.Fatalf("events = %#v, want display-next mismatch warning", store.events)
	}
}

func TestQRSPIStageMismatchStartsCorrectionFlow(t *testing.T) {
	svc, store, runner := newQRSPIIntegrationHarness(t, nil)
	store.run = qRSPIIntegrationRun(qrspi.NodeQuestion)
	store.assistantText = qRSPIIntegrationResultXML(
		qrspi.NodeResearch,
		"complete",
		"research/runtime.md",
	)

	if err := svc.OnRunComplete(
		context.Background(),
		conversation.RunResult{
			RunID:       store.run.ID,
			ThreadID:    store.run.ThreadID,
			HeadEntryID: "assistant-1",
		},
	); err != nil {
		t.Fatalf("OnRunComplete() error = %v", err)
	}
	if len(runner.starts) != 1 || runner.starts[0].NodeID != qrspi.NodeQuestion ||
		runner.starts[0].Attempt != 1 ||
		runner.starts[0].Prompt == "" {
		t.Fatalf("starts = %#v, want correction for question", runner.starts)
	}
	if store.state.Status != wruntime.WorkspaceStatusRunning {
		t.Fatalf("status = %q, want running correction", store.state.Status)
	}
}

func newQRSPIIntegrationHarness(
	t *testing.T,
	policy []byte,
) (*Service, *fakeStore, *fakeRunner) {
	t.Helper()
	def, err := qrspi.Definition()
	if err != nil {
		t.Fatalf("qrspi.Definition() error = %v", err)
	}
	state, err := wruntime.InitialState(def, policy)
	if err != nil {
		t.Fatalf("InitialState() error = %v", err)
	}
	registry := wruntime.NewRegistry()
	if err := registry.Register(def); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	store := &fakeStore{state: state, artifactExists: true}
	runner := &fakeRunner{}
	return &Service{Definitions: registry, Store: store, Runner: runner}, store, runner
}

func completeQRSPIIntegrationNode(
	t *testing.T,
	svc *Service,
	store *fakeStore,
	node wruntime.NodeID,
	status, artifact string,
) {
	t.Helper()
	if store.state.CurrentNodeID != node {
		t.Fatalf("current node = %q, want completing %q", store.state.CurrentNodeID, node)
	}
	store.run = qRSPIIntegrationRun(node)
	if node == qrspi.NodeWorkspace {
		store.assistantText = qRSPIIntegrationResultXMLWithWorkspace(
			node,
			status,
			artifact,
			t.TempDir(),
		)
	} else {
		store.assistantText = qRSPIIntegrationResultXML(node, status, artifact)
	}
	if err := svc.OnRunComplete(
		context.Background(),
		conversation.RunResult{
			RunID:       store.run.ID,
			ThreadID:    store.run.ThreadID,
			HeadEntryID: "assistant-1",
			SessionPath: "/tmp/session.jsonl",
		},
	); err != nil {
		t.Fatalf("OnRunComplete(%s/%s) error = %v", node, status, err)
	}
}

func advanceQRSPIIntegrationGate(
	t *testing.T,
	svc *Service,
	store *fakeStore,
	runner *fakeRunner,
	want wruntime.NodeID,
) {
	t.Helper()
	before := len(runner.starts)
	if _, err := svc.AdvanceHumanGate(
		context.Background(),
		"workspace-1",
		"person@example.com",
	); err != nil {
		t.Fatalf("AdvanceHumanGate() error = %v", err)
	}
	if store.state.CurrentNodeID != want ||
		store.state.Status != wruntime.WorkspaceStatusIdle ||
		store.state.HumanGate != nil {
		t.Fatalf("state after advance = %#v, want current %s idle", store.state, want)
	}
	if len(runner.starts) != before+1 ||
		runner.starts[len(runner.starts)-1].NodeID != want {
		t.Fatalf("starts = %#v, want latest %s", runner.starts, want)
	}
}

func assertCurrentNode(
	t *testing.T,
	store *fakeStore,
	runner *fakeRunner,
	want wruntime.NodeID,
) {
	t.Helper()
	if store.state.CurrentNodeID != want ||
		store.state.Status != wruntime.WorkspaceStatusIdle {
		t.Fatalf("state = %#v, want current %s idle", store.state, want)
	}
	if len(runner.starts) == 0 || runner.starts[len(runner.starts)-1].NodeID != want {
		t.Fatalf("starts = %#v, want latest %s", runner.starts, want)
	}
}

func assertLatestPromptContains(t *testing.T, runner *fakeRunner, parts ...string) {
	t.Helper()
	if len(runner.starts) == 0 {
		t.Fatalf("starts = %#v, want a prompt", runner.starts)
	}
	prompt := runner.starts[len(runner.starts)-1].Prompt
	for _, part := range parts {
		if !strings.Contains(prompt, part) {
			t.Fatalf("prompt = %q, want to contain %q", prompt, part)
		}
	}
}

func assertWaitingHuman(
	t *testing.T,
	store *fakeStore,
	want, reviewContext wruntime.NodeID,
	artifact string,
) {
	t.Helper()
	if store.state.Status != wruntime.WorkspaceStatusWaitingHuman ||
		store.state.HumanGate == nil {
		t.Fatalf("state = %#v, want waiting human", store.state)
	}
	if store.state.HumanGate.To != want ||
		store.state.HumanGate.ReviewContextNodeID != reviewContext ||
		store.state.HumanGate.ReviewArtifact != artifact {
		t.Fatalf(
			"human gate = %#v, want to=%s context=%s artifact=%s",
			store.state.HumanGate,
			want,
			reviewContext,
			artifact,
		)
	}
}

func assertBypassedAndWaiting(
	t *testing.T,
	store *fakeStore,
	bypassed, humanReview, reviewContext wruntime.NodeID,
) {
	t.Helper()
	if store.state.Nodes[bypassed].Status != wruntime.NodeStatusBypassed {
		t.Fatalf(
			"node %s state = %#v, want bypassed",
			bypassed,
			store.state.Nodes[bypassed],
		)
	}
	assertWaitingHuman(
		t,
		store,
		humanReview,
		reviewContext,
		store.state.LastResult.PrimaryArtifact,
	)
}

func moveQRSPIIntegrationState(t *testing.T, store *fakeStore, node wruntime.NodeID) {
	t.Helper()
	store.state.CurrentNodeID = node
	store.state.PendingNextNodeID = node
	store.state.Status = wruntime.WorkspaceStatusIdle
}

func qRSPIIntegrationRun(node wruntime.NodeID) db.AgentRun {
	return db.AgentRun{
		ID:          "run-" + string(node),
		WorkspaceID: sql.NullString{String: "workspace-1", Valid: true},
		ThreadID:    "thread-1",
		SessionID:   sql.NullString{String: "session-1", Valid: true},
		WorkflowNodeID: sql.NullString{
			String: string(node),
			Valid:  true,
		},
	}
}

func qRSPIIntegrationResultXML(node wruntime.NodeID, status, artifact string) string {
	return qRSPIIntegrationResultXMLWithArtifacts(
		node,
		status,
		qRSPIIntegrationOutcome(node),
		artifact,
		fmt.Sprintf("/q-next %s", artifact),
	)
}

func qRSPIIntegrationResultXMLWithArtifacts(
	node wruntime.NodeID,
	status string,
	outcome wruntime.ResultOutcome,
	artifact string,
	next string,
	artifacts ...wruntime.ArtifactRef,
) string {
	return qRSPIIntegrationResultXMLWithWorkspaceAndArtifacts(
		node,
		status,
		outcome,
		artifact,
		next,
		"",
		artifacts...,
	)
}

func qRSPIIntegrationResultXMLWithWorkspace(
	node wruntime.NodeID,
	status string,
	artifact string,
	workspace string,
) string {
	return qRSPIIntegrationResultXMLWithWorkspaceAndArtifacts(
		node,
		status,
		qRSPIIntegrationOutcome(node),
		artifact,
		"/q-next "+artifact,
		workspace,
	)
}

func qRSPIIntegrationResultXMLWithWorkspaceAndArtifacts(
	node wruntime.NodeID,
	status string,
	outcome wruntime.ResultOutcome,
	artifact string,
	next string,
	workspace string,
	artifacts ...wruntime.ArtifactRef,
) string {
	var related strings.Builder
	if len(artifacts) > 0 {
		related.WriteString("\n  <artifacts>")
		for _, artifact := range artifacts {
			related.WriteString(fmt.Sprintf(
				"\n    <artifact role=\"%s\">%s</artifact>",
				artifact.Role,
				artifact.Path,
			))
		}
		related.WriteString("\n  </artifacts>")
	}
	workspaceXML := ""
	if strings.TrimSpace(workspace) != "" {
		workspaceXML = fmt.Sprintf("\n  <workspace>%s</workspace>", workspace)
	}
	return fmt.Sprintf(`<qrspi-result>
  <stage>%s</stage>
  <status>%s</status>
  <outcome>%s</outcome>%s
  <policy>
    <autoMode>false</autoMode>
    <enablePlanReviews>true</enablePlanReviews>
    <invalidResultRetryLimit>1</invalidResultRetryLimit>
  </policy>
  <summary>
    <plan-goal>Build Agent Chat-native generic workflow runtime; QRSPI first.</plan-goal>
    <stage-completed>Completed %s for runtime parity.</stage-completed>
    <key-decisions>Continue along the registered QRSPI graph.</key-decisions>
  </summary>
  <artifact>%s</artifact>%s
  <next>%s</next>
</qrspi-result>`, node, status, outcome, workspaceXML, node, artifact, related.String(), next)
}

func hasWorkflowEvent(events []wruntime.Event, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func mustQRSPIIntegrationPolicy(t *testing.T, policy qrspi.Policy) []byte {
	t.Helper()
	return []byte(
		fmt.Sprintf(
			`{"autoMode":%t,"enablePlanReviews":%t,"invalidResultRetryLimit":%d}`,
			policy.AutoMode,
			policy.EnablePlanReviews,
			policy.InvalidResultRetryLimit,
		),
	)
}

func qRSPIIntegrationOutcome(node wruntime.NodeID) wruntime.ResultOutcome {
	switch node {
	case qrspi.NodeReviewDesign:
		return wruntime.OutcomeReadyForOutline
	case qrspi.NodeReviewOutline, qrspi.NodeReviewImplementation:
		return wruntime.OutcomeReadyForHumanReview
	case qrspi.NodeReviewPlan:
		return wruntime.OutcomeReadyForWorkspace
	default:
		return wruntime.OutcomeComplete
	}
}
