package agentchat

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/CoreyCole/vamos/pkg/db"
)

func TestHermesArtifactListAndProjectionReadersUseTranscriptLock(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	schema, err := os.ReadFile(filepath.Join("..", "..", "..", "pkg", "db", "migrations", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	plan := filepath.Join(root, "owner", "plans", "alpha")
	if err := os.MkdirAll(plan, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plan, "AGENTS.md"), []byte("# Plan\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(
		plan, hermesMetadataFixture("owner/plans/alpha", "thread_1"),
	); err != nil {
		t.Fatal(err)
	}
	service := &Service{thoughtsRoot: root, queries: db.New(database)}
	assertSharedLock := func(name string, read func() error) {
		t.Helper()
		acquisitions := 0
		hermesTranscriptLockAcquiredHook = func(_ string, _ string, shared bool) {
			if shared {
				acquisitions++
			}
		}
		if err := read(); err != nil {
			t.Fatalf("%s read: %v", name, err)
		}
		hermesTranscriptLockAcquiredHook = nil
		if acquisitions < 2 {
			t.Fatalf("%s readers acquired transcript lock %d times, want at least 2", name, acquisitions)
		}
	}
	defer func() { hermesTranscriptLockAcquiredHook = nil }()
	assertSharedLock("artifact", func() error {
		_, err := DiscoverPlanAgentSessionsUnderThoughts(root, plan)
		return err
	})
	assertSharedLock("list", func() error {
		_, err := service.ListHermesThreads(
			context.Background(), ThreadQuery{PlanDir: "owner/plans/alpha"},
		)
		return err
	})
	assertSharedLock("projection", func() error {
		_, err := service.RebuildSessionProjection(context.Background())
		return err
	})
}

func TestRebuildSessionProjectionRestoresHermesMetadataAfterDBWipe(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	schema, err := os.ReadFile(filepath.Join("..", "..", "..", "pkg", "db", "migrations", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	plan := filepath.Join(root, "owner", "plans", "alpha")
	if err := os.MkdirAll(plan, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plan, "AGENTS.md"), []byte("# Plan\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(plan, hermesMetadataFixture("owner/plans/alpha", "thread_1")); err != nil {
		t.Fatal(err)
	}
	service := &Service{thoughtsRoot: root, queries: db.New(database)}
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 1 {
			if _, err := database.Exec("DELETE FROM agent_sessions"); err != nil {
				t.Fatal(err)
			}
		}
		result, err := service.RebuildSessionProjection(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if result.Sessions != 1 {
			t.Fatalf("attempt %d sessions = %d", attempt, result.Sessions)
		}
		row, err := service.queries.GetAgentSessionByPath(
			context.Background(), nullableString("owner/plans/alpha/.vamos/sessions/hermes/thread_1.jsonl"),
		)
		if err != nil {
			t.Fatal(err)
		}
		var indexed SessionArtifactIndex
		if err := json.Unmarshal([]byte(row.MetadataJson.String), &indexed); err != nil {
			t.Fatal(err)
		}
		if indexed.HermesMetadata == nil || indexed.HermesMetadata.Title != "Shared thread" ||
			indexed.HermesMetadata.PromptAuthority.PrincipalValue != "owner@example.com" {
			t.Fatalf("metadata after attempt %d = %#v", attempt, indexed.HermesMetadata)
		}
	}
}
