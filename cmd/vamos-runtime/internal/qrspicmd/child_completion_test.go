package qrspicmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

func TestValidateHandoffArtifact(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(t *testing.T, state *ManagerState, source *ChildRunRef, result *wruntime.WorkflowResult)
		wantErr bool
	}{
		{name: "valid research handoff"},
		{
			name: "empty artifact",
			mutate: func(_ *testing.T, _ *ManagerState, _ *ChildRunRef, result *wruntime.WorkflowResult) {
				result.PrimaryArtifact = ""
			},
			wantErr: true,
		},
		{
			name: "missing file",
			mutate: func(_ *testing.T, _ *ManagerState, _ *ChildRunRef, result *wruntime.WorkflowResult) {
				result.PrimaryArtifact = "thoughts/example/handoffs/missing.md"
			},
			wantErr: true,
		},
		{
			name: "directory",
			mutate: func(t *testing.T, _ *ManagerState, source *ChildRunRef, result *wruntime.WorkflowResult) {
				path := filepath.Join(source.Cwd, "thoughts", "example", "handoffs", "directory")
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
				result.PrimaryArtifact = "thoughts/example/handoffs/directory"
			},
			wantErr: true,
		},
		{
			name: "lexical escape",
			mutate: func(t *testing.T, _ *ManagerState, source *ChildRunRef, result *wruntime.WorkflowResult) {
				outside := filepath.Join(filepath.Dir(source.Cwd), "outside.md")
				writeHandoffFile(t, outside, "research", "in_progress")
				result.PrimaryArtifact = "../outside.md"
			},
			wantErr: true,
		},
		{
			name: "absolute outside path",
			mutate: func(t *testing.T, _ *ManagerState, source *ChildRunRef, result *wruntime.WorkflowResult) {
				outside := filepath.Join(filepath.Dir(source.Cwd), "absolute-outside.md")
				writeHandoffFile(t, outside, "research", "in_progress")
				result.PrimaryArtifact = outside
			},
			wantErr: true,
		},
		{
			name: "artifact symlink escape",
			mutate: func(t *testing.T, _ *ManagerState, source *ChildRunRef, result *wruntime.WorkflowResult) {
				outside := filepath.Join(filepath.Dir(source.Cwd), "symlink-outside.md")
				writeHandoffFile(t, outside, "research", "in_progress")
				link := filepath.Join(source.Cwd, "thoughts", "example", "handoffs", "link.md")
				if err := os.Symlink(outside, link); err != nil {
					t.Fatal(err)
				}
				result.PrimaryArtifact = "thoughts/example/handoffs/link.md"
			},
			wantErr: true,
		},
		{
			name: "handoffs directory symlink escape",
			mutate: func(t *testing.T, _ *ManagerState, source *ChildRunRef, result *wruntime.WorkflowResult) {
				handoffs := filepath.Join(source.Cwd, "thoughts", "example", "handoffs")
				if err := os.RemoveAll(handoffs); err != nil {
					t.Fatal(err)
				}
				outside := filepath.Join(filepath.Dir(source.Cwd), "external-handoffs")
				if err := os.MkdirAll(outside, 0o755); err != nil {
					t.Fatal(err)
				}
				writeHandoffFile(t, filepath.Join(outside, "research.md"), "research", "in_progress")
				if err := os.Symlink(outside, handoffs); err != nil {
					t.Fatal(err)
				}
				result.PrimaryArtifact = "thoughts/example/handoffs/research.md"
			},
			wantErr: true,
		},
		{
			name: "implementation copy",
			mutate: func(t *testing.T, _ *ManagerState, source *ChildRunRef, _ *wruntime.WorkflowResult) {
				implementation := filepath.Join(filepath.Dir(source.Cwd), "implementation")
				path := filepath.Join(implementation, "thoughts", "example", "handoffs", "research.md")
				writeHandoffFile(t, path, "research", "in_progress")
				source.Cwd = implementation
			},
		},
		{
			name: "mismatched stage",
			mutate: func(t *testing.T, _ *ManagerState, source *ChildRunRef, _ *wruntime.WorkflowResult) {
				path := filepath.Join(source.Cwd, "thoughts", "example", "handoffs", "research.md")
				writeHandoffFile(t, path, "design", "in_progress")
			},
			wantErr: true,
		},
		{
			name: "wrong status",
			mutate: func(t *testing.T, _ *ManagerState, source *ChildRunRef, _ *wruntime.WorkflowResult) {
				path := filepath.Join(source.Cwd, "thoughts", "example", "handoffs", "research.md")
				writeHandoffFile(t, path, "research", "complete")
			},
			wantErr: true,
		},
		{
			name: "nonempty outcome",
			mutate: func(_ *testing.T, _ *ManagerState, _ *ChildRunRef, result *wruntime.WorkflowResult) {
				result.Outcome = wruntime.OutcomeComplete
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, source, result := handoffArtifactFixture(t)
			if tt.mutate != nil {
				tt.mutate(t, &state, &source, &result)
			}
			handoff, err := validateHandoffArtifact(state, source, result)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("validateHandoffArtifact() = %+v, want error", handoff)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateHandoffArtifact() error = %v", err)
			}
			if handoff.Stage != qrspi.NodeResearch || handoff.Status != "in_progress" || !filepath.IsAbs(handoff.Path) {
				t.Fatalf("handoff = %+v", handoff)
			}
		})
	}
}

func TestDeriveChildLaunchIntentUsesGraphDecisionAndManagerPolicy(t *testing.T) {
	state, source, handoffResult := handoffArtifactFixture(t)
	state.Workflow = testWorkflowState(t, qrspi.NodeResearch, nil)
	def, err := Definition()
	if err != nil {
		t.Fatal(err)
	}
	decision, err := wruntime.DecideTransition(def, state.Workflow, handoffResult)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := deriveChildLaunchIntent(state, source, handoffResult, decision)
	if err != nil {
		t.Fatalf("deriveChildLaunchIntent() error = %v", err)
	}
	if intent.Kind != ChildLaunchResumeHandoff || intent.NodeID != qrspi.NodeResearch ||
		intent.SkillPath != ".pi/skills/q-resume/SKILL.md" || intent.Cwd != source.Cwd ||
		intent.DeliveryID == "" || !filepath.IsAbs(intent.PrimaryArtifact) {
		t.Fatalf("intent = %+v", intent)
	}

	forged := decision
	forged.StartNext = false
	if _, err := deriveChildLaunchIntent(state, source, handoffResult, forged); err == nil {
		t.Fatal("forged decision accepted")
	}

	complete := handoffResult
	complete.Status = wruntime.StatusComplete
	complete.Outcome = wruntime.OutcomeComplete
	complete.DisplayNext = "read .pi/skills/q-resume/SKILL.md"
	completeDecision, err := wruntime.DecideTransition(def, state.Workflow, complete)
	if err != nil {
		t.Fatal(err)
	}
	normal, err := deriveChildLaunchIntent(state, source, complete, completeDecision)
	if err != nil {
		t.Fatalf("normal deriveChildLaunchIntent() error = %v", err)
	}
	if normal.Kind != ChildLaunchNormal || normal.NodeID != qrspi.NodeDesign ||
		normal.SkillPath != ".pi/skills/q-design/SKILL.md" {
		t.Fatalf("normal intent = %+v", normal)
	}

	blocked := handoffResult
	blocked.Status = wruntime.StatusBlocked
	blocked.Outcome = ""
	blocked.DisplayNext = complete.DisplayNext
	forgedBlocked := decision
	if _, err := deriveChildLaunchIntent(state, source, blocked, forgedBlocked); err == nil {
		t.Fatal("blocked result created resume intent")
	}
}

func TestDeriveChildLaunchIntentDiscussPolicyDoesNotLaunch(t *testing.T) {
	state, source, result := handoffArtifactFixture(t)
	policy, err := json.Marshal(qrspi.Policy{
		AdvanceMode:             qrspi.AdvanceModeDiscuss,
		EnablePlanReviews:       true,
		InvalidResultRetryLimit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	state.Workflow = testWorkflowState(t, qrspi.NodeResearch, policy)
	def, err := Definition()
	if err != nil {
		t.Fatal(err)
	}
	decision, err := wruntime.DecideTransition(def, state.Workflow, result)
	if err != nil {
		t.Fatal(err)
	}
	if decision.StartNext {
		t.Fatalf("decision = %+v, want no start", decision)
	}
	if _, err := deriveChildLaunchIntent(state, source, result, decision); err == nil {
		t.Fatal("discuss decision created launch intent")
	}
}

func TestExistingHandoffContinuationMatchesSourceAndDelivery(t *testing.T) {
	state := ManagerState{ActiveChild: &ChildRunRef{
		ID:                     "replacement",
		LaunchKind:             ChildLaunchResumeHandoff,
		ContinuationOf:         "source",
		ContinuationDeliveryID: "delivery",
		ContinuationArtifact:   "/tmp/handoff.md",
	}}
	got, ok := existingHandoffContinuation(state, "source", "delivery")
	if !ok || got.ID != "replacement" {
		t.Fatalf("existingHandoffContinuation() = %+v, %t", got, ok)
	}
	if _, ok := existingHandoffContinuation(state, "source", "different"); ok {
		t.Fatal("mismatched delivery unexpectedly matched")
	}
}

func handoffArtifactFixture(t *testing.T) (ManagerState, ChildRunRef, wruntime.WorkflowResult) {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	plan := filepath.Join(repo, "thoughts", "example")
	path := filepath.Join(plan, "handoffs", "research.md")
	writeHandoffFile(t, path, "research", "in_progress")
	return ManagerState{
			RepoID:            repo,
			CanonicalPlanDir:  plan,
			SourceCwd:         repo,
			ImplementationCwd: filepath.Join(root, "implementation"),
		}, ChildRunRef{
			ID:         "child-1",
			Stage:      "research",
			Cwd:        repo,
			Generation: 1,
		}, wruntime.WorkflowResult{
			WorkflowType:    string(qrspi.AgentChatWorkflowType),
			SourceNodeID:    qrspi.NodeResearch,
			Status:          wruntime.StatusHandoff,
			PrimaryArtifact: "thoughts/example/handoffs/research.md",
			Summary:         "checkpoint",
			Evidence:        wruntime.EvidenceRef{RunID: "run-1"},
		}
}

func writeHandoffFile(t *testing.T, path, stage, status string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "---\nstage: "+stage+"\nstatus: "+status+"\n---\n\n# Handoff\n")
}

func TestChildCompleteInvalidHandoffArtifactWaitsForManager(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	plan := filepath.Join(repo, "thoughts", "example")
	if err := os.MkdirAll(filepath.Join(plan, "handoffs"), 0o755); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(dir, "state.json")
	sessionDir := filepath.Join(dir, "sessions")
	sessionPath := writePiSession(
		t,
		sessionDir,
		"session.jsonl",
		"session-1",
		repo,
		assistantLine(testResultYAML(
			"research",
			"handoff",
			"",
			"thoughts/example/handoffs/missing.md",
			"",
		)),
	)
	state := ManagerState{
		RepoID:           repo,
		CanonicalPlanDir: plan,
		SourceCwd:        repo,
		ManagerPaneID:    "%parent",
		Workflow:         testWorkflowState(t, qrspi.NodeResearch, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "research",
			Cwd:                  repo,
			SessionID:            "session-1",
			SessionDir:           sessionDir,
			SessionPath:          sessionPath,
			ValidationStatusPath: filepath.Join(dir, "validation-status.json"),
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)
	status, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: &recordingTmux{}},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete() error = %v", err)
	}
	if !status.Validated || !status.ManagerNeeded || status.ActionCard == nil ||
		status.ActionCard.Kind != ActionInvalidHandoffArtifact {
		t.Fatalf("status = %+v", status)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Workflow.CurrentNodeID != qrspi.NodeResearch ||
		loaded.ActiveChild == nil || loaded.ActiveChild.LifecycleStatus != "awaiting_manager" ||
		loaded.LastActionCard == nil || loaded.LastActionCard.Kind != ActionInvalidHandoffArtifact {
		t.Fatalf("loaded state = %+v", loaded)
	}
}

func TestChildCompleteHandoffLaunchesFreshSameStageResumeChild(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	plan := filepath.Join(repo, "thoughts", "example")
	handoffPath := filepath.Join(plan, "handoffs", "research.md")
	writeHandoffFile(t, handoffPath, "research", "in_progress")
	stateFile := filepath.Join(dir, "state.json")
	sessionDir := filepath.Join(dir, "sessions")
	validationPath := filepath.Join(dir, "runs", "source", "validation-status.json")
	sessionPath := writePiSession(
		t,
		sessionDir,
		"session.jsonl",
		"session-1",
		repo,
		assistantLine(testResultYAML(
			"research",
			"handoff",
			"",
			"thoughts/example/handoffs/research.md",
			"",
		)),
	)
	state := ManagerState{
		RepoID:           repo,
		CanonicalPlanDir: plan,
		SourceCwd:        repo,
		ManagerPaneID:    "%parent",
		Workflow:         testWorkflowState(t, qrspi.NodeResearch, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "source-child",
			Stage:                "research",
			Cwd:                  repo,
			TmuxPaneID:           "%old",
			SessionID:            "session-1",
			SessionDir:           sessionDir,
			SessionPath:          sessionPath,
			ValidationStatusPath: validationPath,
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)
	runner := &fakeChildRunner{panes: []string{"%new"}}
	tmux := &recordingTmux{}
	status, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "source-child"},
		deps{Clock: func() time.Time { return time.Unix(200, 456) }, Runner: runner, Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete error = %v", err)
	}
	if !status.Validated || status.ManagerNeeded || !status.ContinuationStarted ||
		status.Reason != "handoff_auto_resumed" || status.Wake.Mode != "deliver" ||
		status.NextChild.Stage != "research" ||
		status.NextChild.Skill != ".pi/skills/q-resume/SKILL.md" {
		t.Fatalf("status = %+v", status)
	}
	if len(runner.started) != 1 || runner.started[0].Stage != "research" ||
		runner.started[0].Cwd != repo {
		t.Fatalf("runner starts = %+v", runner.started)
	}
	prompt, err := os.ReadFile(runner.started[0].PromptFile)
	if err != nil {
		t.Fatal(err)
	}
	realHandoffPath, err := filepath.EvalSymlinks(handoffPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"2. .pi/skills/q-resume/SKILL.md",
		"4. " + realHandoffPath,
		"Current node: research",
		"Previous QRSPI result",
		"qrspi_result:",
	} {
		if !strings.Contains(string(prompt), want) {
			t.Fatalf("resume prompt missing %q:\n%s", want, prompt)
		}
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Workflow.CurrentNodeID != qrspi.NodeResearch ||
		loaded.ActiveChild == nil || loaded.ActiveChild.ID == "source-child" ||
		loaded.ActiveChild.Stage != "research" ||
		loaded.ActiveChild.LaunchKind != ChildLaunchResumeHandoff ||
		loaded.ActiveChild.ContinuationOf != "source-child" ||
		loaded.ActiveChild.ContinuationDeliveryID != status.DeliveryID ||
		loaded.PendingCleanupChild == nil || loaded.PendingCleanupChild.ID != "source-child" {
		t.Fatalf("loaded state = %+v", loaded)
	}
	if len(tmux.kills) != 0 {
		t.Fatalf("old pane killed before deferred notification cleanup: %#v", tmux.kills)
	}
	var disk ChildCompletionStatus
	data, err := os.ReadFile(validationPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatal(err)
	}
	if !disk.ContinuationStarted || disk.DeliveryID != status.DeliveryID {
		t.Fatalf("source validation status = %+v", disk)
	}

	duplicate, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "source-child"},
		deps{Clock: func() time.Time { return time.Unix(201, 456) }, Runner: runner, Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("duplicate RunChildComplete error = %v", err)
	}
	if !duplicate.ContinuationStarted || duplicate.DeliveryID != status.DeliveryID ||
		duplicate.Wake.Mode != "suppress" || len(runner.started) != 1 {
		t.Fatalf("duplicate status = %+v; starts = %d", duplicate, len(runner.started))
	}
}

func TestChildCompleteWritesValidatedStatus(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	sessionDir := filepath.Join(dir, "sessions")
	validationPath := filepath.Join(dir, "runs", "child-1", "validation-status.json")
	sessionPath := writePiSession(
		t,
		sessionDir,
		"session.jsonl",
		"session-1",
		filepath.Join(dir, "repo"),
		assistantLine(
			testResultYAML(
				"review-outline",
				"complete",
				"complete",
				"thoughts/example/reviews/outline/review.md",
				"",
			),
		),
	)
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		ManagerPaneID:    "%parent",
		Workflow:         testWorkflowState(t, qrspi.NodeReviewOutline, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "review-outline",
			Cwd:                  filepath.Join(dir, "repo"),
			TmuxPaneID:           "%9",
			SessionID:            "session-1",
			SessionDir:           sessionDir,
			SessionPath:          sessionPath,
			ValidationStatusPath: validationPath,
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)

	var out strings.Builder
	tmux := &recordingTmux{}
	status, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1", Output: "json"},
		deps{Tmux: tmux},
		&out,
	)
	if err != nil {
		t.Fatalf("RunChildComplete error = %v", err)
	}
	if !status.Validated || status.Result.Outcome != "ready-for-plan" ||
		status.Wake.Mode != "deliver" {
		t.Fatalf("status = %+v", status)
	}
	if len(status.Normalizations) != 1 ||
		status.Normalizations[0].Canonical != "ready-for-plan" {
		t.Fatalf("normalizations = %+v", status.Normalizations)
	}
	var disk ChildCompletionStatus
	data, err := os.ReadFile(validationPath)
	if err != nil {
		t.Fatalf("read validation status: %v", err)
	}
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatalf("decode validation status: %v", err)
	}
	if !disk.Validated || disk.DeliveryID == "" ||
		disk.Result.Outcome != "ready-for-plan" {
		t.Fatalf("disk status = %+v", disk)
	}
	if !strings.Contains(out.String(), `"validated": true`) {
		t.Fatalf("json output = %q", out.String())
	}

	loaded := loadManagerState(t, stateFile)
	if loaded.Workflow.CurrentNodeID != qrspi.NodeReviewOutline {
		t.Fatalf(
			"child-complete advanced workflow to %q; want still review-outline",
			loaded.Workflow.CurrentNodeID,
		)
	}
	if loaded.ActiveChild == nil || loaded.ActiveChild.LifecycleStatus != "completed" ||
		loaded.ActiveChild.LastDeliveryID == "" {
		t.Fatalf("loaded active child = %+v", loaded.ActiveChild)
	}

	status, err = RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete duplicate error = %v", err)
	}
	if status.DeliveryID != loaded.ActiveChild.LastDeliveryID ||
		status.Wake.Mode != "suppress" ||
		status.Wake.Reason != "duplicate_delivery" {
		t.Fatalf("duplicate status = %+v", status)
	}
}

func TestChildCompleteQueuesValidatedWakeWhileManagerCompacting(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	sessionDir := filepath.Join(dir, "sessions")
	validationPath := filepath.Join(dir, "runs", "child-1", "validation-status.json")
	sessionPath := writePiSession(
		t,
		sessionDir,
		"session.jsonl",
		"session-1",
		filepath.Join(dir, "repo"),
		assistantLine(
			testResultYAML(
				"review-plan",
				"complete",
				"complete",
				"thoughts/example/reviews/plan/review.md",
				"",
			),
		),
	)
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		ManagerPaneID:    "%parent",
		Delivery: ManagerDeliveryState{
			Status:        "compacting",
			ManagerPaneID: "%parent",
		},
		Workflow: testWorkflowState(t, qrspi.NodeReviewPlan, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "review-plan",
			Cwd:                  filepath.Join(dir, "repo"),
			TmuxPaneID:           "%9",
			SessionID:            "session-1",
			SessionDir:           sessionDir,
			SessionPath:          sessionPath,
			ValidationStatusPath: validationPath,
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)

	tmux := &recordingTmux{}
	status, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete error = %v", err)
	}
	if !status.Validated || status.Wake.Mode != "queue" ||
		status.Wake.Reason != "manager_compacting" {
		t.Fatalf("status = %+v, want validated queued wake", status)
	}
	if len(tmux.pastes) != 0 || len(tmux.keys) != 0 {
		t.Fatalf(
			"tmux pastes=%#v keys=%#v, want no parent paste while compacting",
			tmux.pastes,
			tmux.keys,
		)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Delivery.QueuedWake == nil ||
		loaded.Delivery.QueuedWake.DeliveryID != status.DeliveryID {
		t.Fatalf(
			"loaded delivery = %+v, want queued wake %q",
			loaded.Delivery,
			status.DeliveryID,
		)
	}
	var disk ChildCompletionStatus
	data, err := os.ReadFile(validationPath)
	if err != nil {
		t.Fatalf("read validation status: %v", err)
	}
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatalf("decode validation status: %v", err)
	}
	if disk.Wake.Mode != "queue" || disk.Wake.Reason != "manager_compacting" {
		t.Fatalf("disk wake = %+v, want queued manager_compacting", disk.Wake)
	}
}

func TestLenientPositiveOutcomeEndToEnd(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	repo := filepath.Join(dir, "repo")
	sessionDir := filepath.Join(dir, "sessions")
	sessionPath := writePiSession(
		t,
		sessionDir,
		"session.jsonl",
		"session-1",
		repo,
		assistantLine(
			testResultYAML(
				"review-plan",
				"complete",
				"complete",
				"thoughts/example/reviews/plan/review.md",
				"",
			),
		),
	)
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		ManagerPaneID:    "%parent",
		Workflow:         testWorkflowState(t, qrspi.NodeReviewPlan, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "review-plan",
			Cwd:                  repo,
			SessionID:            "session-1",
			SessionDir:           sessionDir,
			SessionPath:          sessionPath,
			ValidationStatusPath: filepath.Join(dir, "validation-status.json"),
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)

	status, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: &recordingTmux{}},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete error = %v", err)
	}
	if !status.Validated || status.Result.Outcome != "ready-for-workspace" ||
		len(status.Normalizations) != 1 {
		t.Fatalf("status = %+v", status)
	}
	if status.Normalizations[0].Original != "complete" ||
		status.Normalizations[0].Canonical != "ready-for-workspace" {
		t.Fatalf("normalization = %+v", status.Normalizations[0])
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Workflow.CurrentNodeID != qrspi.NodeReviewPlan {
		t.Fatalf(
			"child-complete advanced workflow to %q; want still review-plan",
			loaded.Workflow.CurrentNodeID,
		)
	}
}

func TestChildCompleteManagerAwareReviewPlanNormalization(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	sessionDir := filepath.Join(dir, "sessions")
	sessionPath := writePiSession(
		t,
		sessionDir,
		"session.jsonl",
		"session-1",
		filepath.Join(dir, "repo"),
		assistantLine(
			testResultYAML(
				"review-plan",
				"complete",
				"complete",
				"thoughts/example/reviews/plan/review.md",
				"",
			),
		),
	)
	state := ManagerState{
		CanonicalPlanDir: filepath.Join(
			dir,
			"thoughts",
			"plan",
			"reviews",
			"impl-review",
		),
		ImplementationCwd: filepath.Join(dir, "repo"),
		ManagerPaneID:     "%parent",
		Workflow:          testWorkflowState(t, qrspi.NodeReviewPlan, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "review-plan",
			Cwd:                  filepath.Join(dir, "repo"),
			SessionID:            "session-1",
			SessionDir:           sessionDir,
			SessionPath:          sessionPath,
			ValidationStatusPath: filepath.Join(dir, "validation-status.json"),
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)

	status, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: &recordingTmux{}},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete error = %v", err)
	}
	if status.Result.Outcome != "ready-for-implement" || len(status.Normalizations) != 1 {
		t.Fatalf("status = %+v", status)
	}
}

func TestChildCompleteProviderContextErrorDeliversManagerWake(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	repo := filepath.Join(dir, "repo")
	sessionDir := filepath.Join(dir, "sessions")
	validationPath := filepath.Join(dir, "runs", "child-1", "validation-status.json")
	sessionPath := filepath.Join(sessionDir, "session.jsonl")
	writeSessionWithBlockedResultThenProviderError(t, sessionPath, "session-1", repo)
	oldDeliveryID := "child-1:1:verify:blocked::thoughts/example/verify.md"
	if err := writeValidationStatus(validationPath, ChildCompletionStatus{
		Validated:     true,
		ChildID:       "child-1",
		DeliveryID:    oldDeliveryID,
		ManagerNeeded: true,
		Result: ChildCompletionResult{
			Stage:        "verify",
			Status:       "blocked",
			Artifact:     "thoughts/example/verify.md",
			PlanGoal:     "stale plan goal",
			KeyDecisions: "stale key decisions",
		},
	}); err != nil {
		t.Fatalf("write prior validation status: %v", err)
	}
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		ManagerPaneID:    "%parent",
		Delivery: ManagerDeliveryState{
			LastDeliveryID: oldDeliveryID,
		},
		Workflow: testWorkflowState(t, qrspi.NodeVerify, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "verify",
			Cwd:                  repo,
			TmuxPaneID:           "%9",
			SessionID:            "session-1",
			SessionDir:           sessionDir,
			SessionPath:          sessionPath,
			ValidationStatusPath: validationPath,
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)

	tmux := &recordingTmux{}
	status, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1", Output: "json"},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete error = %v", err)
	}
	if status.Validated || !status.ManagerNeeded || status.RetryExhausted ||
		status.Result.Status != ActionChildContextExhausted ||
		status.TerminalEvidence == nil || !status.TerminalEvidence.ContextWindowError ||
		!strings.Contains(status.DeliveryID, ":provider_context_error:") ||
		status.Wake.Mode != "deliver" {
		t.Fatalf("status = %+v", status)
	}
	if status.Result.Artifact != "thoughts/example/verify.md" ||
		status.Result.PlanGoal != "stale plan goal" {
		t.Fatalf("status result did not preserve prior context = %+v", status.Result)
	}
	if len(tmux.pastes) != 1 ||
		!strings.Contains(tmux.pastes[0].text, "q_manager_child_wake:") ||
		!strings.Contains(tmux.pastes[0].text, "terminal_evidence:") ||
		!strings.Contains(tmux.pastes[0].text, "context_window_error: true") ||
		!strings.Contains(tmux.pastes[0].text, "evidence_id:") ||
		!strings.Contains(tmux.pastes[0].text, "Your input exceeds the context window") {
		t.Fatalf("pastes = %#v", tmux.pastes)
	}
	var disk ChildCompletionStatus
	data, err := os.ReadFile(validationPath)
	if err != nil {
		t.Fatalf("read validation status: %v", err)
	}
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatalf("decode validation status: %v", err)
	}
	if disk.TerminalEvidence == nil || !disk.TerminalEvidence.ContextWindowError ||
		disk.DeliveryID != status.DeliveryID {
		t.Fatalf("disk status = %+v", disk)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.LastActionCard == nil ||
		loaded.LastActionCard.Kind != ActionChildContextExhausted ||
		loaded.ActiveChild == nil ||
		loaded.ActiveChild.LifecycleStatus != "awaiting_manager" ||
		loaded.Delivery.LastDeliveryID != status.DeliveryID {
		t.Fatalf("loaded state = %+v", loaded)
	}
}

func TestChildCompleteProviderContextErrorSuppressesSameEvidenceDuplicate(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	repo := filepath.Join(dir, "repo")
	sessionDir := filepath.Join(dir, "sessions")
	sessionPath := filepath.Join(sessionDir, "session.jsonl")
	writeSessionWithBlockedResultThenProviderError(t, sessionPath, "session-1", repo)
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		ManagerPaneID:    "%parent",
		Workflow:         testWorkflowState(t, qrspi.NodeVerify, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "verify",
			Cwd:                  repo,
			SessionID:            "session-1",
			SessionDir:           sessionDir,
			SessionPath:          sessionPath,
			ValidationStatusPath: filepath.Join(dir, "validation-status.json"),
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)
	tmux := &recordingTmux{}
	first, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("first RunChildComplete error = %v", err)
	}
	second, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("second RunChildComplete error = %v", err)
	}
	if first.DeliveryID == "" || second.DeliveryID != first.DeliveryID ||
		second.Wake.Mode != "suppress" || second.Wake.Reason != "duplicate_delivery" ||
		len(tmux.pastes) != 1 {
		t.Fatalf("first=%+v second=%+v pastes=%#v", first, second, tmux.pastes)
	}
}

func TestTerminalEvidenceRefreshDoesNotReturnEarlyForOlderResult(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	sessionDir := filepath.Join(dir, "sessions")
	sessionPath := filepath.Join(sessionDir, "session.jsonl")
	writeSessionTestFile(t, sessionPath, strings.Join([]string{
		sessionHeader("session-1", repo),
		assistantLine(
			testResultYAML(
				"verify",
				"blocked",
				"",
				"thoughts/example/verify.md",
				"stale blocked",
			),
		),
	}, "\n")+"\n")
	state := ManagerState{ActiveChild: &ChildRunRef{
		ID:          "child-1",
		Stage:       "verify",
		Cwd:         repo,
		SessionID:   "session-1",
		SessionDir:  sessionDir,
		SessionPath: sessionPath,
	}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(25 * time.Millisecond)
		file, err := os.OpenFile(sessionPath, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer file.Close()
		_, _ = file.WriteString(
			providerContextErrorLine(
				"Codex error: Your input exceeds the context window of this model.",
			) + "\n",
		)
	}()
	evidence, ok, err := terminalEvidenceForActiveChildWithRefresh(state)
	<-done
	if err != nil || !ok || !evidence.ContextWindowError || evidence.Line != 3 {
		t.Fatalf("evidence=%+v ok=%v err=%v", evidence, ok, err)
	}
}

func TestChildCompleteInvalidResultSuppressesThenExhausts(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	donePath := filepath.Join(dir, "done")
	sessionPath := writePiSession(
		t,
		filepath.Join(dir, "sessions"),
		"session.jsonl",
		"session-1",
		filepath.Join(dir, "repo"),
		assistantLine("not yaml"),
	)
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		ManagerPaneID:    "%parent",
		Workflow:         testWorkflowState(t, qrspi.NodeDesign, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "design",
			Cwd:                  filepath.Join(dir, "repo"),
			TmuxPaneID:           "%9",
			SessionID:            "session-1",
			SessionDir:           filepath.Join(dir, "sessions"),
			SessionPath:          sessionPath,
			DonePath:             donePath,
			ValidationStatusPath: filepath.Join(dir, "validation-status.json"),
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)
	writeFile(t, donePath, "")
	tmux := &recordingTmux{}
	status, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete retry error = %v", err)
	}
	if status.Validated || status.ManagerNeeded || status.Wake.Mode != "suppress" ||
		status.Reason != "retryable_invalid_result" {
		t.Fatalf("retry status = %+v", status)
	}
	if len(tmux.pastes) != 1 {
		t.Fatalf("pastes = %#v, want reprompt", tmux.pastes)
	}

	status, err = RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete exhausted error = %v", err)
	}
	if !status.ManagerNeeded || !status.RetryExhausted ||
		status.Result.Status != "invalid_result" ||
		status.Wake.Mode != "deliver" {
		t.Fatalf("exhausted status = %+v", status)
	}
}
