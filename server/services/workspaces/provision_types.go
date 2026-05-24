package workspaces

import (
	"context"
	"time"
)

type WorkspaceProvisionInput struct {
	PlanPath         string `json:"plan_path"`
	PlanDir          string `json:"plan_dir"`
	WorkspaceSlug    string `json:"workspace_slug"`
	RequestedPath    string `json:"requested_path"`
	SourceCheckout   string `json:"source_checkout"`
	BaselineCheckout string `json:"baseline_checkout"`
	TrunkBranch      string `json:"trunk_branch"`
	ParentStackRef   string `json:"parent_stack_ref,omitempty"`
	ReviewFollowup   bool   `json:"review_followup,omitempty"`
	Force            bool   `json:"force,omitempty"`
}

type WorkspaceProvisionResult struct {
	WorkspacePath string `json:"workspace_path"`
	WorkspaceSlug string `json:"workspace_slug"`
	BaseRef       string `json:"base_ref"`
	BaseCommit    string `json:"base_commit"`
	MetadataDir   string `json:"metadata_dir"`
	Status        string `json:"status"`
	Message       string `json:"message,omitempty"`
}

type WorkspaceProvisionMetadata struct {
	Slug             string    `json:"slug"`
	PlanPath         string    `json:"plan_path"`
	PlanDir          string    `json:"plan_dir"`
	WorkspacePath    string    `json:"workspace_path"`
	SourceCheckout   string    `json:"source_checkout"`
	BaselineCheckout string    `json:"baseline_checkout,omitempty"`
	BaseRef          string    `json:"base_ref"`
	BaseCommit       string    `json:"base_commit"`
	TrunkBranch      string    `json:"trunk_branch"`
	ParentStackRef   string    `json:"parent_stack_ref,omitempty"`
	ReviewFollowup   bool      `json:"review_followup,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	WorkflowID       string    `json:"workflow_id,omitempty"`
	RunID            string    `json:"run_id,omitempty"`
}

const (
	WorkspaceProvisionStatusComplete = "complete"
	WorkspaceProvisionStatusBlocked  = "blocked"
	WorkspaceProvisionStatusError    = "error"
)

type WorkspaceProvisionStarter interface {
	StartProvision(ctx context.Context, input WorkspaceProvisionInput) (WorkspaceProvisionResult, error)
}
