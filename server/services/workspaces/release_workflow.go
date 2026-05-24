package workspaces

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	ReleaseQueueSignalName      = "enqueue"
	ReleaseQueueStatusQueryName = "status"
	releaseQueueMaxItemsPerRun  = 100
	releaseQueueIdleBackoff     = 30 * time.Second
	releaseQueueActivityTimeout = 30 * time.Minute
)

type ReleaseQueueWorkflowInput struct {
	QueueName string `json:"queue_name"`
}

type ReleaseQueueSignal struct {
	ItemID string `json:"item_id"`
}

type ReleaseQueueWorkflowState struct {
	QueueName   string   `json:"queue_name"`
	Processed   int      `json:"processed"`
	SignalItems []string `json:"signal_items"`
}

type ReleaseQueueQueryResult struct {
	QueueName   string   `json:"queue_name"`
	Processed   int      `json:"processed"`
	SignalItems []string `json:"signal_items"`
}

func (s ReleaseQueueWorkflowState) toQuery() ReleaseQueueQueryResult {
	items := append([]string(nil), s.SignalItems...)
	return ReleaseQueueQueryResult{QueueName: s.QueueName, Processed: s.Processed, SignalItems: items}
}

func ReleaseQueueWorkflow(ctx workflow.Context, input ReleaseQueueWorkflowInput) error {
	queue := input.QueueName
	if queue == "" {
		queue = defaultReleaseQueueName
	}
	state := ReleaseQueueWorkflowState{QueueName: queue}
	if err := workflow.SetQueryHandler(ctx, ReleaseQueueStatusQueryName, func() (ReleaseQueueQueryResult, error) {
		return state.toQuery(), nil
	}); err != nil {
		return err
	}

	signalCh := workflow.GetSignalChannel(ctx, ReleaseQueueSignalName)
	opts := workflow.ActivityOptions{
		StartToCloseTimeout: releaseQueueActivityTimeout,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, opts)

	for state.Processed < releaseQueueMaxItemsPerRun {
		drainReleaseSignals(signalCh, &state)
		var processed bool
		err := workflow.ExecuteActivity(ctx, "ProcessNextReleaseQueueItem").Get(ctx, &processed)
		if err == nil && processed {
			state.Processed++
			continue
		}
		selector := workflow.NewSelector(ctx)
		selector.AddReceive(signalCh, func(c workflow.ReceiveChannel, _ bool) {
			var sig ReleaseQueueSignal
			c.Receive(ctx, &sig)
			if sig.ItemID != "" {
				state.SignalItems = append(state.SignalItems, sig.ItemID)
			}
		})
		selector.AddFuture(workflow.NewTimer(ctx, releaseQueueIdleBackoff), func(workflow.Future) {})
		selector.Select(ctx)
	}
	drainReleaseSignals(signalCh, &state)
	return workflow.NewContinueAsNewError(ctx, ReleaseQueueWorkflow, input)
}

func drainReleaseSignals(signalCh workflow.ReceiveChannel, state *ReleaseQueueWorkflowState) {
	for {
		var sig ReleaseQueueSignal
		if !signalCh.ReceiveAsync(&sig) {
			return
		}
		if sig.ItemID != "" {
			state.SignalItems = append(state.SignalItems, sig.ItemID)
		}
	}
}
