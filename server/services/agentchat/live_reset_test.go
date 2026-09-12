package agentchat

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/pkg/db"
)

func TestResetLiveThreadPreservesPendingUserOnFailRun(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	service.liveThreads = make(map[string]*liveThreadState)

	run, err := queries.CreateAgentRun(t.Context(), db.CreateAgentRunParams{
		ID:          "run_fail_keep_user",
		ThreadID:    "thread_1",
		Trigger:     "resume",
		Status:      "running",
		PromptText:  "keep me on fail",
		WorkflowID:  "wf_fail_keep",
		RootDocPath: "thoughts/plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.seedPendingUserPrompt(db.AgentThread{ID: "thread_1"}, run); err != nil {
		t.Fatal(err)
	}
	before, _ := service.buildLiveTranscript("thread_1")
	if len(before.Items) == 0 {
		t.Fatal("expected seeded pending user before FailRun")
	}

	if err := service.FailRun(t.Context(), conversation.RunFailure{
		RunID:        run.ID,
		ThreadID:     "thread_1",
		ErrorMessage: "boom",
	}); err != nil {
		t.Fatal(err)
	}

	after, _ := service.buildLiveTranscript("thread_1")
	if len(after.Items) == 0 {
		t.Fatal("FailRun must not empty-REPLACE live when a pending user exists")
	}
	foundUser := false
	for _, item := range after.Items {
		if strings.EqualFold(item.Role, "user") &&
			strings.Contains(item.Content, "keep me on fail") {
			foundUser = true
			break
		}
	}
	if !foundUser {
		t.Fatalf("expected pending user retained after FailRun, got %#v", after.Items)
	}
	if service.liveTranscriptShowWorking("thread_1", after) {
		t.Fatal("ShowWorking should be false after failed run")
	}

	var buf strings.Builder
	if err := LiveTranscriptRegion(
		"thread_1",
		TranscriptPaneState{Live: after, ShowWorking: false},
		"",
	).Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "keep me on fail") {
		t.Fatalf("live region HTML dropped pending user: %s", html)
	}
	if strings.Contains(html, "Waiting for the first completed turn") {
		t.Fatalf("empty live waiting copy must not REPLACE pending user: %s", html)
	}
}

func TestResetLiveThreadPreservesPendingUserOnFinalizeWithoutAssistant(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	service.liveThreads = make(map[string]*liveThreadState)

	run, err := queries.CreateAgentRun(t.Context(), db.CreateAgentRunParams{
		ID:          "run_finalize_keep_user",
		ThreadID:    "thread_1",
		Trigger:     "resume",
		Status:      "running",
		PromptText:  "keep me on finalize",
		WorkflowID:  "wf_finalize_keep",
		RootDocPath: "thoughts/plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.seedPendingUserPrompt(db.AgentThread{ID: "thread_1"}, run); err != nil {
		t.Fatal(err)
	}
	// Empty assistant (model error) is not renderable — user must stay.
	emptyAsst, err := json.Marshal(map[string]any{
		"message": map[string]any{
			"role":    "assistant",
			"content": []any{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyLiveEvent(conversation.EventEnvelope{
		RunID:       run.ID,
		ThreadID:    "thread_1",
		EventType:   "message_end",
		PayloadJSON: string(emptyAsst),
		EventKey:    run.ID + ":empty_asst",
	}); err != nil {
		t.Fatal(err)
	}

	if err := service.FinalizeRun(t.Context(), conversation.RunResult{
		RunID:    run.ID,
		ThreadID: "thread_1",
	}); err != nil {
		t.Fatal(err)
	}

	after, _ := service.buildLiveTranscript("thread_1")
	if len(after.Items) == 0 {
		t.Fatal("FinalizeRun must not empty live when only empty assistant existed")
	}
	foundUser := false
	for _, item := range after.Items {
		if strings.EqualFold(item.Role, "user") {
			foundUser = true
			break
		}
	}
	if !foundUser {
		t.Fatalf("expected pending user after FinalizeRun, got %#v", after.Items)
	}
}

func TestResetLiveThreadKeepsPendingUserWhenAssistantPresentButUserNotPromoted(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	service.liveThreads = make(map[string]*liveThreadState)

	run, err := queries.CreateAgentRun(t.Context(), db.CreateAgentRunParams{
		ID:          "run_with_assistant",
		ThreadID:    "thread_1",
		Trigger:     "resume",
		Status:      "running",
		PromptText:  "user then assistant",
		WorkflowID:  "wf_assistant",
		RootDocPath: "thoughts/plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.seedPendingUserPrompt(db.AgentThread{ID: "thread_1"}, run); err != nil {
		t.Fatal(err)
	}
	assistantPayload, err := json.Marshal(map[string]any{
		"message": map[string]any{
			"role":    "assistant",
			"content": "hello from assistant",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyLiveEvent(conversation.EventEnvelope{
		RunID:       run.ID,
		ThreadID:    "thread_1",
		EventType:   "message_end",
		PayloadJSON: string(assistantPayload),
		EventKey:    run.ID + ":assistant",
	}); err != nil {
		t.Fatal(err)
	}

	// Assistant present in live but user never promoted via clearLive/checkpoint.
	service.resetLiveThread("thread_1")
	after, _ := service.buildLiveTranscript("thread_1")
	if len(after.Items) == 0 {
		t.Fatal("must keep pending user when assistant present but user not promoted")
	}
	foundUser := false
	for _, item := range after.Items {
		if strings.EqualFold(item.Role, "user") &&
			strings.Contains(item.Content, "user then assistant") {
			foundUser = true
			break
		}
	}
	if !foundUser {
		t.Fatalf("expected pending user kept after reset with assistant, got %#v", after.Items)
	}

	state, err := service.BuildLiveTranscriptState(t.Context(), "owner@example.com", "thread_1")
	if err != nil {
		// thread may use createDraftThread email — fall back to HTML from after
		state = TranscriptPaneState{Live: after, ShowWorking: false}
	}
	var buf strings.Builder
	if err := LiveTranscriptRegion("thread_1", state, "").Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "user then assistant") {
		t.Fatalf("PatchLiveTranscript/BuildLive HTML must keep user bubble: %s", html)
	}
	if strings.Contains(html, "Waiting for the first completed turn") {
		t.Fatalf("must not empty-REPLACE live: %s", html)
	}
}

func TestFailRunKeepsUserBubbleHTMLWhenAssistantPresentButNotPromoted(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	service.liveThreads = make(map[string]*liveThreadState)

	run, err := queries.CreateAgentRun(t.Context(), db.CreateAgentRunParams{
		ID:          "run_fail_asst_keep_user",
		ThreadID:    "thread_1",
		Trigger:     "resume",
		Status:      "running",
		PromptText:  "keep after fail with asst",
		WorkflowID:  "wf_fail_asst",
		RootDocPath: "thoughts/plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.seedPendingUserPrompt(db.AgentThread{ID: "thread_1"}, run); err != nil {
		t.Fatal(err)
	}
	assistantPayload, err := json.Marshal(map[string]any{
		"message": map[string]any{
			"role":    "assistant",
			"content": "partial asst",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyLiveEvent(conversation.EventEnvelope{
		RunID:       run.ID,
		ThreadID:    "thread_1",
		EventType:   "message_end",
		PayloadJSON: string(assistantPayload),
		EventKey:    run.ID + ":asst",
	}); err != nil {
		t.Fatal(err)
	}

	if err := service.FailRun(t.Context(), conversation.RunFailure{
		RunID:        run.ID,
		ThreadID:     "thread_1",
		ErrorMessage: "boom",
	}); err != nil {
		t.Fatal(err)
	}

	after, _ := service.buildLiveTranscript("thread_1")
	var buf strings.Builder
	if err := LiveTranscriptRegion(
		"thread_1",
		TranscriptPaneState{Live: after, ShowWorking: false},
		"",
	).Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "keep after fail with asst") {
		t.Fatalf("after Accept seed + Fail with assistant, live HTML must keep user: %s", html)
	}
}

func TestClearLiveThreadEmptiesPendingUser(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	service.liveThreads = make(map[string]*liveThreadState)

	run, err := queries.CreateAgentRun(t.Context(), db.CreateAgentRunParams{
		ID:          "run_clear_live",
		ThreadID:    "thread_1",
		Trigger:     "resume",
		Status:      "running",
		PromptText:  "cleared by checkpoint",
		WorkflowID:  "wf_clear",
		RootDocPath: "thoughts/plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.seedPendingUserPrompt(db.AgentThread{ID: "thread_1"}, run); err != nil {
		t.Fatal(err)
	}
	service.clearLiveThread("thread_1")
	after, _ := service.buildLiveTranscript("thread_1")
	if len(after.Items) != 0 {
		t.Fatalf("clearLiveThread must drop live, got %#v", after.Items)
	}
}

func TestBuildLiveTranscriptDecodeErrorDoesNotEmptyLive(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")

	snap := conversation.LiveTurnState{
		RunID: "run_decode_err",
		Items: []conversation.LiveTurnItem{
			{
				Key:         "message-000",
				Kind:        conversation.LiveTurnUserMessage,
				Role:        "user",
				MessageJSON: json.RawMessage(`{"role":"user","content":"seeded before bad item"}`),
			},
			{
				Key:         "message-001",
				Kind:        conversation.LiveTurnAssistantMessage,
				Role:        "assistant",
				MessageJSON: json.RawMessage(`{not-json`),
			},
		},
	}
	view := service.buildLiveTranscriptFromSnapshot("thread_1", snap)
	if len(view.Items) == 0 {
		t.Fatal("decode error must not produce empty live when a seeded user exists")
	}
	found := false
	for _, item := range view.Items {
		if strings.Contains(item.Content, "seeded before bad item") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected seeded user to survive decode err, got %#v", view.Items)
	}

	var buf strings.Builder
	if err := LiveTranscriptRegion(
		"thread_1",
		TranscriptPaneState{Live: view, ShowWorking: false},
		"",
	).Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if strings.Contains(html, "Waiting for the first completed turn") {
		t.Fatalf("decode-err path must not emit empty waiting REPLACE: %s", html)
	}
	if !strings.Contains(html, "seeded before bad item") {
		t.Fatalf("live HTML missing surviving user: %s", html)
	}
}

func TestApplyCheckpointEmptyNewEntriesKeepsPendingUser(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	service.liveThreads = make(map[string]*liveThreadState)

	run, err := queries.CreateAgentRun(t.Context(), db.CreateAgentRunParams{
		ID:          "run_empty_cp_keep_user",
		ThreadID:    "thread_1",
		Trigger:     "resume",
		Status:      "running",
		PromptText:  "keep me on empty checkpoint",
		WorkflowID:  "wf_empty_cp",
		RootDocPath: "thoughts/plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.seedPendingUserPrompt(db.AgentThread{ID: "thread_1"}, run); err != nil {
		t.Fatal(err)
	}
	before, _ := service.buildLiveTranscript("thread_1")
	if len(before.Items) == 0 {
		t.Fatal("expected seeded pending user before empty ApplyCheckpoint")
	}

	if err := service.ApplyCheckpoint(t.Context(), conversation.Checkpoint{
		RunID:      run.ID,
		ThreadID:   "thread_1",
		TurnIndex:  0,
		NewEntries: nil,
	}); err != nil {
		t.Fatal(err)
	}

	after, _ := service.buildLiveTranscript("thread_1")
	if len(after.Items) == 0 {
		t.Fatal("empty NewEntries ApplyCheckpoint must not empty-REPLACE pending user")
	}
	foundUser := false
	for _, item := range after.Items {
		if strings.EqualFold(item.Role, "user") &&
			strings.Contains(item.Content, "keep me on empty checkpoint") {
			foundUser = true
			break
		}
	}
	if !foundUser {
		t.Fatalf("expected pending user retained after empty checkpoint, got %#v", after.Items)
	}
}

func TestApplyCheckpointWithUserEntryClearsLive(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	service.liveThreads = make(map[string]*liveThreadState)

	run, err := queries.CreateAgentRun(t.Context(), db.CreateAgentRunParams{
		ID:          "run_cp_with_user",
		ThreadID:    "thread_1",
		Trigger:     "resume",
		Status:      "running",
		PromptText:  "promoted to stable",
		WorkflowID:  "wf_cp_user",
		RootDocPath: "thoughts/plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.seedPendingUserPrompt(db.AgentThread{ID: "thread_1"}, run); err != nil {
		t.Fatal(err)
	}
	before, _ := service.buildLiveTranscript("thread_1")
	if len(before.Items) == 0 {
		t.Fatal("expected seeded pending user before promoting checkpoint")
	}

	payload := `{"type":"message","id":"entry_cp_user","message":{"role":"user","content":"promoted to stable"}}`
	if err := service.ApplyCheckpoint(t.Context(), conversation.Checkpoint{
		RunID:       run.ID,
		ThreadID:    "thread_1",
		TurnIndex:   1,
		HeadEntryID: "entry_cp_user",
		NewEntries: []conversation.SnapshotEntry{{
			EntryID:     "entry_cp_user",
			EntryType:   "message",
			OriginOrder: 1,
			Timestamp:   time.Now().UTC(),
			PayloadJSON: payload,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	after, _ := service.buildLiveTranscript("thread_1")
	if len(after.Items) != 0 {
		t.Fatalf("NewEntries with user must clearLiveThread (no dup), got %#v", after.Items)
	}
}

func TestApplyCheckpointAssistantOnlyKeepsPendingUser(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	service.liveThreads = make(map[string]*liveThreadState)

	run, err := queries.CreateAgentRun(t.Context(), db.CreateAgentRunParams{
		ID:          "run_cp_assistant_only",
		ThreadID:    "thread_1",
		Trigger:     "resume",
		Status:      "running",
		PromptText:  "keep me on assistant-only ck",
		WorkflowID:  "wf_cp_asst",
		RootDocPath: "thoughts/plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.seedPendingUserPrompt(db.AgentThread{ID: "thread_1"}, run); err != nil {
		t.Fatal(err)
	}
	before, _ := service.buildLiveTranscript("thread_1")
	if len(before.Items) == 0 {
		t.Fatal("expected seeded pending user before assistant-only checkpoint")
	}

	// Non-empty assistant with NO user entry: old gate cleared live; keep seed.
	payload := `{"type":"message","id":"entry_cp_asst","message":{"role":"assistant","content":[{"type":"text","text":"assistant only"}]}}`
	if err := service.ApplyCheckpoint(t.Context(), conversation.Checkpoint{
		RunID:       run.ID,
		ThreadID:    "thread_1",
		TurnIndex:   1,
		HeadEntryID: "entry_cp_asst",
		NewEntries: []conversation.SnapshotEntry{{
			EntryID:     "entry_cp_asst",
			EntryType:   "message",
			OriginOrder: 1,
			Timestamp:   time.Now().UTC(),
			PayloadJSON: payload,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	after, _ := service.buildLiveTranscript("thread_1")
	if len(after.Items) == 0 {
		t.Fatal("assistant-only NewEntries must not clearLive/empty-REPLACE pending user")
	}
	foundUser := false
	for _, item := range after.Items {
		if strings.EqualFold(item.Role, "user") &&
			strings.Contains(item.Content, "keep me on assistant-only ck") {
			foundUser = true
			break
		}
	}
	if !foundUser {
		t.Fatalf("expected pending user retained after assistant-only checkpoint, got %#v", after.Items)
	}

	var buf strings.Builder
	if err := LiveTranscriptRegion(
		"thread_1",
		TranscriptPaneState{Live: after, ShowWorking: false},
		"",
	).Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "keep me on assistant-only ck") {
		t.Fatalf("PatchLive HTML must keep user bubble: %s", html)
	}
	if strings.Contains(html, "Waiting for the first completed turn") {
		t.Fatalf("must not empty-REPLACE live: %s", html)
	}
}
