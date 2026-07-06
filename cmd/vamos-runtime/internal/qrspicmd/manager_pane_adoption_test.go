package qrspicmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestManagerPaneAdoptionExplicitRebindsLiveDifferentParent(t *testing.T) {
	state := ManagerState{
		ManagerPaneID: "%old",
		Delivery:      ManagerDeliveryState{ManagerPaneID: "%old"},
	}
	got, err := ResolveManagerPaneAdoption(t.Context(), state, ManagerPaneAdoptionOptions{
		StateFile:    "state.json",
		Command:      ManagerPaneAdoptionContinue,
		ExplicitPane: "%new",
		CurrentPane:  "%env",
	}, deps{Tmux: &recordingTmux{}})
	if err != nil {
		t.Fatal(err)
	}
	if got.ActionCard != nil || !got.Changed || got.State.ManagerPaneID != "%new" ||
		got.State.Delivery.ManagerPaneID != "%new" ||
		got.Reason != "explicit_manager_pane" {
		t.Fatalf("adoption = %+v", got)
	}
}

func TestManagerPaneAdoptionCurrentPaneWhenStoredBlank(t *testing.T) {
	got, err := ResolveManagerPaneAdoption(
		t.Context(),
		ManagerState{},
		ManagerPaneAdoptionOptions{
			StateFile:   "state.json",
			Command:     ManagerPaneAdoptionContinue,
			CurrentPane: "%parent",
		},
		deps{Tmux: &recordingTmux{}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Changed || got.State.ManagerPaneID != "%parent" || got.ActionCard != nil {
		t.Fatalf("adoption = %+v", got)
	}
}

func TestManagerPaneAdoptionCurrentPaneWhenStoredDead(t *testing.T) {
	state := ManagerState{
		ManagerPaneID: "%dead",
		Delivery:      ManagerDeliveryState{ManagerPaneID: "%dead"},
	}
	tmux := &recordingTmux{missingPanes: map[string]bool{"%dead": true}}
	got, err := ResolveManagerPaneAdoption(t.Context(), state, ManagerPaneAdoptionOptions{
		StateFile:   "state.json",
		Command:     ManagerPaneAdoptionStartNext,
		CurrentPane: "%new",
	}, deps{Tmux: tmux})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Changed || got.State.ManagerPaneID != "%new" ||
		got.State.Delivery.ManagerPaneID != "%new" {
		t.Fatalf("adoption = %+v", got)
	}
}

func TestManagerPaneAdoptionLiveConflictRequiresExplicitPane(t *testing.T) {
	state := ManagerState{
		ManagerPaneID: "%old",
		Delivery:      ManagerDeliveryState{ManagerPaneID: "%old"},
	}
	got, err := ResolveManagerPaneAdoption(t.Context(), state, ManagerPaneAdoptionOptions{
		StateFile:   "state.json",
		Command:     ManagerPaneAdoptionContinue,
		CurrentPane: "%new",
	}, deps{Tmux: &recordingTmux{}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Changed || got.ActionCard == nil ||
		got.ActionCard.Kind != ActionManagerPaneAdoptionRequired ||
		!strings.Contains(
			got.ActionCard.SafeCommand,
			"continue --state-file state.json --manager-pane",
		) {
		t.Fatalf("adoption = %+v", got)
	}
}

func TestManagerPaneAdoptionDoesNotAdoptUnavailableCurrentPane(t *testing.T) {
	tmux := &recordingTmux{missingPanes: map[string]bool{"%new": true}}
	got, err := ResolveManagerPaneAdoption(
		t.Context(),
		ManagerState{},
		ManagerPaneAdoptionOptions{
			StateFile:   "state.json",
			Command:     ManagerPaneAdoptionContinue,
			CurrentPane: "%new",
		},
		deps{Tmux: tmux},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Changed || got.State.ManagerPaneID != "" ||
		got.Reason != "current_manager_pane_unavailable" {
		t.Fatalf("adoption = %+v", got)
	}
}

func TestContinueManagerPaneRebindsBeforeNextChildLaunch(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "state.json")
	sessionDir := filepath.Join(fixture.dir, "sessions")
	donePath := filepath.Join(fixture.dir, "done")
	sessionPath := writePiSession(
		t,
		sessionDir,
		"session.jsonl",
		"session-1",
		fixture.projectRoot,
		assistantLine(
			testResultYAML(
				"question",
				"complete",
				"complete",
				"thoughts/example/questions/q.md",
				"",
			),
		),
	)
	writeFile(t, donePath, "")
	saveManagerState(t, stateFile, ManagerState{
		RepoID:           fixture.projectRoot,
		CanonicalPlanDir: fixture.planDir,
		SourceCwd:        fixture.projectRoot,
		ManagerPaneID:    "%old",
		Workflow:         testWorkflowState(t, qrspi.NodeQuestion, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "question",
			Cwd:                  fixture.projectRoot,
			SessionID:            "session-1",
			SessionDir:           sessionDir,
			SessionPath:          sessionPath,
			DonePath:             donePath,
			ValidationStatusPath: filepath.Join(fixture.dir, "validation.json"),
			LifecycleStatus:      "completed",
			Generation:           1,
		},
	})
	runner := &fakeChildRunner{panes: []string{"%research"}}
	if err := RunContinue(
		t.Context(),
		ContinueOptions{StateFile: stateFile, ManagerPane: "%new"},
		deps{Clock: fixture.clock, Runner: runner, Tmux: &recordingTmux{}},
		&strings.Builder{},
	); err != nil {
		t.Fatalf("RunContinue error = %v", err)
	}
	if len(runner.started) != 1 || runner.started[0].ParentPaneID != "%new" {
		t.Fatalf("started = %+v", runner.started)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ManagerPaneID != "%new" || loaded.ActiveChild == nil ||
		loaded.ActiveChild.Stage != string(qrspi.NodeResearch) {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestContinueCurrentPaneLiveConflictWritesActionCard(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "state.json")
	saveManagerState(t, stateFile, ManagerState{
		RepoID:           fixture.projectRoot,
		CanonicalPlanDir: fixture.planDir,
		SourceCwd:        fixture.projectRoot,
		ManagerPaneID:    "%old",
		Workflow:         testWorkflowState(t, qrspi.NodeQuestion, nil),
		ActiveChild: &ChildRunRef{
			ID:    "child-1",
			Stage: "question",
			Cwd:   fixture.projectRoot,
		},
	})
	t.Setenv("TMUX_PANE", "%new")
	var out strings.Builder
	if err := RunContinue(
		t.Context(),
		ContinueOptions{StateFile: stateFile},
		deps{Clock: fixture.clock, Tmux: &recordingTmux{}},
		&out,
	); err != nil {
		t.Fatalf("RunContinue error = %v", err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.LastActionCard == nil ||
		loaded.LastActionCard.Kind != ActionManagerPaneAdoptionRequired ||
		loaded.ManagerPaneID != "%old" {
		t.Fatalf("loaded = %+v", loaded)
	}
	text := out.String()
	if !strings.Contains(text, "action: manager_pane_adoption_required") ||
		!strings.Contains(
			text,
			"safe command: vamos qrspi continue --state-file "+stateFile+" --manager-pane \"$TMUX_PANE\"",
		) {
		t.Fatalf("output = %q", text)
	}
}
