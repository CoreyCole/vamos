package chatsession

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/CoreyCole/vamos/pkg/db"
)

func openTestDB(t *testing.T) (*sql.DB, *db.Queries) {
	t.Helper()
	dbConn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	schema, err := os.ReadFile(
		filepath.Join("..", "..", "db", "migrations", "schema.sql"),
	)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if _, err := dbConn.Exec(string(schema)); err != nil {
		t.Fatalf("exec schema: %v", err)
	}
	return dbConn, db.New(dbConn)
}

func insertServiceTestFixtures(t *testing.T, ctx context.Context, q *db.Queries) {
	t.Helper()
	if _, err := q.CreateWorkspace(ctx, db.CreateWorkspaceParams{
		ID:           "workspace-1",
		UserEmail:    "owner@example.com",
		Title:        "Workspace",
		RootDocPath:  "thoughts/demo",
		WorkflowType: "freeform",
		Source:       "web",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := q.CreateChatSession(ctx, db.CreateChatSessionParams{
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

func TestSubmitCommandDuplicateIdempotencyReturnsOriginalOutcome(t *testing.T) {
	ctx := context.Background()
	dbConn, q := openTestDB(t)
	insertServiceTestFixtures(t, ctx, q)
	svc := NewService(dbConn, q)

	input := SubmitCommandInput{
		WorkspaceID:    "workspace-1",
		SessionID:      "session-1",
		IdempotencyKey: "idem-1",
		ActorEmail:     "owner@example.com",
		Type:           CommandType("message.send"),
		PayloadJSON:    []byte(`{"prompt":"hello"}`),
	}
	first, err := svc.SubmitCommand(ctx, input)
	if err != nil {
		t.Fatalf("submit first command: %v", err)
	}
	second, err := svc.SubmitCommand(ctx, input)
	if err != nil {
		t.Fatalf("submit duplicate command: %v", err)
	}
	if second.CommandID != first.CommandID || second.Status != first.Status {
		t.Fatalf(
			"duplicate outcome = (%s, %s), want (%s, %s)",
			second.CommandID,
			second.Status,
			first.CommandID,
			first.Status,
		)
	}

	var commandCount int
	if err := dbConn.QueryRowContext(ctx, `SELECT COUNT(*) FROM chat_session_commands`).
		Scan(&commandCount); err != nil {
		t.Fatalf("count commands: %v", err)
	}
	if commandCount != 1 {
		t.Fatalf("command count = %d, want 1", commandCount)
	}
	var eventCount int
	if err := dbConn.QueryRowContext(ctx, `SELECT COUNT(*) FROM chat_session_events`).
		Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != 2 {
		t.Fatalf("event count = %d, want submitted+accepted only", eventCount)
	}
}

func TestSubmitCommandPersistsSemanticCommandEvents(t *testing.T) {
	ctx := context.Background()
	dbConn, q := openTestDB(t)
	insertServiceTestFixtures(t, ctx, q)
	svc := NewService(dbConn, q)

	outcome, err := svc.SubmitCommand(ctx, SubmitCommandInput{
		WorkspaceID:    "workspace-1",
		SessionID:      "session-1",
		IdempotencyKey: "idem-1",
		ActorEmail:     "owner@example.com",
		Type:           CommandType("message.send"),
		PayloadJSON:    []byte(`{"prompt":"hello"}`),
	})
	if err != nil {
		t.Fatalf("submit command: %v", err)
	}
	if outcome.Status != CommandAccepted {
		t.Fatalf("status = %s, want accepted", outcome.Status)
	}
	events, err := q.ListChatSessionEventsAfter(
		ctx,
		db.ListChatSessionEventsAfterParams{
			SessionID: "session-1",
			AfterSeq:  0,
			Limit:     10,
		},
	)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events count = %d, want 2", len(events))
	}
	if events[0].Seq != 1 || events[0].EventType != string(EventCommandSubmitted) {
		t.Fatalf(
			"first event = (%d, %s), want command.submitted seq 1",
			events[0].Seq,
			events[0].EventType,
		)
	}
	if events[1].Seq != 2 || events[1].EventType != string(EventCommandAccepted) {
		t.Fatalf(
			"second event = (%d, %s), want command.accepted seq 2",
			events[1].Seq,
			events[1].EventType,
		)
	}
}

func TestAppendEventConcurrentWriters(t *testing.T) {
	ctx := context.Background()
	dbConn, q := openTestDB(t)
	dbConn.SetMaxOpenConns(1)
	insertServiceTestFixtures(t, ctx, q)
	svc := NewService(dbConn, q)

	const writers = 16
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		i := i
		go func() {
			_, err := svc.AppendEvent(ctx, AppendEventInput{
				SessionID:   "session-1",
				EventType:   EventMessageCreated,
				PayloadJSON: []byte(fmt.Sprintf(`{"writer":%d}`, i)),
			})
			errCh <- err
		}()
	}
	for i := 0; i < writers; i++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
	events, err := q.ListChatSessionEventsAfter(
		ctx,
		db.ListChatSessionEventsAfterParams{
			SessionID: "session-1",
			AfterSeq:  0,
			Limit:     writers,
		},
	)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	seqs := make([]int, 0, len(events))
	for _, event := range events {
		seqs = append(seqs, int(event.Seq))
	}
	sort.Ints(seqs)
	for i, seq := range seqs {
		if seq != i+1 {
			t.Fatalf("seqs = %v, want contiguous 1..%d", seqs, writers)
		}
	}
}

func TestAppendEventPayloadDefaultsToObject(t *testing.T) {
	ctx := context.Background()
	dbConn, q := openTestDB(t)
	insertServiceTestFixtures(t, ctx, q)
	svc := NewService(dbConn, q)

	_, err := svc.AppendEvent(
		ctx,
		AppendEventInput{SessionID: "session-1", EventType: EventMessageCreated},
	)
	if err != nil {
		t.Fatalf("append event: %v", err)
	}
	events, err := q.ListChatSessionEventsAfter(
		ctx,
		db.ListChatSessionEventsAfterParams{
			SessionID: "session-1",
			AfterSeq:  0,
			Limit:     1,
		},
	)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if got := events[0].PayloadJson; got != `{}` {
		t.Fatalf("payload = %q, want {}", got)
	}
}
