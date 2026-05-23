package agentchat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/server/services/workspaces"
)

type devWorkspaceManager interface {
	Refresh(context.Context) error
	List() []workspaces.Workspace
	Lookup(string) (workspaces.Workspace, bool)
	Start(context.Context, string) (workspaces.Workspace, error)
	Stop(context.Context, string) (workspaces.Workspace, error)
}

func (s *Service) SetDevWorkspaceManager(manager devWorkspaceManager) {
	s.devWorkspaceManager = manager
}

func CheckoutNameForPlan(repoName, rootRelPath string) string {
	repoName = strings.TrimSpace(repoName)
	if repoName == "" {
		repoName = "workspace"
	}
	base := filepath.Base(filepath.Clean(strings.TrimSpace(rootRelPath)))
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "plan"
	}
	return repoName + "-" + base
}

func WorkspaceSlugForPlan(repoName, rootRelPath string) (string, error) {
	repoName = strings.TrimSpace(repoName)
	if repoName == "" {
		repoName = "workspace"
	}
	return workspaces.SlugFromCheckoutNameWithConfig(
		CheckoutNameForPlan(repoName, rootRelPath),
		workspaces.DiscoveryConfig{
			CheckoutPrefixes: []string{repoName},
			MainCheckoutName: repoName,
		},
	)
}

func VerifyPlanWorkspaceLinks(ctx context.Context, parentRoot string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	lineage, err := DiscoverPlanNodes(parentRoot)
	if err != nil {
		return err
	}
	seen := map[string]string{}
	for _, node := range append([]PlanNode{lineage}, lineage.Children...) {
		rootRel := nodeRootRelForWorkspace(&node)
		slug, err := WorkspaceSlugForPlan("vamos", rootRel)
		if err != nil {
			return fmt.Errorf("workspace slug for %q: %w", rootRel, err)
		}
		if other := seen[slug]; other != "" {
			return fmt.Errorf(
				"workspace slug %q is shared by %q and %q",
				slug,
				other,
				rootRel,
			)
		}
		seen[slug] = rootRel
		if node.Kind == PlanNodeImplementationReviewFollowup &&
			!IsImplementationReviewPlanDir(node.AbsPath) {
			return fmt.Errorf(
				"implementation review follow-up is missing plan markers: %s",
				node.AbsPath,
			)
		}
	}
	return nil
}

func (s *Service) linkPlanLineage(ctx context.Context, lineage *PlanNode) {
	if lineage == nil {
		return
	}
	workspaceBySlug := map[string]workspaces.Workspace{}
	if s.devWorkspaceManager != nil {
		_ = s.devWorkspaceManager.Refresh(ctx)
		for _, ws := range s.devWorkspaceManager.List() {
			workspaceBySlug[ws.Slug] = ws
		}
	}
	s.linkPlanNode(lineage, workspaceBySlug)
	for i := range lineage.Children {
		s.linkPlanNode(&lineage.Children[i], workspaceBySlug)
	}
}

func (s *Service) linkPlanNode(
	node *PlanNode,
	workspaceBySlug map[string]workspaces.Workspace,
) {
	rootRel := nodeRootRelForWorkspace(node)
	slug, err := WorkspaceSlugForPlan(s.projectName, rootRel)
	if err != nil {
		node.Workspace = &WorkspaceLink{
			CheckoutName: CheckoutNameForPlan(s.projectName, rootRel),
			Status:       string(workspaces.StatusInvalid),
			Error:        err.Error(),
			Actions:      []WorkspaceAction{WorkspaceActionRefresh},
		}
		node.Stack = &StackSummary{Available: false, Detail: err.Error()}
		return
	}
	link := WorkspaceLink{
		Slug:         slug,
		CheckoutName: CheckoutNameForPlan(s.projectName, rootRel),
		Status:       string(workspaces.StatusStopped),
		Actions:      []WorkspaceAction{WorkspaceActionStart, WorkspaceActionRefresh},
	}
	stack := StackSummary{Available: false, Detail: "workspace checkout not found"}
	if ws, ok := workspaceBySlug[slug]; ok {
		link.CheckoutPath = ws.CheckoutPath
		link.URL = ws.URL
		link.Status = string(ws.Status)
		link.Phase = string(ws.Phase)
		link.Error = ws.Error
		link.Actions = workspaceActionsForStatus(ws.Status)
		stack = stackSummaryFromWorkspaces(
			workspaces.InspectStack(context.Background(), ws.CheckoutPath),
		)
		if stack.Merged {
			link.Actions = appendUniqueWorkspaceAction(
				link.Actions,
				WorkspaceActionDelete,
			)
		}
	}
	node.Workspace = &link
	node.Stack = &stack
}

func nodeRootRelForWorkspace(node *PlanNode) string {
	if strings.TrimSpace(node.RootRelPath) != "" {
		return node.RootRelPath
	}
	return node.AbsPath
}

func workspaceActionsForStatus(status workspaces.Status) []WorkspaceAction {
	switch status {
	case workspaces.StatusRunning, workspaces.StatusStarting, workspaces.StatusStopping:
		return []WorkspaceAction{
			WorkspaceActionStop,
			WorkspaceActionMerge,
			WorkspaceActionRefresh,
		}
	case workspaces.StatusFailed, workspaces.StatusCrashed, workspaces.StatusInvalid:
		return []WorkspaceAction{
			WorkspaceActionRetry,
			WorkspaceActionMerge,
			WorkspaceActionRefresh,
		}
	default:
		return []WorkspaceAction{
			WorkspaceActionStart,
			WorkspaceActionMerge,
			WorkspaceActionRefresh,
		}
	}
}

func appendUniqueWorkspaceAction(
	actions []WorkspaceAction,
	action WorkspaceAction,
) []WorkspaceAction {
	for _, existing := range actions {
		if existing == action {
			return actions
		}
	}
	return append(actions, action)
}

func stackSummaryFromWorkspaces(summary workspaces.StackSummary) StackSummary {
	return StackSummary{
		Branch:       summary.Branch,
		TopBranch:    summary.TopBranch,
		BottomParent: summary.BottomParent,
		BaseBranch:   summary.BaseBranch,
		AheadCount:   summary.AheadCount,
		BehindCount:  summary.BehindCount,
		Merged:       summary.Merged,
		Available:    summary.Available,
		Detail:       summary.Detail,
	}
}

type PlanWorkspaceActionInput struct {
	WorkspaceID   string
	PlanRoot      string
	Slug          string
	Action        WorkspaceAction
	ConfirmDelete string
	ActorEmail    string
}

func (s *Service) HandlePlanWorkspaceAction(
	ctx context.Context,
	input PlanWorkspaceActionInput,
) error {
	if s.devWorkspaceManager == nil {
		return fmt.Errorf("workspace manager is unavailable")
	}
	slug := strings.TrimSpace(input.Slug)
	if slug == "" {
		return fmt.Errorf("workspace slug is required")
	}
	if err := s.devWorkspaceManager.Refresh(ctx); err != nil {
		return err
	}
	ws, ok := s.devWorkspaceManager.Lookup(slug)
	if !ok {
		return fmt.Errorf("workspace %q was not found", slug)
	}
	switch input.Action {
	case WorkspaceActionStart, WorkspaceActionRetry:
		_, err := s.devWorkspaceManager.Start(ctx, slug)
		return err
	case WorkspaceActionStop:
		_, err := s.devWorkspaceManager.Stop(ctx, slug)
		return err
	case WorkspaceActionMerge:
		return nil
	case WorkspaceActionRefresh:
		return nil
	case WorkspaceActionDelete:
		return s.deleteLinkedWorkspace(ctx, ws, input)
	default:
		return fmt.Errorf("unsupported workspace action %q", input.Action)
	}
}

func (s *Service) deleteLinkedWorkspace(
	ctx context.Context,
	ws workspaces.Workspace,
	input PlanWorkspaceActionInput,
) error {
	if ws.IsMain {
		return fmt.Errorf("main workspace cannot be deleted")
	}
	if strings.TrimSpace(input.ConfirmDelete) != ws.Slug {
		return fmt.Errorf("confirm_delete must match workspace slug")
	}
	stack := stackSummaryFromWorkspaces(workspaces.InspectStack(ctx, ws.CheckoutPath))
	if !stack.Merged || !stack.Available {
		return fmt.Errorf("workspace stack is not safely merged: %s", stack.Detail)
	}
	if ws.Status == workspaces.StatusRunning || ws.Status == workspaces.StatusStarting ||
		ws.Status == workspaces.StatusStopping {
		if _, err := s.devWorkspaceManager.Stop(ctx, ws.Slug); err != nil {
			return err
		}
	}
	trash := filepath.Join(filepath.Dir(ws.CheckoutPath), ".vamos-trash")
	if err := os.MkdirAll(trash, 0o755); err != nil {
		return err
	}
	dest := filepath.Join(
		trash,
		filepath.Base(ws.CheckoutPath)+"-"+time.Now().Format("20060102-150405"),
	)
	if err := os.Rename(ws.CheckoutPath, dest); err != nil {
		return err
	}
	return s.devWorkspaceManager.Refresh(ctx)
}
