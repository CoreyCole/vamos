package chatsession

import (
	"reflect"
	"testing"
)

func TestProjectionAppliesSemanticEvents(t *testing.T) {
	events := []ChatEvent{
		{
			SessionID:   "session-1",
			Seq:         1,
			EventType:   EventMessageCompleted,
			PayloadJSON: []byte(`{"id":"m1","role":"user","content":"hello"}`),
		},
		{
			SessionID:   "session-1",
			Seq:         2,
			EventType:   EventToolStarted,
			RunID:       "run-1",
			PayloadJSON: []byte(`{"tool_call_id":"tool-1","tool_name":"read","summary":"reading"}`),
		},
		{
			SessionID:   "session-1",
			Seq:         3,
			EventType:   EventFileWritten,
			PayloadJSON: []byte(`{"path":"notes.md"}`),
		},
	}
	proj, err := RebuildProjection(events, nil)
	if err != nil {
		t.Fatalf("RebuildProjection() error = %v", err)
	}
	if len(proj.Messages) != 1 || proj.Messages[0].Content != "hello" {
		t.Fatalf("messages = %+v, want semantic completed message", proj.Messages)
	}
	if len(proj.Tools) != 1 || proj.Tools[0].ID != "tool-1" || proj.Tools[0].Status != "running" {
		t.Fatalf("tools = %+v, want projected running tool", proj.Tools)
	}
	if len(proj.Artifacts) != 1 || proj.Artifacts[0].Path != "notes.md" || proj.Artifacts[0].Kind != "written" {
		t.Fatalf("artifacts = %+v, want written file artifact", proj.Artifacts)
	}
}

func TestProjectionRebuildEqualsIncremental(t *testing.T) {
	events := []ChatEvent{
		{
			SessionID:   "session-1",
			Seq:         1,
			EventType:   EventMessageCreated,
			PayloadJSON: []byte(`{"id":"m1","role":"user","content":"hello"}`),
		},
		{
			SessionID:   "session-1",
			Seq:         2,
			EventType:   EventRunStarted,
			RunID:       "run-1",
			PayloadJSON: []byte(`{"run_id":"run-1"}`),
		},
		{
			SessionID:   "session-1",
			Seq:         3,
			EventType:   EventRunCompleted,
			RunID:       "run-1",
			PayloadJSON: []byte(`{"summary":"done"}`),
		},
	}
	rebuilt, err := RebuildProjection(events, nil)
	if err != nil {
		t.Fatalf("RebuildProjection() error = %v", err)
	}
	incremental := ChatProjection{}
	for _, event := range events {
		incremental, err = ApplyEvent(incremental, event)
		if err != nil {
			t.Fatalf("ApplyEvent() error = %v", err)
		}
	}
	if !reflect.DeepEqual(rebuilt, incremental) {
		t.Fatalf("rebuilt = %+v, incremental = %+v", rebuilt, incremental)
	}
	if rebuilt.LastSeq != 3 || len(rebuilt.Messages) != 1 || len(rebuilt.Runs) != 1 {
		t.Fatalf("projection = %+v, want message, run, last seq 3", rebuilt)
	}
}
