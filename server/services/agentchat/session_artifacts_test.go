package agentchat

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverPlanAgentSessionsIndexesAllCheckpointsSeparatelyFromLegacyResult(
	t *testing.T,
) {
	root := t.TempDir()
	plan := filepath.Join(root, "me", "plans", "example")
	sessionDir := filepath.Join(plan, ".vamos", "sessions", "pi")
	if err := os.MkdirAll(
		filepath.Join(sessionDir, "session-1", "checkpoints"),
		0o700,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(sessionDir, "session-1.jsonl"),
		[]byte("{\"id\":\"session-1\",\"cwd\":\"/tmp\"}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(sessionDir, "session-1_result.yaml"),
		[]byte("legacy\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []string{"entry-b", "entry-a"} {
		if err := os.WriteFile(
			filepath.Join(sessionDir, "session-1", "checkpoints", entry+".yaml"),
			[]byte("version: 2\n"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}

	items, err := DiscoverPlanAgentSessionsUnderThoughts(root, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	item := items[0]
	if item.ResultPath != "me/plans/example/.vamos/sessions/pi/session-1_result.yaml" {
		t.Fatalf("legacy result = %q", item.ResultPath)
	}
	want := []CheckpointArtifact{
		{
			FinalEntryID: "entry-a",
			Path:         "me/plans/example/.vamos/sessions/pi/session-1/checkpoints/entry-a.yaml",
		},
		{
			FinalEntryID: "entry-b",
			Path:         "me/plans/example/.vamos/sessions/pi/session-1/checkpoints/entry-b.yaml",
		},
	}
	if !reflect.DeepEqual(item.Checkpoints, want) {
		t.Fatalf("checkpoints = %#v, want %#v", item.Checkpoints, want)
	}
}

func TestDiscoverPlanAgentSessionsRejectsUnsafeCheckpointComponent(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "me", "plans", "example")
	sessionDir := filepath.Join(plan, ".vamos", "sessions", "pi")
	if err := os.MkdirAll(
		filepath.Join(sessionDir, "session-1", "checkpoints"),
		0o700,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(sessionDir, "session-1.jsonl"),
		[]byte("{\"id\":\"session-1\"}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(sessionDir, "session-1", "checkpoints", "not.safe.yaml"),
		[]byte("version: 2\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverPlanAgentSessionsUnderThoughts(root, plan); err == nil {
		t.Fatal("accepted unsafe checkpoint component")
	}
}
