package workspaces

import (
	"context"
	"fmt"

	"github.com/CoreyCole/vamos/pkg/db"
)

type implWorkspaceCleanupMarker interface {
	MarkImplWorkspaceCleanedUp(ctx context.Context, arg db.MarkImplWorkspaceCleanedUpParams) (int64, error)
}

// CleanupActivities own retry-safe side effects for workspace cleanup workflows.
type CleanupActivities struct {
	Manager  *ManagerService
	Store    implWorkspaceCleanupMarker
	Notifier WorkspaceLifecycleNotifier
}

// CleanupWorkspace stops the workspace if needed, removes its checkout/runtime files,
// marks its implementation row cleaned up, and notifies Workspaces SSE subscribers.
func (a *CleanupActivities) CleanupWorkspace(ctx context.Context, input WorkspaceCleanupWorkflowInput) error {
	if a.Manager == nil {
		return fmt.Errorf("workspace manager is not configured")
	}
	if err := a.Manager.CleanupWorkspace(ctx, input); err != nil {
		return err
	}
	if a.Store != nil {
		if _, err := a.Store.MarkImplWorkspaceCleanedUp(ctx, db.MarkImplWorkspaceCleanedUpParams{ProjectID: input.ProjectID, WorkspaceSlug: input.Slug}); err != nil {
			return fmt.Errorf("mark implementation workspace cleaned up: %w", err)
		}
	}
	if a.Notifier != nil {
		a.Notifier.Notify(input.Slug)
	}
	return nil
}
