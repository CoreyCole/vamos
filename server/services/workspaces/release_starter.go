package workspaces

import (
	"context"
	"fmt"
)

const defaultReleaseQueueName = "default"

type temporalReleaseClient interface {
	StartWorkflowIdempotent(
		ctx context.Context,
		workflowID string,
		workflowFunc, input any,
	) (string, error)
	SignalWorkflow(ctx context.Context, workflowID, runID, signalName string, arg any) error
}

type ReleaseWorkflowStarter interface {
	EnqueueRelease(ctx context.Context, itemID string) error
}

type TemporalReleaseStarter struct {
	temporal temporalReleaseClient
	queue    string
}

func NewTemporalReleaseStarter(temporal temporalReleaseClient) *TemporalReleaseStarter {
	return &TemporalReleaseStarter{temporal: temporal, queue: defaultReleaseQueueName}
}

func (s *TemporalReleaseStarter) EnqueueRelease(ctx context.Context, itemID string) error {
	if s == nil || s.temporal == nil {
		return fmt.Errorf("temporal release starter is not configured")
	}
	queue := s.queue
	if queue == "" {
		queue = defaultReleaseQueueName
	}
	workflowID := ReleaseQueueWorkflowID(queue)
	input := ReleaseQueueWorkflowInput{QueueName: queue}
	if _, err := s.temporal.StartWorkflowIdempotent(ctx, workflowID, ReleaseQueueWorkflow, input); err != nil {
		return err
	}
	return s.temporal.SignalWorkflow(ctx, workflowID, "", ReleaseQueueSignalName, ReleaseQueueSignal{ItemID: itemID})
}

func ReleaseQueueWorkflowID(queue string) string {
	if queue == "" {
		queue = defaultReleaseQueueName
	}
	return "release-queue/" + queue
}
