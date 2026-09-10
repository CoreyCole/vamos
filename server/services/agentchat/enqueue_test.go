package agentchat

import (
	"context"
	"path/filepath"
	"testing"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/pkg/db"
	serverdb "github.com/CoreyCole/vamos/server/services/db"
)

type recordingTemporal struct {
	signals int
}

func (r *recordingTemporal) StartWorkflow(
	context.Context,
	string,
	any,
	any,
) (string, error) {
	return "run", nil
}

func (r *recordingTemporal) SignalWithStartWorkflow(
	context.Context,
	string,
	string,
	any,
	any,
	any,
) (string, error) {
	r.signals++
	return "run", nil
}

func TestEnqueueThreadMailDedupesOpIDAcrossCalls(t *testing.T) {
	t.Parallel()
	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "ops.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	q := database.Queries
	ctx := t.Context()
	if _, err := q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:        "thread-ops",
		UserEmail: "owner@example.com",
		Title:     "Chat",
		Cwd:       "thoughts/plan",
		LineageID: "lin-ops",
	}); err != nil {
		t.Fatal(err)
	}
	temporal := &recordingTemporal{}
	svc := &Service{db: database.DB(), queries: q, temporal: temporal}
	in := EnqueueThreadMailInput{
		ThreadID:      "thread-ops",
		OpID:          "op-1",
		FromKind:      EnqueueFromUser,
		FromUserEmail: "owner@example.com",
		Body:          "hello",
	}
	first, err := svc.EnqueueThreadMail(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if first.Duplicate ||
		first.WorkflowID != conversation.ThreadWorkflowID("thread-ops") {
		t.Fatalf("first enqueue = %+v", first)
	}
	second, err := svc.EnqueueThreadMail(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicate {
		t.Fatal("second enqueue with same op_id should be a no-op")
	}
	if temporal.signals != 1 {
		t.Fatalf("signals = %d, want 1", temporal.signals)
	}
	count, err := q.CountAgentThreadOps(ctx, "thread-ops")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("ops rows = %d, want 1", count)
	}
}
