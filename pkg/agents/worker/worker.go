package worker

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	temporalmgr "github.com/CoreyCole/vamos/pkg/agents/temporal"
)

type temporalWorker interface {
	RegisterWorkflow(any)
	RegisterActivity(any)
	Start() error
	Stop()
}

var newTemporalWorker = func(c client.Client, taskQueue string) temporalWorker {
	return worker.New(c, taskQueue, worker.Options{})
}

// Worker owns the Go Temporal worker used by Agent Chat workflows and activities.
type Worker struct {
	worker temporalWorker
}

// New constructs an Agent Chat Go worker on the shared agents task queue.
func New(c client.Client) *Worker {
	return &Worker{worker: newTemporalWorker(c, temporalmgr.GoTaskQueue)}
}

// RegisterWorkflow registers a workflow function or named workflow value.
func (w *Worker) RegisterWorkflow(workflow any) {
	w.worker.RegisterWorkflow(workflow)
}

// RegisterActivity registers a Go activity function or receiver.
func (w *Worker) RegisterActivity(activity any) {
	w.worker.RegisterActivity(activity)
}

// Run starts polling until ctx is cancelled or the worker fails.
func (w *Worker) Run(ctx context.Context) error {
	if err := w.worker.Start(); err != nil {
		return fmt.Errorf("agent worker: %w", err)
	}
	<-ctx.Done()
	w.Stop()
	return nil
}

// Stop stops the underlying Temporal worker.
func (w *Worker) Stop() {
	w.worker.Stop()
}
