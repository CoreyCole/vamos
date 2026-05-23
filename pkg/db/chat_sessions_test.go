package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

func insertChatSessionTestFixtures(t *testing.T, ctx context.Context, q *Queries) {
	t.Helper()
	if _, err := q.db.ExecContext(ctx, `
INSERT INTO workspaces (id, user_email, title, root_doc_path, workflow_type, source)
VALUES ('workspace-1', 'owner@example.com', 'Workspace', 'thoughts/demo', 'freeform', 'web')
`); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	if _, err := q.CreateChatSession(ctx, CreateChatSessionParams{
		ID:                 "session-1",
		WorkspaceID:        "workspace-1",
		CreatedByUserEmail: "owner@example.com",
		BranchID:           "branch-1",
		WorkflowAttempt:    0,
		TopologyKind:       "root",
	}); err != nil {
		t.Fatalf("create chat session: %v", err)
	}
}

func TestChatCommandIdempotencyKeyUniquePerSession(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)
	insertChatSessionTestFixtures(t, ctx, q)

	first, err := q.CreateChatCommand(ctx, CreateChatCommandParams{
		ID:             "command-1",
		SessionID:      "session-1",
		IdempotencyKey: "idem-1",
		CommandType:    "message.send",
		Status:         "submitted",
		ActorEmail:     "owner@example.com",
		PayloadJson:    `{"prompt":"hello"}`,
	})
	if err != nil {
		t.Fatalf("create first command: %v", err)
	}

	_, err = q.CreateChatCommand(ctx, CreateChatCommandParams{
		ID:             "command-duplicate",
		SessionID:      "session-1",
		IdempotencyKey: "idem-1",
		CommandType:    "message.send",
		Status:         "submitted",
		ActorEmail:     "owner@example.com",
		PayloadJson:    `{"prompt":"again"}`,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Fatalf("duplicate command err = %v, want unique constraint", err)
	}

	got, err := q.GetChatCommandByIdempotencyKey(
		ctx,
		GetChatCommandByIdempotencyKeyParams{
			SessionID:      "session-1",
			IdempotencyKey: "idem-1",
		},
	)
	if err != nil {
		t.Fatalf("get command by idempotency: %v", err)
	}
	if got.ID != first.ID || got.PayloadJson != first.PayloadJson {
		t.Fatalf(
			"got command (%s, %s), want first (%s, %s)",
			got.ID,
			got.PayloadJson,
			first.ID,
			first.PayloadJson,
		)
	}
}

func TestReserveChatSessionSeqConcurrentWriters(t *testing.T) {
	ctx := context.Background()
	dbConn, q := openWorkspaceDocsTestDB(t)
	dbConn.SetMaxOpenConns(1)
	insertChatSessionTestFixtures(t, ctx, q)

	const writers = 16
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := dbConn.BeginTx(ctx, nil)
			if err != nil {
				errCh <- fmt.Errorf("begin tx %d: %w", i, err)
				return
			}
			defer tx.Rollback()
			txq := q.WithTx(tx)
			if err := txq.EnsureChatSessionSequence(ctx, "session-1"); err != nil {
				errCh <- fmt.Errorf("ensure seq %d: %w", i, err)
				return
			}
			seq, err := txq.ReserveChatSessionSeq(ctx, "session-1")
			if err != nil {
				errCh <- fmt.Errorf("reserve seq %d: %w", i, err)
				return
			}
			if _, err := txq.AppendChatSessionEvent(ctx, AppendChatSessionEventParams{
				SessionID:   "session-1",
				Seq:         seq,
				EventType:   "message.created",
				PayloadJson: fmt.Sprintf(`{"writer":%d}`, i),
			}); err != nil {
				errCh <- fmt.Errorf("append event %d: %w", i, err)
				return
			}
			if err := tx.Commit(); err != nil {
				errCh <- fmt.Errorf("commit %d: %w", i, err)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	events, err := q.ListChatSessionEventsAfter(ctx, ListChatSessionEventsAfterParams{
		SessionID: "session-1",
		AfterSeq:  0,
		Limit:     writers,
	})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != writers {
		t.Fatalf("event count = %d, want %d", len(events), writers)
	}
	seqs := make([]int, 0, len(events))
	for _, event := range events {
		seqs = append(seqs, int(event.Seq))
	}
	sort.Ints(seqs)
	for i, seq := range seqs {
		want := i + 1
		if seq != want {
			t.Fatalf("seqs = %v, want contiguous 1..%d", seqs, writers)
		}
	}
}

func TestChatSessionModeConstraints(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)
	insertChatSessionTestFixtures(t, ctx, q)

	external, err := q.CreateExternalAgentSession(ctx, CreateExternalAgentSessionParams{
		ID:       "external-1",
		Provider: "pi",
	})
	if err != nil {
		t.Fatalf("create external session: %v", err)
	}
	if _, err := q.LinkExternalAgentSession(ctx, LinkExternalAgentSessionParams{
		ID:                     "link-1",
		ChatSessionID:          "session-1",
		ExternalAgentSessionID: external.ID,
		LinkMode:               "imported",
	}); err != nil {
		t.Fatalf("link imported external session: %v", err)
	}
	if _, err := q.LinkExternalAgentSession(ctx, LinkExternalAgentSessionParams{
		ID:                     "link-invalid",
		ChatSessionID:          "session-1",
		ExternalAgentSessionID: external.ID,
		LinkMode:               "invalid",
	}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "constraint") {
		t.Fatalf("invalid link mode err = %v, want constraint", err)
	}

	if _, err := q.CreateAgentSurfaceAttachment(ctx, CreateAgentSurfaceAttachmentParams{
		ID:             "surface-1",
		ChatSessionID:  "session-1",
		SurfaceKind:    "temporal_worker",
		PermissionMode: "own",
	}); err != nil {
		t.Fatalf("create valid surface attachment: %v", err)
	}
	if _, err := q.CreateAgentSurfaceAttachment(ctx, CreateAgentSurfaceAttachmentParams{
		ID:             "surface-invalid-kind",
		ChatSessionID:  "session-1",
		SurfaceKind:    "invalid",
		PermissionMode: "observe",
	}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "constraint") {
		t.Fatalf("invalid surface kind err = %v, want constraint", err)
	}
	if _, err := q.CreateAgentSurfaceAttachment(ctx, CreateAgentSurfaceAttachmentParams{
		ID:             "surface-invalid-permission",
		ChatSessionID:  "session-1",
		SurfaceKind:    "web",
		PermissionMode: "invalid",
	}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "constraint") {
		t.Fatalf("invalid surface permission err = %v, want constraint", err)
	}
}

func TestUpdateWorkspaceCurrentSession(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)
	insertChatSessionTestFixtures(t, ctx, q)

	if err := q.UpdateWorkspaceCurrentSession(ctx, UpdateWorkspaceCurrentSessionParams{
		ID:               "workspace-1",
		CurrentSessionID: sql.NullString{String: "session-1", Valid: true},
		CurrentBranchID:  sql.NullString{String: "branch-1", Valid: true},
	}); err != nil {
		t.Fatalf("update current session: %v", err)
	}
	workspace, err := q.GetWorkspace(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if !workspace.CurrentSessionID.Valid ||
		workspace.CurrentSessionID.String != "session-1" {
		t.Fatalf("current session = %#v, want session-1", workspace.CurrentSessionID)
	}
	if !workspace.CurrentBranchID.Valid ||
		workspace.CurrentBranchID.String != "branch-1" {
		t.Fatalf("current branch = %#v, want branch-1", workspace.CurrentBranchID)
	}
}
