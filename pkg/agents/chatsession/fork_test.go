package chatsession

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/CoreyCole/vamos/pkg/db"
)

func TestForkSessionCopiesCompleteBaseline(t *testing.T) {
	ctx := context.Background()
	dbConn, q := openTestDB(t)
	insertServiceTestFixtures(t, ctx, q)
	svc := NewService(dbConn, q)
	seedParentProjectionEvents(t, ctx, svc)

	child, err := svc.ForkSession(ctx, ForkSessionInput{
		ParentID:     "session-1",
		ForkedAtSeq:  3,
		ForkedNodeID: "msg-1",
		ActorEmail:   "forker@example.com",
		Prompt:       "try another way",
	})
	if err != nil {
		t.Fatalf("ForkSession() error = %v", err)
	}
	baseline, err := q.GetChatSessionBaseline(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetChatSessionBaseline() error = %v", err)
	}
	if baseline.ParentSessionID != "session-1" || baseline.ForkedFromSeq != 3 {
		t.Fatalf(
			"baseline parent/seq = %s/%d, want session-1/3",
			baseline.ParentSessionID,
			baseline.ForkedFromSeq,
		)
	}
	assertJSONArrayLen(t, baseline.MessagesJson, 1, "messages")
	assertJSONArrayLen(t, baseline.RunsJson, 1, "runs")
	assertJSONArrayLen(t, baseline.ArtifactsJson, 1, "artifacts")
	assertJSONNotEmpty(t, baseline.ParticipantsJson, "participants")
	assertJSONNotEmpty(t, baseline.TopologyJson, "topology")
	assertJSONContains(t, baseline.SelectedStateJson, "forked_node_id", "msg-1")
}

func TestForkSessionReloadDoesNotReadParentEvents(t *testing.T) {
	ctx := context.Background()
	dbConn, q := openTestDB(t)
	insertServiceTestFixtures(t, ctx, q)
	svc := NewService(dbConn, q)
	seedParentProjectionEvents(t, ctx, svc)

	child, err := svc.ForkSession(ctx, ForkSessionInput{
		ParentID:     "session-1",
		ForkedAtSeq:  1,
		ForkedNodeID: "msg-1",
		ActorEmail:   "forker@example.com",
	})
	if err != nil {
		t.Fatalf("ForkSession() error = %v", err)
	}
	if _, err := svc.AppendEvent(ctx, AppendEventInput{
		SessionID: "session-1",
		EventType: EventMessageCreated,
		PayloadJSON: []byte(
			`{"id":"parent-later","role":"assistant","content":"parent later"}`,
		),
	}); err != nil {
		t.Fatalf("append parent later event: %v", err)
	}
	if _, err := svc.AppendEvent(ctx, AppendEventInput{
		SessionID: child.ID,
		EventType: EventMessageCreated,
		PayloadJSON: []byte(
			`{"id":"child-msg","role":"assistant","content":"child only"}`,
		),
	}); err != nil {
		t.Fatalf("append child event: %v", err)
	}
	baseline, err := q.GetChatSessionBaseline(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetChatSessionBaseline() error = %v", err)
	}
	events, err := q.ListChatSessionEventsAfter(ctx, db.ListChatSessionEventsAfterParams{
		SessionID: child.ID,
		AfterSeq:  baseline.ForkedFromSeq,
		Limit:     100,
	})
	if err != nil {
		t.Fatalf("ListChatSessionEventsAfter(child) error = %v", err)
	}
	proj, err := RebuildProjection(eventsFromRows(events), &ChatBaseline{
		SessionID:         baseline.SessionID,
		ParentSessionID:   baseline.ParentSessionID,
		ForkedFromSeq:     baseline.ForkedFromSeq,
		MessagesJSON:      json.RawMessage(baseline.MessagesJson),
		RunsJSON:          json.RawMessage(baseline.RunsJson),
		ArtifactsJSON:     json.RawMessage(baseline.ArtifactsJson),
		ParticipantsJSON:  json.RawMessage(baseline.ParticipantsJson),
		TopologyJSON:      json.RawMessage(baseline.TopologyJson),
		SelectedStateJSON: json.RawMessage(baseline.SelectedStateJson),
	})
	if err != nil {
		t.Fatalf("RebuildProjection(child) error = %v", err)
	}
	for _, msg := range proj.Messages {
		if msg.ID == "parent-later" || msg.Content == "parent later" {
			t.Fatalf(
				"child projection inherited post-fork parent message: %#v",
				proj.Messages,
			)
		}
	}
}

func TestPromoteSessionUpdatesWorkspaceCurrentSessionAndAppendsEvent(t *testing.T) {
	ctx := context.Background()
	dbConn, q := openTestDB(t)
	insertServiceTestFixtures(t, ctx, q)
	svc := NewService(dbConn, q)
	child, err := svc.ForkSession(
		ctx,
		ForkSessionInput{ParentID: "session-1", ActorEmail: "owner@example.com"},
	)
	if err != nil {
		t.Fatalf("ForkSession() error = %v", err)
	}
	if err := svc.PromoteSession(
		ctx,
		"workspace-1",
		child.ID,
		"owner@example.com",
	); err != nil {
		t.Fatalf("PromoteSession() error = %v", err)
	}
	workspace, err := q.GetWorkspace(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if !workspace.CurrentSessionID.Valid ||
		workspace.CurrentSessionID.String != child.ID {
		t.Fatalf("current session = %#v, want %s", workspace.CurrentSessionID, child.ID)
	}
	events, err := q.ListChatSessionEventsAfter(
		ctx,
		db.ListChatSessionEventsAfterParams{SessionID: child.ID, AfterSeq: 0, Limit: 10},
	)
	if err != nil {
		t.Fatalf("ListChatSessionEventsAfter() error = %v", err)
	}
	if events[len(events)-1].EventType != string(EventPromoted) {
		t.Fatalf(
			"last event = %s, want %s",
			events[len(events)-1].EventType,
			EventPromoted,
		)
	}
}

func TestSelectionForWorkspaceLoadPriority(t *testing.T) {
	cases := []struct {
		name       string
		input      WorkspaceLoadInput
		wantID     string
		wantReason string
	}{
		{
			name: "url",
			input: WorkspaceLoadInput{
				URLSessionID:       "url",
				WorkspaceCurrentID: "current",
			},
			wantID:     "url",
			wantReason: "url",
		},
		{
			name: "last selected",
			input: WorkspaceLoadInput{
				UserLastSelectedID: "last",
				WorkspaceCurrentID: "current",
			},
			wantID:     "last",
			wantReason: "user_last_selected",
		},
		{
			name: "current",
			input: WorkspaceLoadInput{
				WorkspaceCurrentID: "current",
				RunningSessionID:   "running",
			},
			wantID:     "current",
			wantReason: "workspace_current",
		},
		{
			name: "running",
			input: WorkspaceLoadInput{
				RunningSessionID:       "running",
				LatestUpdatedSessionID: "latest",
			},
			wantID:     "running",
			wantReason: "running",
		},
		{
			name:       "latest",
			input:      WorkspaceLoadInput{LatestUpdatedSessionID: "latest"},
			wantID:     "latest",
			wantReason: "latest_updated",
		},
		{name: "empty", input: WorkspaceLoadInput{}, wantID: "", wantReason: "empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotID, gotReason, err := SelectionForWorkspaceLoad(tc.input)
			if err != nil {
				t.Fatalf("SelectionForWorkspaceLoad() error = %v", err)
			}
			if gotID != tc.wantID || gotReason != tc.wantReason {
				t.Fatalf(
					"SelectionForWorkspaceLoad() = %q/%q, want %q/%q",
					gotID,
					gotReason,
					tc.wantID,
					tc.wantReason,
				)
			}
		})
	}
}

func seedParentProjectionEvents(t *testing.T, ctx context.Context, svc *Service) {
	t.Helper()
	inputs := []AppendEventInput{
		{
			SessionID:   "session-1",
			EventType:   EventMessageCreated,
			PayloadJSON: []byte(`{"id":"msg-1","role":"user","content":"hello"}`),
		},
		{
			SessionID:   "session-1",
			EventType:   EventRunStarted,
			RunID:       "run-1",
			PayloadJSON: []byte(`{"id":"run-1"}`),
		},
		{
			SessionID:   "session-1",
			EventType:   EventType("artifact.produced"),
			PayloadJSON: []byte(`{"path":"plan.md","kind":"markdown"}`),
		},
	}
	for _, input := range inputs {
		if _, err := svc.AppendEvent(ctx, input); err != nil {
			t.Fatalf("AppendEvent(%s) error = %v", input.EventType, err)
		}
	}
}

func assertJSONArrayLen(t *testing.T, raw string, want int, label string) {
	t.Helper()
	var items []any
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		t.Fatalf("%s json: %v", label, err)
	}
	if len(items) != want {
		t.Fatalf("%s length = %d, want %d; raw=%s", label, len(items), want, raw)
	}
}

func assertJSONNotEmpty(t *testing.T, raw, label string) {
	t.Helper()
	if !json.Valid([]byte(raw)) || raw == "" || raw == "null" {
		t.Fatalf("%s json invalid/empty: %q", label, raw)
	}
}

func assertJSONContains(t *testing.T, raw, key, want string) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("json object: %v", err)
	}
	if got, _ := payload[key].(string); got != want {
		t.Fatalf("%s = %q, want %q in %s", key, got, want, raw)
	}
}
