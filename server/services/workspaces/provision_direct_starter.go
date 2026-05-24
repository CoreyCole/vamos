package workspaces

import (
	"context"
	"fmt"
)

type DirectProvisionStarter struct {
	Activities *WorkspaceProvisionActivities
}

func NewDirectProvisionStarter(activities *WorkspaceProvisionActivities) *DirectProvisionStarter {
	return &DirectProvisionStarter{Activities: activities}
}

func (s *DirectProvisionStarter) StartProvision(
	ctx context.Context,
	input WorkspaceProvisionInput,
) (WorkspaceProvisionResult, error) {
	if s == nil || s.Activities == nil {
		return WorkspaceProvisionResult{}, fmt.Errorf("workspace provision activities are not configured")
	}
	return s.Activities.provision(ctx, input)
}
