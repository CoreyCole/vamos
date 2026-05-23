package temporal

import "testing"

func TestTaskQueueNames(t *testing.T) {
	if GoTaskQueue != "agents-go" {
		t.Fatalf("GoTaskQueue = %q, want %q", GoTaskQueue, "agents-go")
	}
	if TSTaskQueue != "agents-ts" {
		t.Fatalf("TSTaskQueue = %q, want %q", TSTaskQueue, "agents-ts")
	}
}
