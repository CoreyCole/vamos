package agentchat

import "github.com/CoreyCole/vamos/server/layouts/workbench"

const (
	workbenchSideRatio        = 0.22
	workbenchPrimaryRatio     = 0.39
	workspacePrimaryRatio     = 0.39
	workspaceContextRatio     = 0.39
	workbenchNavigationMinRem = 14
	workbenchPrimaryMinRem    = 24
	workbenchContextMinRem    = 18
)

func ensureFreeformWorkbenchState(args ChatPageArgs) workbench.WorkbenchState {
	if args.Workbench.Config.Version != 0 {
		return args.Workbench
	}
	state, _ := buildFreeformWorkbenchState(args, nil, "")
	return state
}

func buildFreeformWorkbenchState(
	args ChatPageArgs,
	saved *workbench.WorkbenchConfig,
	routeHref string,
) (workbench.WorkbenchState, error) {
	regions := []workbench.WorkbenchRegion{
		{
			ID:        "agent-chat-navigation",
			Slot:      workbench.WorkbenchSlotNavigation,
			Kind:      workbench.RegionPlanSidebar,
			Ratio:     workbenchSideRatio,
			MinRem:    workbenchNavigationMinRem,
			Visible:   false,
			TargetID:  "agent-chat-navigation-region",
			Title:     "Workspaces",
			Component: workbench.SharedSidebar(BuildAgentChatSidebarArgs(args)),
		},
		{
			ID:        "agent-chat-primary",
			Slot:      workbench.WorkbenchSlotPrimary,
			Kind:      workbench.RegionChat,
			Ratio:     workbenchPrimaryRatio,
			MinRem:    workbenchPrimaryMinRem,
			Visible:   true,
			TargetID:  "agent-chat-chat-pane",
			Component: FreeformChatPrimaryRegion(args),
		},
		{
			ID:        "agent-chat-context",
			Slot:      workbench.WorkbenchSlotContext,
			Kind:      workbench.RegionArtifact,
			Ratio:     workbenchPrimaryRatio,
			MinRem:    workbenchContextMinRem,
			Visible:   false,
			TargetID:  "agent-chat-rail-pane",
			Title:     "Artifacts",
			Component: FreeformContextRegion(args),
		},
	}
	return workbench.BuildWorkbenchState(workbench.BuildWorkbenchStateInput{
		UserEmail:     args.UserEmail,
		Page:          workbench.WorkbenchPageAgentChat,
		View:          workbench.WorkbenchViewFocus,
		ContextMode:   "",
		RouteHref:     routeHref,
		SavedConfig:   saved,
		Regions:       regions,
		FocusDefault:  true,
		NormalRegions: agentChatNormalRegions(false),
	})
}

func ensureWorkspaceWorkbenchState(args WorkspacePageArgs) workbench.WorkbenchState {
	if args.Workbench.Config.Version != 0 {
		return args.Workbench
	}
	state, _ := buildWorkspaceWorkbenchState(args, nil, "")
	return state
}

func buildWorkspaceWorkbenchState(
	args WorkspacePageArgs,
	saved *workbench.WorkbenchConfig,
	routeHref string,
) (workbench.WorkbenchState, error) {
	threadID := getThreadID(args.Projection.SelectedThread)
	if saved == nil {
		cfg := workbench.DefaultWorkbenchConfig(
			workbench.WorkbenchPageAgentChat,
			workbench.WorkbenchViewSplit,
			"chat",
		)
		saved = &cfg
	}
	cfg := *saved
	for i := range cfg.Regions {
		switch cfg.Regions[i].ID {
		case "agent-chat-navigation", "agent-chat-primary", "agent-chat-context":
			cfg.Regions[i].Visible = true
		}
	}
	saved = &cfg
	regions := []workbench.WorkbenchRegion{
		{
			ID:        "agent-chat-navigation",
			Slot:      workbench.WorkbenchSlotNavigation,
			Kind:      workbench.RegionWorkspaceTopology,
			Ratio:     workbenchSideRatio,
			MinRem:    workbenchNavigationMinRem,
			Visible:   true,
			TargetID:  "agent-chat-workspace-topology-region",
			Title:     "Workspace",
			Component: WorkspacePlanSidebarRegion(args),
		},
		{
			ID:        "agent-chat-primary",
			Slot:      workbench.WorkbenchSlotPrimary,
			Kind:      workbench.RegionDoc,
			Ratio:     workspacePrimaryRatio,
			MinRem:    workbenchPrimaryMinRem,
			Visible:   true,
			TargetID:  "agent-chat-doc-region",
			Title:     "Document",
			Component: WorkspaceDocPrimaryRegion(args),
		},
		{
			ID:        "agent-chat-context",
			Slot:      workbench.WorkbenchSlotContext,
			Kind:      workbench.RegionChat,
			Ratio:     workspaceContextRatio,
			MinRem:    workbenchContextMinRem,
			Visible:   true,
			TargetID:  "agent-chat-chat-region",
			Title:     "Chat",
			Component: WorkspaceChatPrimaryRegion(args),
		},
	}
	return workbench.BuildWorkbenchState(workbench.BuildWorkbenchStateInput{
		UserEmail:     args.UserEmail,
		Page:          workbench.WorkbenchPageAgentChat,
		View:          workbench.WorkbenchViewSplit,
		ActivePath:    threadID,
		ContextMode:   "chat",
		RouteHref:     routeHref,
		SavedConfig:   saved,
		Regions:       regions,
		NormalRegions: agentChatNormalRegions(true),
	})
}

func agentChatNormalRegions(contextVisible bool) []workbench.RegionNormalState {
	return []workbench.RegionNormalState{
		{SignalKey: "agent-chat-navigation", Available: true, Visible: true},
		{SignalKey: "agent-chat-primary", Available: true, Visible: true},
		{SignalKey: "agent-chat-context", Available: true, Visible: contextVisible},
	}
}
