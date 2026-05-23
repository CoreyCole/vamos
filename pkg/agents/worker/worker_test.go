package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.temporal.io/sdk/client"

	temporalmgr "github.com/CoreyCole/vamos/pkg/agents/temporal"
)

type fakeTemporalWorker struct {
	workflows  []any
	activities []any
	startErr   error
	started    bool
	stopped    bool
}

func (f *fakeTemporalWorker) RegisterWorkflow(workflow any) {
	f.workflows = append(f.workflows, workflow)
}

func (f *fakeTemporalWorker) RegisterActivity(activity any) {
	f.activities = append(f.activities, activity)
}

func (f *fakeTemporalWorker) Start() error {
	f.started = true
	return f.startErr
}

func (f *fakeTemporalWorker) Stop() {
	f.stopped = true
}

func withFakeTemporalWorker(
	t *testing.T,
	replacement func(client.Client, string) temporalWorker,
) {
	t.Helper()
	original := newTemporalWorker
	newTemporalWorker = replacement
	t.Cleanup(func() { newTemporalWorker = original })
}

func TestNewUsesAgentsGoTaskQueue(t *testing.T) {
	var gotQueue string
	fake := &fakeTemporalWorker{}
	withFakeTemporalWorker(t, func(_ client.Client, taskQueue string) temporalWorker {
		gotQueue = taskQueue
		return fake
	})

	worker := New(nil)
	if worker.worker != fake {
		t.Fatalf("New() worker = %#v, want fake", worker.worker)
	}
	if gotQueue != temporalmgr.GoTaskQueue {
		t.Fatalf("New() task queue = %q, want %q", gotQueue, temporalmgr.GoTaskQueue)
	}
}

func TestRegisterForwardsToTemporalWorker(t *testing.T) {
	fake := &fakeTemporalWorker{}
	worker := &Worker{worker: fake}
	workflowFn := func() {}
	activityFn := func() {}

	worker.RegisterWorkflow(workflowFn)
	worker.RegisterActivity(activityFn)

	if len(fake.workflows) != 1 || fake.workflows[0] == nil {
		t.Fatalf("RegisterWorkflow() did not forward workflow: %#v", fake.workflows)
	}
	if len(fake.activities) != 1 || fake.activities[0] == nil {
		t.Fatalf("RegisterActivity() did not forward activity: %#v", fake.activities)
	}
}

func TestRunStartsAndStopsOnContextCancel(t *testing.T) {
	fake := &fakeTemporalWorker{}
	worker := &Worker{worker: fake}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- worker.Run(ctx) }()

	deadline := time.After(2 * time.Second)
	for !fake.started {
		select {
		case <-deadline:
			t.Fatal("Run() did not start worker")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
	if !fake.stopped {
		t.Fatal("Run() did not stop worker after context cancellation")
	}
}

func TestRunReturnsStartError(t *testing.T) {
	startErr := errors.New("boom")
	fake := &fakeTemporalWorker{startErr: startErr}
	worker := &Worker{worker: fake}

	err := worker.Run(context.Background())
	if !errors.Is(err, startErr) {
		t.Fatalf("Run() error = %v, want %v", err, startErr)
	}
	if fake.stopped {
		t.Fatal("Run() stopped worker after failed start")
	}
}
