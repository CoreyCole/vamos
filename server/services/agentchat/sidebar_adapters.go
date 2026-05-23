package agentchat

import (
	"strings"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
)

func BuildAgentChatSidebarArgs(args ChatPageArgs) workbench.WorkbenchSidebarArgs {
	return workbench.WorkbenchSidebarArgs{
		ID:         "agent-chat-shared-sidebar",
		DefaultTab: workbench.SidebarTabWorkspaces,
		Tabs:       workbench.DefaultSidebarTabs(),
		Workspaces: workspacesPanelFromChatArgs(args),
		Files: filesPanelFromArtifactTree(
			documentPanelFromArtifact(args.DocPane),
			args.DocPane,
		),
	}
}

func BuildAgentChatWorkspaceSidebarArgs(
	args WorkspacePageArgs,
) workbench.WorkbenchSidebarArgs {
	return workbench.WorkbenchSidebarArgs{
		ID:         "agent-chat-shared-sidebar",
		DefaultTab: workbench.SidebarTabWorkspaces,
		Tabs:       workbench.DefaultSidebarTabs(),
		Workspaces: workspacesPanelFromWorkspaceProjection(args.Projection),
		Files: filesPanelFromArtifactTree(
			documentPanelFromArtifact(args.Projection.Docs),
			args.Projection.Docs,
		),
	}
}

func workspacesPanelFromChatArgs(args ChatPageArgs) workbench.WorkspacesPanelModel {
	model := workbench.WorkspacesPanelModel{EmptyLabel: "No sessions yet."}
	model.Roots = planSidebarRoots(args.PlanSidebar.Nodes)
	model.ThreadGroups = threadSidebarGroups(args.ThreadGroups)
	return model
}

func workspacesPanelFromWorkspaceProjection(
	projection WorkspaceProjection,
) workbench.WorkspacesPanelModel {
	return workbench.WorkspacesPanelModel{
		Roots:        planSidebarRoots(projection.PlanSidebar.Nodes),
		ThreadGroups: threadSidebarGroups(projection.Sidebar.Groups),
		EmptyLabel:   "No workspace sessions yet.",
	}
}

func planSidebarRoots(nodes []PlanSidebarNode) []workbench.WorkspaceRootItem {
	items := make([]workbench.WorkspaceRootItem, 0, len(nodes))
	for _, node := range nodes {
		items = append(items, workbench.WorkspaceRootItem{
			ID:            node.Key,
			Label:         strings.TrimSpace(node.Label),
			Href:          node.Href,
			Active:        node.Active,
			KindLabel:     node.LatestSourceLabel,
			Timestamp:     planSidebarTimestampLabel(node),
			CountLabel:    planSidebarCountLabel(node),
			Children:      planSidebarRoots(node.Children),
			InitiallyOpen: node.Expanded || node.Active,
		})
	}
	return items
}

func threadSidebarGroups(groups []ThreadSidebarGroup) []workbench.WorkspaceThreadGroup {
	out := make([]workbench.WorkspaceThreadGroup, 0, len(groups))
	for _, group := range groups {
		mapped := workbench.WorkspaceThreadGroup{
			ID:          group.Key,
			Label:       group.Label,
			KindLabel:   group.KindLabel,
			Timestamp:   group.Timestamp,
			ThreadCount: group.ThreadCount,
			Active:      group.IsActive,
		}
		for _, thread := range group.Threads {
			mapped.Threads = append(mapped.Threads, workbench.WorkspaceThreadItem{
				ID:          thread.ID,
				Label:       thread.Title,
				Href:        thread.Href,
				Active:      thread.IsActive,
				SourceLabel: thread.SourceLabel,
				CwdLabel:    thread.CwdLabel,
			})
		}
		out = append(out, mapped)
	}
	return out
}

func documentPanelFromArtifact(state DocPaneState) workbench.DocumentPanelModel {
	model := workbench.DocumentPanelModel{CurrentPath: state.Selected.RelativePath}
	for _, section := range state.Selected.Sections {
		model.Sections = append(model.Sections, workbench.DocumentSectionItem{
			ID:    section.ID,
			Title: section.Title,
			Level: 1,
		})
	}
	return model
}

func filesPanelFromArtifactTree(
	document workbench.DocumentPanelModel,
	state DocPaneState,
) workbench.FilesPanelModel {
	return workbench.NewFilesPanelModel(
		state.Selected.RelativePath,
		document,
		artifactTreeItems(state.WorkspaceID, state.Tree),
	)
}

func artifactTreeItems(
	workspaceID string,
	nodes []ArtifactTreeNode,
) []workbench.FileTreeItem {
	out := make([]workbench.FileTreeItem, 0, len(nodes))
	for _, node := range nodes {
		item := workbench.FileTreeItem{
			Name:       node.Name,
			Path:       node.Path,
			IsDir:      node.IsDir,
			IsExpanded: node.IsExpanded,
			IsActive:   node.Selected,
			Children:   artifactTreeItems(workspaceID, node.Children),
		}
		if !node.IsDir && strings.TrimSpace(node.Path) != "" {
			item.FormAction = docSelectAction(workspaceID)
			item.HiddenFields = map[string]string{"artifact_rel_path": node.Path}
		}
		out = append(out, item)
	}
	return out
}
