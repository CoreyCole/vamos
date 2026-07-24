package qrspicmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestInitResultRecordValidatesSelectionAndBindsActiveChild(t *testing.T) {
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
			SessionID:   "session-1",
			SessionDir:  PlanSessionDir(plan),
			SessionPath: session,
			Generation:  1,
		},
	}
	ref, record, err := InitResultRecord(t.Context(), ResultInitRequest{
		State: state, StateID: "complete", Outcome: "complete", Artifact: artifactRef,
		Now: time.Date(2026, 7, 24, 21, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("InitResultRecord() error = %v", err)
	}
	if !strings.HasPrefix(ref.ID, "qrspi_result_") ||
		!strings.HasPrefix(ref.Path, PlanResultDir(plan)) {
		t.Fatalf("ref = %#v", ref)
	}
	if record.Summary.TextContent() != "" {
		t.Fatalf("generated summary = %#v, want editable blank template", record.Summary)
	}
	if _, _, err := InitResultRecord(
		t.Context(),
		ResultInitRequest{State: state, StateID: "complete", Artifact: artifactRef},
	); err == nil ||
		!strings.Contains(err.Error(), "outcome") {
		t.Fatalf("missing outcome error = %v", err)
	}
	record.Summary = qrspi.Summary{
		PlanGoal:       "goal",
		StageCompleted: "design",
		KeyDecisions:   "outline next",
	}
	data := marshalRecord(t, record)
	if err := os.WriteFile(ref.Path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	state.ActiveChild.ResultID = ref.ID
	state.ActiveChild.ResultPath = ref.Path
	decision, err := ReadValidatedActiveResultRecord(state)
	if err != nil {
		t.Fatalf("ReadValidatedActiveResultRecord() error = %v", err)
	}
	if decision.Decision.NextNodeID != qrspi.NodeOutline || decision.RecordRef == nil ||
		decision.RecordRef.ID != ref.ID {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestRunResultInitPersistsActiveRecordBinding(t *testing.T) {
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
	root := t.TempDir()
	stateFile := filepath.Join(root, "manager", "run.json")
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
	if err := (FileStateStore{}).Save(stateFile, state); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	now := time.Date(2026, 7, 24, 21, 0, 0, 0, time.UTC)
	if err := RunResultInit(
		t.Context(),
		ResultInitOptions{
			StateFile: stateFile,
			State:     "complete",
			Outcome:   "complete",
			Artifact:  artifactRef,
		},
		deps{Clock: func() time.Time { return now }},
		&out,
	); err != nil {
		t.Fatalf("RunResultInit() error = %v", err)
	}
	loaded, err := (FileStateStore{}).Load(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ActiveChild.ResultID == "" || loaded.ActiveChild.ResultPath == "" ||
		!strings.Contains(out.String(), loaded.ActiveChild.ResultID) {
		t.Fatalf("state/output = %#v / %q", loaded.ActiveChild, out.String())
	}
}

func TestGatherChildEvidencePrefersBoundResultRecord(t *testing.T) {
	plan := testPlanDir(t)
	record := testResultRecord(t, plan)
	path := filepath.Join(PlanResultDir(plan), "result.yaml")
	if err := WriteResultRecord(path, record); err != nil {
		t.Fatal(err)
	}
	state := ManagerState{
		CanonicalPlanDir: plan,
		ManagerRunID:     record.ManagerRunID,
		Workflow:         testWorkflowState(t, qrspi.NodeDesign, nil),
		ActiveChild: &ChildRunRef{
			ID:          record.SourceChildID,
			Stage:       record.Node,
			Generation:  record.SourceChildGeneration,
			SessionID:   record.Session.ID,
			SessionPath: mustResolveThoughtsRef(t, plan, record.Session.Path),
			ResultID:    record.ID,
			ResultPath:  path,
		},
	}
	evidence, err := GatherChildEvidence(state, ChildCompletionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.RecordDecision == nil || evidence.CurrentResult != nil ||
		evidence.RecordError != nil {
		t.Fatalf("evidence = %#v, record error = %v", evidence, evidence.RecordError)
	}
	if got := ClassifyChildIntentForState(
		state,
		evidence,
	).Kind; got != ChildIntentGraphValidResult {
		t.Fatalf("intent = %q", got)
	}
}

func marshalRecord(t *testing.T, record ResultRecord) []byte {
	t.Helper()
	data, err := yaml.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
