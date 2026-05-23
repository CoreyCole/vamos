package temporal

import (
	"context"
	"errors"
	"fmt"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
)

const (
	GoTaskQueue = "agents-go"
	TSTaskQueue = "agents-ts"
)

// Manager wraps a Temporal client for starting and signaling workflows.
type Manager struct {
	client client.Client
}

// NewManager dials Temporal and returns a Manager.
func NewManager(addr string) (*Manager, error) {
	c, err := client.Dial(client.Options{
		HostPort: addr,
	})
	if err != nil {
		return nil, fmt.Errorf("dial temporal: %w", err)
	}
	return &Manager{client: c}, nil
}

// Client returns the underlying Temporal client.
func (m *Manager) Client() client.Client {
	return m.client
}

// StartWorkflow starts any Temporal workflow and returns the run ID.
func (m *Manager) StartWorkflow(
	ctx context.Context,
	workflowID string,
	workflowFunc, input any,
) (string, error) {
	opts := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: GoTaskQueue,
	}
	run, err := m.client.ExecuteWorkflow(ctx, opts, workflowFunc, input)
	if err != nil {
		return "", fmt.Errorf("start workflow %s: %w", workflowID, err)
	}
	return run.GetRunID(), nil
}

// StartWorkflowIdempotent starts a workflow and treats an already-started
// execution with the same deterministic workflow ID as duplicate success.
func (m *Manager) StartWorkflowIdempotent(
	ctx context.Context,
	workflowID string,
	workflowFunc, input any,
) (string, error) {
	runID, err := m.StartWorkflow(ctx, workflowID, workflowFunc, input)
	if err == nil {
		return runID, nil
	}
	var alreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
	if errors.As(err, &alreadyStarted) {
		return "", nil
	}
	return "", err
}

// StartWorkflowByName starts a workflow using a string name instead of a function
// reference.
func (m *Manager) StartWorkflowByName(
	ctx context.Context,
	workflowID, workflowName string,
	input any,
) (string, error) {
	opts := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: GoTaskQueue,
	}
	run, err := m.client.ExecuteWorkflow(ctx, opts, workflowName, input)
	if err != nil {
		return "", fmt.Errorf("start workflow %s (%s): %w", workflowID, workflowName, err)
	}
	return run.GetRunID(), nil
}

// Close shuts down the Temporal client.
func (m *Manager) Close() {
	m.client.Close()
}
