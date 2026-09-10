package agentchat

import (
	"context"
	"errors"

	"github.com/CoreyCole/vamos/server/services/workspaces"
)

type SyncWorkspacesInput struct {
	ProjectName        string
	ProjectID          string
	ProjectInstanceKey string
	ProjectRoot        string
	ThoughtsRoot       string
	ImplWorkspaces     workspaces.ImplWorkspaceDiscoveryConfig
	ManagerURL         string
	RestartToken       string
	TrunkBranch        string
	RunCompletionHook  bool
}

type SyncWorkspacesResult struct {
	Plan       PlanWorkspaceDiscoveryResult
	Impl       workspaces.ImplWorkspaceSyncResult
	Changed    bool
	Skipped    bool
	SkipReason string
}

type WorkspaceSyncer struct {
	PlanSyncer *PlanWorkspaceSyncer
	ImplSyncer *workspaces.ImplWorkspaceSyncer
	Guard      *WorkspaceSyncGuard
}

func (s *WorkspaceSyncer) Sync(
	ctx context.Context,
	input SyncWorkspacesInput,
) (SyncWorkspacesResult, error) {
	if s == nil {
		return SyncWorkspacesResult{}, errors.New("workspace syncer is nil")
	}
	kind := WorkspaceSyncRunScheduled
	if !input.RunCompletionHook {
		kind = WorkspaceSyncRunManual
	}
	var result SyncWorkspacesResult
	guardResult, err := s.Guard.TryRun(ctx, kind, func(ctx context.Context) error {
		var syncErr error
		result, syncErr = s.syncUnlocked(ctx, input)
		return syncErr
	})
	if errors.Is(err, ErrWorkspaceSyncInProgress) {
		return SyncWorkspacesResult{
			Skipped:    true,
			SkipReason: guardResult.Reason,
		}, nil
	}
	return result, err
}

func (s *WorkspaceSyncer) syncUnlocked(
	ctx context.Context,
	input SyncWorkspacesInput,
) (SyncWorkspacesResult, error) {
	var result SyncWorkspacesResult
	var planErr error
	if s.PlanSyncer != nil {
		var plan PlanWorkspaceDiscoveryResult
		plan, planErr = s.PlanSyncer.Sync(ctx, PlanWorkspaceDiscoveryInput{
			ProjectName:        input.ProjectName,
			ProjectID:          input.ProjectID,
			ProjectInstanceKey: input.ProjectInstanceKey,
			ProjectRoot:        input.ProjectRoot,
			ThoughtsRoot:       input.ThoughtsRoot,
			ImplWorkspaces:     input.ImplWorkspaces,
		})
		result.Plan = plan
	}
	var implErr error
	if s.ImplSyncer != nil {
		implDiscovery := input.ImplWorkspaces
		if implDiscovery.ProjectID == "" {
			implDiscovery.ProjectID = input.ProjectID
		}
		var impl workspaces.ImplWorkspaceSyncResult
		impl, implErr = s.ImplSyncer.Sync(ctx, workspaces.ImplWorkspaceSyncInput{
			ProjectID:    input.ProjectID,
			Discovery:    implDiscovery,
			ManagerURL:   input.ManagerURL,
			RestartToken: input.RestartToken,
			TrunkBranch:  input.TrunkBranch,
		})
		result.Impl = impl
	}
	result.Changed = result.Plan.Changed || result.Impl.Changed
	if implErr != nil {
		return result, implErr
	}
	if s.ImplSyncer == nil {
		return result, planErr
	}
	return result, nil
}
