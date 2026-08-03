package fixtures

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/server/services/agentchat"
	_ "modernc.org/sqlite"
)

func TestBuildHermesSharedThreadsCreatesEqualIDIsolatedDiskEvidence(t *testing.T) {
	root := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(root, "agents.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createThoughtsWorkbenchFixtureSchema(t, db)
	thoughtsRoot := filepath.Join(root, "thoughts")
	state, err := BuildHermesSharedThreads(context.Background(), db, Input{
		Workspace:    WorkspaceIdentity{Slug: "feature-hermes", CheckoutPath: root, DBPath: filepath.Join(root, "agents.db")},
		ThoughtsRoot: thoughtsRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.Name != HermesSharedThreadsFixture || state.Data["thread_id"] != hermesSharedThreadID {
		t.Fatalf("state = %#v", state)
	}

	plans := []agentchat.HermesPlanIdentity{
		"e2e-owner/plans/hermes-alpha",
		"e2e-owner/plans/hermes-beta",
	}
	seenIDs := make([]map[string]bool, len(plans))
	for index, plan := range plans {
		path, err := agentchat.HermesTranscriptPath(
			filepath.Join(thoughtsRoot, filepath.FromSlash(string(plan))),
			hermesSharedThreadID,
		)
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		seenIDs[index] = map[string]bool{}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			var event agentchat.HermesTranscriptEvent
			if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
				file.Close()
				t.Fatal(err)
			}
			if event.PlanDir != plan || event.ThreadID != hermesSharedThreadID {
				file.Close()
				t.Fatalf("cross-bound event = %#v", event)
			}
			seenIDs[index][event.ID] = true
		}
		if err := scanner.Err(); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"session_store", "generation", "process_handle", "settlement_admitted"} {
			if strings.Contains(strings.ToLower(string(raw)), forbidden) {
				t.Fatalf("fixture contains live-proof field %q", forbidden)
			}
		}
	}
	for id := range seenIDs[0] {
		if seenIDs[1][id] {
			t.Fatalf("fixture event ID %q crosses plans", id)
		}
	}
}
