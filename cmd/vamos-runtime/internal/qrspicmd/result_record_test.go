package qrspicmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

func TestResultRecordPathsArePlanLocalAndThoughtsRelative(t *testing.T) {
	plan := testPlanDir(t)
	session := filepath.Join(PlanSessionDir(plan), "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(session), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ref, err := ThoughtsRelativePath(plan, session)
	if err != nil {
		t.Fatalf("ThoughtsRelativePath: %v", err)
	}
	if !strings.HasPrefix(ref, "thoughts/") {
		t.Fatalf("reference = %q, want thoughts-relative", ref)
	}
	if got := PlanResultDir(plan); got != filepath.Join(plan, ".vamos", "qrspi") {
		t.Fatalf("PlanResultDir = %q", got)
	}
	if _, err := ResolveThoughtsReference(plan, ref); err != nil {
		t.Fatalf("ResolveThoughtsReference: %v", err)
	}
}

func TestWriteAndReadResultRecordAreNoReplaceAndStrict(t *testing.T) {
	plan := testPlanDir(t)
	record := testResultRecord(t, plan)
	path := filepath.Join(PlanResultDir(plan), "result.yaml")
	if err := WriteResultRecord(path, record); err != nil {
		t.Fatalf("WriteResultRecord: %v", err)
	}
	if err := WriteResultRecord(
		path,
		record,
	); err == nil ||
		!strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second WriteResultRecord error = %v", err)
	}
	got, err := ReadResultRecord(path)
	if err != nil {
		t.Fatalf("ReadResultRecord: %v", err)
	}
	if got.ID != record.ID || got.Session != record.Session {
		t.Fatalf("record = %#v, want %#v", got, record)
	}
	if err := os.WriteFile(
		path,
		[]byte("version: 1\nunknown: nope\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	_, err = ReadResultRecord(path)
	var invalid ResultRecordInvalidError
	if !errors.As(err, &invalid) {
		t.Fatalf("ReadResultRecord strict error = %T %v", err, err)
	}
}

func TestMigratePlanSessionsDoesNotOverwriteExistingMetadata(t *testing.T) {
	plan := testPlanDir(t)
	legacy := filepath.Join(plan, ".sessions", "pi")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(legacy, "old.jsonl"),
		[]byte("old"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	migration, err := MigratePlanSessions(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !migration.Migrated || migration.SessionDir != PlanSessionDir(plan) {
		t.Fatalf("migration = %#v", migration)
	}
	if _, err := os.Stat(filepath.Join(migration.SessionDir, "old.jsonl")); err != nil {
		t.Fatalf("migrated session missing: %v", err)
	}
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(legacy, "legacy.jsonl"),
		[]byte("legacy"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	migration, err = MigratePlanSessions(plan)
	if err != nil {
		t.Fatal(err)
	}
	if migration.Migrated {
		t.Fatalf("existing target must not be overwritten: %#v", migration)
	}
	if _, err := os.Stat(filepath.Join(legacy, "legacy.jsonl")); err != nil {
		t.Fatalf("legacy collision data was lost: %v", err)
	}
}

func TestValidateResultRecordRejectsProtectedFieldChanges(t *testing.T) {
	plan := testPlanDir(t)
	record := testResultRecord(t, plan)
	state := ManagerState{
		CanonicalPlanDir: plan,
		ManagerRunID:     record.ManagerRunID,
		Workflow:         wruntime.State{CurrentNodeID: wruntime.NodeID(record.Node)},
		ActiveChild: &ChildRunRef{
			ID:          record.SourceChildID,
			Generation:  record.SourceChildGeneration,
			SessionID:   record.Session.ID,
			SessionPath: mustResolveThoughtsRef(t, plan, record.Session.Path),
			ResultID:    record.ID,
			ResultPath:  filepath.Join(PlanResultDir(plan), "generated.yaml"),
		},
	}
	ref := ResultRecordRef{ID: record.ID, Path: state.ActiveChild.ResultPath}
	if err := ValidateResultRecordAt(state, ref, record); err != nil {
		t.Fatalf("ValidateResultRecordAt: %v", err)
	}
	ref.Path = filepath.Join(PlanResultDir(plan), "other.yaml")
	if err := ValidateResultRecordAt(state, ref, record); err == nil ||
		!strings.Contains(err.Error(), "result path") {
		t.Fatalf("tampered path error = %v", err)
	}
	record.Node = "research"
	if err := ValidateResultRecord(
		state,
		record,
	); err == nil ||
		!strings.Contains(err.Error(), "node") {
		t.Fatalf("tampered node error = %v", err)
	}
}

func testPlanDir(t *testing.T) string {
	t.Helper()
	plan := filepath.Join(t.TempDir(), "thoughts", "CoreyCole", "plans", "example")
	if err := os.MkdirAll(plan, 0o755); err != nil {
		t.Fatal(err)
	}
	return plan
}

func testResultRecord(t *testing.T, plan string) ResultRecord {
	t.Helper()
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
	sessionRef, err := ThoughtsRelativePath(plan, session)
	if err != nil {
		t.Fatal(err)
	}
	artifactRef, err := ThoughtsRelativePath(plan, artifact)
	if err != nil {
		t.Fatal(err)
	}
	return ResultRecord{
		Version:               resultRecordVersion,
		ID:                    "qrspi_result_test",
		ManagerRunID:          "run-1",
		SourceChildID:         "child-1",
		SourceChildGeneration: 1,
		Node:                  "design",
		State:                 "complete",
		Outcome:               "complete",
		CreatedAt:             time.Unix(100, 0).UTC(),
		Session:               SessionReference{ID: "session-1", Path: sessionRef},
		Summary: qrspi.Summary{
			PlanGoal:       "result records",
			StageCompleted: "foundation",
			KeyDecisions:   "protect generated fields",
		},
		Artifacts: []qrspi.Artifact{{Role: "primary", Path: artifactRef}},
	}
}

func mustResolveThoughtsRef(t *testing.T, plan, ref string) string {
	t.Helper()
	path, err := ResolveThoughtsReference(plan, ref)
	if err != nil {
		t.Fatal(err)
	}
	return path
}
