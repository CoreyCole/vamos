package agentchat

import "context"

type SyncCoordinatorInput struct{ Workspace SyncWorkspacesInput }
type SyncCoordinatorResult struct {
	Workspace SyncWorkspacesResult
	Changed   bool
}
type SyncCoordinatorOptions struct{ WorkspaceSync *WorkspaceSyncer }
type SyncCoordinator struct{ workspaceSync *WorkspaceSyncer }

func NewSyncCoordinator(opts SyncCoordinatorOptions) *SyncCoordinator {
	return &SyncCoordinator{workspaceSync: opts.WorkspaceSync}
}
func DefaultSyncCoordinatorInput(input SyncWorkspacesInput) SyncCoordinatorInput {
	return SyncCoordinatorInput{Workspace: input}
}
func (c *SyncCoordinator) Run(ctx context.Context, input SyncCoordinatorInput) (SyncCoordinatorResult, error) {
	if c == nil || c.workspaceSync == nil {
		return SyncCoordinatorResult{}, nil
	}
	workspace, err := c.workspaceSync.Sync(ctx, input.Workspace)
	return SyncCoordinatorResult{Workspace: workspace, Changed: workspace.Changed}, err
}
