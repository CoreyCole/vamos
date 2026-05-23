package conversation

import (
	"reflect"
	"testing"
)

func TestLiveTurnReducerPreservesMessageAndToolExecutionOrdering(t *testing.T) {
	reducer := NewLiveTurnReducer()

	events := []EventEnvelope{
		{RunID: "run-1", EventType: "message_end", PayloadJSON: `{"message":{"role":"user","content":"hi"}}`},
		{RunID: "run-1", EventType: "message_update", PayloadJSON: `{"message":{"role":"assistant","content":[{"type":"thinking","thinking":"plan"},{"type":"text","text":"answer"},{"type":"toolCall","id":"call-1","name":"read","arguments":{"path":"main.go"}}]}}`},
		{RunID: "run-1", EventType: "tool_execution_start", PayloadJSON: `{"toolCallId":"call-1","toolName":"read","args":{"path":"main.go"}}`},
		{RunID: "run-1", EventType: "tool_execution_end", PayloadJSON: `{"toolCallId":"call-1","toolName":"read","result":{"content":[{"type":"text","text":"file body"}],"details":{}},"isError":false}`},
		{RunID: "run-1", EventType: "message_end", PayloadJSON: `{"message":{"role":"toolResult","toolCallId":"call-1","toolName":"read","content":[{"type":"text","text":"file body"}],"details":{},"isError":false}}`},
	}

	for _, env := range events {
		if _, err := reducer.Apply(env); err != nil {
			t.Fatalf("Apply(%s) error = %v", env.EventType, err)
		}
	}

	snapshot := reducer.Snapshot()
	gotKinds := make([]LiveTurnItemKind, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		gotKinds = append(gotKinds, item.Kind)
	}

	wantKinds := []LiveTurnItemKind{
		LiveTurnUserMessage,
		LiveTurnAssistantMessage,
		LiveTurnToolExecution,
		LiveTurnToolResult,
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("kinds = %v, want %v", gotKinds, wantKinds)
	}
	if snapshot.Items[2].Status != ToolExecutionDone {
		t.Fatalf("tool execution status = %q, want %q", snapshot.Items[2].Status, ToolExecutionDone)
	}
}

func TestLiveTurnReducerResetsWhenRunChanges(t *testing.T) {
	reducer := NewLiveTurnReducer()

	if _, err := reducer.Apply(EventEnvelope{RunID: "run-1", EventType: "message_end", PayloadJSON: `{"message":{"role":"user","content":"hi"}}`}); err != nil {
		t.Fatalf("Apply(run-1) error = %v", err)
	}
	if _, err := reducer.Apply(EventEnvelope{RunID: "run-2", EventType: "message_update", PayloadJSON: `{"message":{"role":"assistant","content":[{"type":"text","text":"next"}]}}`}); err != nil {
		t.Fatalf("Apply(run-2) error = %v", err)
	}

	snapshot := reducer.Snapshot()
	if snapshot.RunID != "run-2" {
		t.Fatalf("RunID = %q, want run-2", snapshot.RunID)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("len(snapshot.Items) = %d, want 1", len(snapshot.Items))
	}
	if snapshot.Items[0].Kind != LiveTurnAssistantMessage {
		t.Fatalf("snapshot.Items[0].Kind = %q, want %q", snapshot.Items[0].Kind, LiveTurnAssistantMessage)
	}
}

func TestLiveTurnReducerSnapshotIsDeepCopy(t *testing.T) {
	reducer := NewLiveTurnReducer()
	if _, err := reducer.Apply(EventEnvelope{RunID: "run-1", EventType: "message_update", PayloadJSON: `{"message":{"role":"assistant","content":[{"type":"text","text":"hello"}]}}`}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	snapshot := reducer.Snapshot()
	if len(snapshot.Items) != 1 || len(snapshot.Items[0].MessageJSON) == 0 {
		t.Fatalf("snapshot = %#v, want one message item", snapshot)
	}
	snapshot.Items[0].MessageJSON[0] = 'x'
	snapshot.Items = append(snapshot.Items, LiveTurnItem{Key: "mutated"})

	again := reducer.Snapshot()
	if len(again.Items) != 1 {
		t.Fatalf("len(again.Items) = %d, want 1", len(again.Items))
	}
	if again.Items[0].MessageJSON[0] == 'x' {
		t.Fatalf("snapshot mutation leaked into reducer state: %q", again.Items[0].MessageJSON)
	}
}
