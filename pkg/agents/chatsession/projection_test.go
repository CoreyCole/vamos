package chatsession

import (
	"reflect"
	"testing"
)

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
