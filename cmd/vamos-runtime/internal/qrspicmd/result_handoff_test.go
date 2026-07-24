package qrspicmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestResultHandoffResolvesWithinSelectedPlanWithoutManagerMutation(t *testing.T) {
	plan := testPlanDir(t)
	record := testResultRecord(t, plan)
	path := filepath.Join(PlanResultDir(plan), "record.yaml")
	if err := WriteResultRecord(path, record); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := RunResultHandoff(
		ResultHandoffOptions{PlanDir: plan, ResultID: record.ID, Print: true},
		&out,
	); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{record.ID, record.Session.Path, "does not attach a q-manager"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("handoff missing %q:\n%s", want, out.String())
		}
	}
	if _, err := ResolveResultID(plan, "qrspi_result_missing"); err == nil {
		t.Fatal("ResolveResultID accepted missing record")
	}
}

func TestAttachManagedSessionBindsOnlyExactEligibleRecord(t *testing.T) {
	plan := testPlanDir(t)
	session := filepath.Join(PlanSessionDir(plan), "child.jsonl")
	artifact := filepath.Join(plan, "design.md")
	if err := os.MkdirAll(filepath.Dir(session), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact, []byte("# design\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	artifactRef, err := ThoughtsRelativePath(plan, artifact)
	if err != nil {
		t.Fatal(err)
	}
	state := ManagerState{
		RepoID:           "repo",
		CanonicalPlanDir: plan,
		ManagerRunID:     "run-1",
		Workflow:         testWorkflowState(t, qrspi.NodeDesign, nil),
		ActiveChild: &ChildRunRef{
			ID:          "child-1",
			Stage:       "design",
			Generation:  1,
			SessionID:   "session-1",
			SessionPath: session,
		},
	}
	ref, record, err := InitResultRecord(t.Context(), ResultInitRequest{
		State: state, StateID: "complete", Outcome: "complete", Artifact: artifactRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	record.Summary = qrspi.Summary{
		PlanGoal:       "goal",
		StageCompleted: "design",
		KeyDecisions:   "outline",
	}
	if err := os.WriteFile(ref.Path, marshalRecord(t, record), 0o644); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	stateFile := StatePath(
		root,
		LockKey{RepoID: "repo", CanonicalPlanDir: plan},
		state.ManagerRunID,
	)
	if err := (FileStateStore{}).Save(stateFile, state); err != nil {
		t.Fatal(err)
	}
	deps := deps{StateRoot: func() (string, error) { return root, nil }}
	var out strings.Builder
	if err := RunAttachManagedSession(t.Context(), AttachManagedSessionOptions{
		PlanDir:      plan,
		ProjectRoot:  "",
		ManagerRunID: state.ManagerRunID,
		ResultID:     record.ID,
		SessionProof: "session-1",
	}, deps, &out); err == nil {
		// ProjectRoot defaults to the test process cwd, not the synthetic repo ID,
		// so attach correctly refuses this mismatched manager identity.
		t.Fatal("attachment unexpectedly accepted foreign project root")
	}

	// Use a manager rooted at the actual test cwd to exercise the exact bind.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoID, err := RepoID(cwd)
	if err != nil {
		t.Fatal(err)
	}
	state.RepoID = repoID
	stateFile = StatePath(
		root,
		LockKey{RepoID: repoID, CanonicalPlanDir: plan},
		state.ManagerRunID,
	)
	if err := (FileStateStore{}).Save(stateFile, state); err != nil {
		t.Fatal(err)
	}
	if err := RunAttachManagedSession(t.Context(), AttachManagedSessionOptions{
		PlanDir:      plan,
		ProjectRoot:  cwd,
		ManagerRunID: state.ManagerRunID,
		ResultID:     record.ID,
		SessionProof: "session-1",
	}, deps, &out); err != nil {
		t.Fatalf("RunAttachManagedSession() error = %v", err)
	}
	loaded, err := (FileStateStore{}).Load(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ActiveChild.ResultID != record.ID ||
		loaded.ActiveChild.ResultPath != ref.Path {
		t.Fatalf("attachment = %#v", loaded.ActiveChild)
	}
}
