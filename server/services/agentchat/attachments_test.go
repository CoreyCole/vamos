package agentchat

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/pkg/db"
)

const attachmentTestBasename = "design.md"

func TestValidateAttachedThoughtsPath(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	root := filepath.Join(t.TempDir(), "thoughts")
	if err := os.MkdirAll(filepath.Join(root, "plans", "demo"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "plans", "demo", "design.md"),
		[]byte("# Design"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile outside: %v", err)
	}
	if err := os.Symlink(
		outside,
		filepath.Join(root, "plans", "demo", "outside.md"),
	); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	service.thoughtsRoot = root

	attached, err := service.ValidateAttachedThoughtsPath("thoughts/plans/demo/design.md")
	if err != nil {
		t.Fatalf("ValidateAttachedThoughtsPath(valid) error = %v", err)
	}
	if attached.Path != "thoughts/plans/demo/design.md" ||
		attached.Basename != attachmentTestBasename {
		t.Fatalf("attached = %#v", attached)
	}

	for _, path := range []string{
		"",
		"plans/demo/design.md",
		"/tmp/design.md",
		"thoughts/../outside.md",
		"thoughts/plans/../../outside.md",
		"thoughts/plans/demo/missing.md",
		"thoughts/plans/demo/outside.md",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			if _, err := service.ValidateAttachedThoughtsPath(path); err == nil {
				t.Fatalf(
					"ValidateAttachedThoughtsPath(%q) error = nil, want rejection",
					path,
				)
			}
		})
	}
}

func TestBuildAttachedPathContextListsFullThoughtsPaths(t *testing.T) {
	t.Parallel()

	got := BuildAttachedPathContext(
		[]AttachedPath{
			{Path: "thoughts/plans/demo/design.md", Basename: attachmentTestBasename},
		},
	)
	if !strings.Contains(got, "this prompt only") ||
		!strings.Contains(got, "- thoughts/plans/demo/design.md") {
		t.Fatalf("BuildAttachedPathContext() = %q", got)
	}
}

func TestStableTranscriptAttachesRunScopedPillsToUserMessage(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)
	run := mustCreateAgentRun(t, service, thread.ID, "run-1")
	if err := service.appendRunAttachments(
		t.Context(),
		service.queries,
		run.ID,
		thread.ID,
		[]AttachedPath{
			{Path: "thoughts/plans/demo/design.md", Basename: attachmentTestBasename},
		},
	); err != nil {
		t.Fatalf("appendRunAttachments: %v", err)
	}
	if err := service.queries.CreateAgentEntry(
		t.Context(),
		db.CreateAgentEntryParams{
			LineageID:        thread.LineageID,
			EntryID:          "user-1",
			ParentEntryID:    sql.NullString{},
			EntryType:        "message",
			OriginOrder:      0,
			PayloadJson:      `{"type":"message","id":"user-1","parentId":null,"timestamp":"2026-04-19T12:00:00Z","message":{"role":"user","content":"read this"}}`,
			OriginThreadID:   thread.ID,
			OriginRunID:      sql.NullString{String: run.ID, Valid: true},
			OriginSessionID:  sql.NullString{},
			SessionTimestamp: time.Now().UTC(),
		},
	); err != nil {
		t.Fatalf("CreateAgentEntry: %v", err)
	}
	thread.HeadEntryID = sql.NullString{String: "user-1", Valid: true}

	messages, err := service.buildStableTranscript(t.Context(), thread)
	if err != nil {
		t.Fatalf("buildStableTranscript: %v", err)
	}
	if len(messages) != 1 || len(messages[0].Attachments) != 1 ||
		messages[0].Attachments[0].Basename != attachmentTestBasename {
		t.Fatalf("messages = %#v, want user message attachment", messages)
	}
}

func TestLiveTranscriptAttachesRunScopedPillsToPendingUserMessage(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)
	run := mustCreateAgentRun(t, service, thread.ID, "run-1")
	if err := service.appendRunAttachments(
		t.Context(),
		service.queries,
		run.ID,
		thread.ID,
		[]AttachedPath{
			{Path: "thoughts/plans/demo/design.md", Basename: attachmentTestBasename},
		},
	); err != nil {
		t.Fatalf("appendRunAttachments: %v", err)
	}
	if err := service.ApplyLiveEvent(conversation.EventEnvelope{
		RunID:       run.ID,
		ThreadID:    thread.ID,
		EventType:   "message_start",
		PayloadJSON: `{"message":{"role":"user","content":"read this"}}`,
	}); err != nil {
		t.Fatalf("ApplyLiveEvent: %v", err)
	}

	live, _ := service.buildLiveTranscript(thread.ID)
	if len(live.Items) != 1 || len(live.Items[0].Attachments) != 1 ||
		live.Items[0].Attachments[0].Basename != attachmentTestBasename {
		t.Fatalf("live.Items = %#v, want user message attachment", live.Items)
	}
}
