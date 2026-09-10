package agentchat

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/db"
	serverdb "github.com/CoreyCole/vamos/server/services/db"
)

func TestRotateRoomCurrentJSONLWritesMatchingTimestamp(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	id := RoomIdentity{Kind: RoomKindBotHome, SpeakerSlug: "nova"}
	current, err := EnsureRoomCurrentJSONL(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		current,
		[]byte("{\"type\":\"session\"}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	listed := filepath.Join(root, "agents", "nova", "notes.md")
	if err := os.WriteFile(listed, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := RotateRoomCurrentJSONL(root, id, RoomHandoffSpec{
		Timestamp: "2026-09-10_05-00-00",
		Body:      "cut body",
		Files:     []string{"thoughts/agents/nova/notes.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.HandoffRel != "thoughts/agents/nova/sessions/handoffs/2026-09-10_05-00-00.md" {
		t.Fatalf("handoff = %q", got.HandoffRel)
	}
	if got.HistoryRel != "thoughts/agents/nova/sessions/history/2026-09-10_05-00-00.jsonl" {
		t.Fatalf("history = %q", got.HistoryRel)
	}
	hist, err := os.ReadFile(got.HistoryAbs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hist), "session") {
		t.Fatalf("history missing prior jsonl: %s", hist)
	}
	cur, err := os.ReadFile(got.CurrentAbs)
	if err != nil {
		t.Fatal(err)
	}
	if len(cur) != 0 {
		t.Fatalf("current.jsonl not empty: %q", cur)
	}
}

func TestRotateRoomCurrentJSONLRejectsMissingFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	id := RoomIdentity{Kind: RoomKindBotHome, SpeakerSlug: "nova"}
	current, err := EnsureRoomCurrentJSONL(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(current, []byte("keep-me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = RotateRoomCurrentJSONL(root, id, RoomHandoffSpec{
		Timestamp: "2026-09-10_05-00-01",
		Files:     []string{"thoughts/agents/nova/missing.md"},
	})
	if err == nil {
		t.Fatal("expected missing files to fail rotate")
	}
	got, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep-me\n" {
		t.Fatalf("current.jsonl moved on failed rotate: %q", got)
	}
}

func TestSettleHotRoomInsertsHandoffCutWithoutWipingHistory(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	thoughtsRoot := filepath.Join(projectRoot, "thoughts")
	home := filepath.Join(thoughtsRoot, "agents", "nova")
	if err := os.MkdirAll(filepath.Join(home, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(home, "notes.md"),
		[]byte("note"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(home, "sessions", "current.jsonl"),
		[]byte("window-1\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "settle.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	threadID := "thread-settle-1"
	if _, err := database.Queries.CreateAgentThread(
		t.Context(),
		db.CreateAgentThreadParams{
			ID:        threadID,
			UserEmail: "owner@example.com",
			Title:     "Nova home",
			Cwd:       home,
			LineageID: threadID,
			ProjectID: "project-1",
		},
	); err != nil {
		t.Fatal(err)
	}
	priorID := "entry-prior"
	if err := database.Queries.CreateAgentEntry(t.Context(), db.CreateAgentEntryParams{
		LineageID:        threadID,
		EntryID:          priorID,
		EntryType:        "message",
		OriginOrder:      0,
		PayloadJson:      `{"type":"message","id":"entry-prior","message":{"role":"user","content":"hi"}}`,
		OriginThreadID:   threadID,
		SessionTimestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Queries.UpdateAgentThreadHead(
		t.Context(),
		db.UpdateAgentThreadHeadParams{
			HeadEntryID: sql.NullString{String: priorID, Valid: true},
			ID:          threadID,
		},
	); err != nil {
		t.Fatal(err)
	}

	service := &Service{
		db:           database.DB(),
		queries:      database.Queries,
		thoughtsRoot: thoughtsRoot,
	}
	result, err := service.SettleHotRoom(t.Context(), SettleHotRoomInput{
		ThreadID:    threadID,
		UsageHot:    true,
		Timestamp:   "2026-09-10_05-01-00",
		HandoffBody: "next window",
		Files:       []string{"thoughts/agents/nova/notes.md"},
		SpeakerSlug: "nova",
		RoleMemoryFiles: map[string]string{
			"MEMORY.md": "remember this",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Rotated || result.CutEntryID == "" {
		t.Fatalf("result = %+v", result)
	}

	laterID := "entry-later"
	if err := database.Queries.CreateAgentEntry(t.Context(), db.CreateAgentEntryParams{
		LineageID: threadID,
		EntryID:   laterID,
		ParentEntryID: sql.NullString{
			String: result.CutEntryID,
			Valid:  true,
		},
		EntryType:        "message",
		OriginOrder:      2,
		PayloadJson:      `{"type":"message","id":"entry-later","message":{"role":"user","content":"after"}}`,
		OriginThreadID:   threadID,
		SessionTimestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Queries.UpdateAgentThreadHead(
		t.Context(),
		db.UpdateAgentThreadHeadParams{
			HeadEntryID: sql.NullString{String: laterID, Valid: true},
			ID:          threadID,
		},
	); err != nil {
		t.Fatal(err)
	}

	path, err := database.Queries.ListAgentEntryPath(
		t.Context(),
		db.ListAgentEntryPathParams{
			LineageID:   threadID,
			HeadEntryID: laterID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(path) != 3 {
		t.Fatalf("path len = %d", len(path))
	}
	if path[0].EntryType != "message" || path[0].EntryID != priorID {
		t.Fatalf("want prior message first, got %+v", path[0])
	}
	if path[1].EntryType != "handoff" {
		t.Fatalf("want handoff cut, got %+v", path[1])
	}
	if !strings.Contains(
		path[1].PayloadJson,
		"thoughts/agents/nova/sessions/handoffs/2026-09-10_05-01-00.md",
	) {
		t.Fatalf("cut missing handoff href: %s", path[1].PayloadJson)
	}
	if !strings.Contains(
		path[1].PayloadJson,
		"thoughts/agents/nova/sessions/history/2026-09-10_05-01-00.jsonl",
	) {
		t.Fatalf("cut missing history href: %s", path[1].PayloadJson)
	}
	if path[2].EntryID != laterID {
		t.Fatalf("want later message after cut, got %+v", path[2])
	}

	mem, err := os.ReadFile(filepath.Join(home, "MEMORY.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(mem) != "remember this" {
		t.Fatalf("MEMORY.md = %q", mem)
	}
}

func TestSettleHotRoomBadFilesLeavesCurrentAndNoCut(t *testing.T) {
	t.Parallel()
	thoughtsRoot := t.TempDir()
	home := filepath.Join(thoughtsRoot, "agents", "nova")
	if err := os.MkdirAll(filepath.Join(home, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(home, "sessions", "current.jsonl")
	if err := os.WriteFile(current, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "settle-fail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	threadID := "thread-settle-fail"
	if _, err := database.Queries.CreateAgentThread(
		t.Context(),
		db.CreateAgentThreadParams{
			ID:        threadID,
			UserEmail: "owner@example.com",
			Title:     "Nova",
			Cwd:       home,
			LineageID: threadID,
			ProjectID: "p",
		},
	); err != nil {
		t.Fatal(err)
	}
	service := &Service{
		db:           database.DB(),
		queries:      database.Queries,
		thoughtsRoot: thoughtsRoot,
	}
	_, err = service.SettleHotRoom(t.Context(), SettleHotRoomInput{
		ThreadID: threadID,
		UsageHot: true,
		Files:    []string{"thoughts/agents/nova/nope.md"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	got, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep\n" {
		t.Fatalf("current moved: %q", got)
	}
	thread, err := database.Queries.GetAgentThread(t.Context(), threadID)
	if err != nil {
		t.Fatal(err)
	}
	if thread.HeadEntryID.Valid {
		t.Fatalf("unexpected cut head %q", thread.HeadEntryID.String)
	}
}

func TestSettleHotRoomPlanDoesNotWriteSpeakerMemory(t *testing.T) {
	t.Parallel()
	thoughtsRoot := t.TempDir()
	planDir := filepath.Join(thoughtsRoot, "acme", "plans", "job")
	if err := os.MkdirAll(
		filepath.Join(planDir, ".vamos", "sessions"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(planDir, "notes.md"),
		[]byte("plan"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(planDir, ".vamos", "sessions", "current.jsonl"),
		[]byte("plan-window\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	speakerHome := filepath.Join(thoughtsRoot, "agents", "nova")
	if err := os.MkdirAll(speakerHome, 0o755); err != nil {
		t.Fatal(err)
	}

	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "settle-plan.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	threadID := "thread-plan-settle"
	if _, err := database.Queries.CreateAgentThread(
		t.Context(),
		db.CreateAgentThreadParams{
			ID:        threadID,
			UserEmail: "owner@example.com",
			Title:     "Plan",
			Cwd:       planDir,
			LineageID: threadID,
			ProjectID: "p",
		},
	); err != nil {
		t.Fatal(err)
	}
	service := &Service{
		db:           database.DB(),
		queries:      database.Queries,
		thoughtsRoot: thoughtsRoot,
	}
	_, err = service.SettleHotRoom(t.Context(), SettleHotRoomInput{
		ThreadID:    threadID,
		UsageHot:    true,
		Timestamp:   "2026-09-10_05-02-00",
		Files:       []string{"thoughts/acme/plans/job/notes.md"},
		SpeakerSlug: "nova",
		RoleMemoryFiles: map[string]string{
			"MEMORY.md": "should not land",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(speakerHome, "MEMORY.md")); !os.IsNotExist(err) {
		t.Fatalf("plan settle wrote speaker MEMORY.md: %v", err)
	}
}
