package agentchat

import (
	"context"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

func (s *Service) BuildThreadMetadataView(
	ctx context.Context,
	workspaceContext ThreadWorkspaceContext,
	piCwd string,
) ThreadMetadataView {
	thread := workspaceContext.Thread
	view := ThreadMetadataView{
		ThreadID:  thread.ID,
		Title:     thread.Title,
		URL:       threadThoughtsURL(workspaceContext),
		ProjectID: strings.TrimSpace(thread.ProjectID),
		ThreadCwd: thread.Cwd,
		PiCwd:     firstNonEmpty(piCwd, thread.Cwd),
		Related:   make([]ThreadWorkspaceView, 0, len(workspaceContext.Related)),
	}
	if workspaceContext.Primary != nil {
		primary := threadWorkspaceView(*workspaceContext.Primary, ThreadWorkspaceRolePrimary)
		view.Primary = &primary
		view.NewTargets = append(view.NewTargets, ThreadNewTargetView{
			Kind:        NewThreadTargetPrimary,
			WorkspaceID: primary.WorkspaceID,
			Label:       "New in primary workspace",
			Selected:    true,
		})
	}
	for _, workspace := range workspaceContext.Related {
		related := threadWorkspaceView(workspace, ThreadWorkspaceRoleRelated)
		view.Related = append(view.Related, related)
		view.NewTargets = append(view.NewTargets, ThreadNewTargetView{
			Kind:        NewThreadTargetRelated,
			WorkspaceID: related.WorkspaceID,
			Label:       "Related: " + related.Label,
		})
	}
	view.NewTargets = append(view.NewTargets, ThreadNewTargetView{
		Kind:  NewThreadTargetFreeform,
		Label: "Freeform blank thread",
	})
	if text, err := s.latestAssistantTextForThread(ctx, thread); err == nil {
		if impl := qrspiImplementationWorkspaceFromText(extractFirstQRSPIResultXML(text)); impl != "" {
			view.ImplementationWorkspace = impl
			view.PiCwd = impl
		}
	}
	return view
}

func threadThoughtsURL(workspaceContext ThreadWorkspaceContext) string {
	return BuildThoughtsChatDocURL(EmbeddedChatURLState{
		Context:  ThoughtsChatContext,
		ThreadID: workspaceContext.Thread.ID,
	})
}

func threadWorkspaceView(workspace db.Workspace, role ThreadWorkspaceRole) ThreadWorkspaceView {
	workflowType := strings.TrimSpace(workspace.WorkflowType)
	return ThreadWorkspaceView{
		WorkspaceID:  workspace.ID,
		Label:        firstNonEmpty(strings.TrimSpace(workspace.Title), planWorkspaceLabel(workspace.RootDocPath), workspace.ID),
		RootDocPath:  workspace.RootDocPath,
		WorkflowType: workspaceWorkflowSidebarLabel(workflowType),
		Lifecycle:    workspaceLifecycleLabel(workspace),
		Role:         string(role),
	}
}

func workspaceLifecycleLabel(workspace db.Workspace) string {
	if workspace.ArchivedAt.Valid {
		return "archived"
	}
	if strings.TrimSpace(workspace.WorkflowStateJson.String) != "" {
		return "active"
	}
	return "ready"
}
