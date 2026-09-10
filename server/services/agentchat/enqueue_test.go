package agentchat

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/pkg/db"
	serverdb "github.com/CoreyCole/vamos/server/services/db"
)

type recordingTemporal struct {
	signals int
	lastArg any
	err     error
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
	_ context.Context,
	_ string,
	_ string,
	signalArg any,
	_ any,
	_ any,
) (string, error) {
	r.signals++
	r.lastArg = signalArg
	if r.err != nil {
		return "", r.err
	}
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

func TestEnqueueThreadMailDeletesReceiptWhenSignalFails(t *testing.T) {
	t.Parallel()
	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "ops-fail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	q := database.Queries
	ctx := t.Context()
	if _, err := q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:        "thread-fail",
		UserEmail: "owner@example.com",
		Title:     "Chat",
		Cwd:       "thoughts/plan",
		LineageID: "lin-fail",
	}); err != nil {
		t.Fatal(err)
	}
	temporal := &recordingTemporal{err: errors.New("temporal down")}
	svc := &Service{db: database.DB(), queries: q, temporal: temporal}
	in := EnqueueThreadMailInput{
		ThreadID:      "thread-fail",
		OpID:          "op-retry",
		FromKind:      EnqueueFromUser,
		FromUserEmail: "owner@example.com",
		Body:          "hello",
	}
	if _, err := svc.EnqueueThreadMail(ctx, in); err == nil {
		t.Fatal("want signal error")
	}
	count, err := q.CountAgentThreadOps(ctx, "thread-fail")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("ops rows = %d, want 0 so retry can insert", count)
	}
	temporal.err = nil
	if _, err := svc.EnqueueThreadMail(ctx, in); err != nil {
		t.Fatal(err)
	}
	if temporal.signals != 2 {
		t.Fatalf("signals = %d, want 2", temporal.signals)
	}
}

func TestPrepareThreadTurnIsIdempotentForSameOp(t *testing.T) {
	t.Parallel()
	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "prep.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	q := database.Queries
	ctx := t.Context()
	if _, err := q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:        "thread-prep",
		UserEmail: "owner@example.com",
		Title:     "Chat",
		Cwd:       "thoughts/plan",
		LineageID: "lin-prep",
	}); err != nil {
		t.Fatal(err)
	}
	svc := &Service{db: database.DB(), queries: q}
	mail := conversation.ThreadMail{
		ThreadID:      "thread-prep",
		OpID:          "op-prep",
		FromKind:      EnqueueFromUser,
		FromUserEmail: "owner@example.com",
		Body:          "hello",
		Attachments:   []string{"thoughts/plan/note.md"},
	}
	first, err := svc.PrepareThreadTurn(ctx, mail)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.PrepareThreadTurn(ctx, mail)
	if err != nil {
		t.Fatal(err)
	}
	if first.RunID != second.RunID {
		t.Fatalf("run ids %q vs %q", first.RunID, second.RunID)
	}
	att, err := q.ListAgentRunAttachmentsForRun(ctx, first.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(att) != 1 || att[0].Path != "thoughts/plan/note.md" {
		t.Fatalf("attachments = %+v", att)
	}
}

func TestResumeThreadQueuesAttachments(t *testing.T) {
	t.Parallel()
	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "resume.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	q := database.Queries
	ctx := t.Context()
	if _, err := q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:        "thread-resume",
		UserEmail: "owner@example.com",
		Title:     "Chat",
		Cwd:       "thoughts/plan",
		LineageID: "lin-resume",
	}); err != nil {
		t.Fatal(err)
	}
	temporal := &recordingTemporal{}
	svc := &Service{db: database.DB(), queries: q, temporal: temporal}
	if _, _, err := svc.ResumeThread(
		ctx,
		"owner@example.com",
		"thread-resume",
		"hello",
		[]AttachedPath{{Path: "thoughts/plan/a.md", Basename: "a.md"}},
	); err != nil {
		t.Fatal(err)
	}
	mail, ok := temporal.lastArg.(conversation.ThreadMail)
	if !ok {
		t.Fatalf("signal arg = %#v", temporal.lastArg)
	}
	if len(mail.Attachments) != 1 || mail.Attachments[0] != "thoughts/plan/a.md" {
		t.Fatalf("attachments = %v", mail.Attachments)
	}
}
