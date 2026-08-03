package agentchat

import (
	"context"
	"encoding/json"
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

func TestDiscoverPlanAgentSessionsParsesHermesHeaderMetadata(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "me", "plans", "example")
	if err := os.MkdirAll(plan, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := hermesMetadataFixture("me/plans/example", "thread_1")
	if err := AppendHermesTranscript(plan, metadata); err != nil {
		t.Fatal(err)
	}
	items, err := DiscoverPlanAgentSessionsUnderThoughts(root, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].HermesMetadata == nil ||
		items[0].HermesMetadata.Title != metadata.Title {
		t.Fatalf("items = %#v", items)
	}
	threads, err := ScanHermesThreads(root, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 || threads[0].PromptAuthority.PrincipalValue != "owner@example.com" {
		t.Fatalf("threads = %#v", threads)
	}
}

func TestScanHermesThreadsDoesNotEnterDescendantPlans(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "me", "plans", "parent")
	child := filepath.Join(parent, "reviews", "child")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(
		parent, hermesMetadataFixture("me/plans/parent", "parent_thread"),
	); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(
		child, hermesMetadataFixture("me/plans/parent/reviews/child", "child_thread"),
	); err != nil {
		t.Fatal(err)
	}

	threads, err := ScanHermesThreads(root, parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 || threads[0].ID != "parent_thread" {
		t.Fatalf("threads = %#v", threads)
	}
	service := &Service{thoughtsRoot: root}
	listed, err := service.ListHermesThreads(
		context.Background(), ThreadQuery{PlanDir: "me/plans/parent"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != "parent_thread" {
		t.Fatalf("listed threads = %#v", listed)
	}
}

func TestDiscoverPlanAgentSessionsRejectsMalformedHermesArtifact(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "me", "plans", "example")
	dir := filepath.Join(plan, ".vamos", "sessions", "hermes")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	event := HermesTranscriptEvent{
		ID: "event_1", Type: "user", ThreadID: "thread_1",
		PlanDir: "me/plans/example", Content: "missing header",
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "thread_1.jsonl"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverPlanAgentSessionsUnderThoughts(root, plan); err == nil {
		t.Fatal("malformed Hermes artifact was indexed")
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
