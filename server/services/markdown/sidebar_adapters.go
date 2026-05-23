package markdown

import (
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/layouts/workbench"
)

func BuildThoughtsSidebarArgs(
	args *PageArgs,
	workspaceLists ...[]db.Workspace,
) workbench.WorkbenchSidebarArgs {
	workspaces := firstWorkspaceList(workspaceLists)
	document := BuildDocumentPanelModel(
		args.FilePath,
		args.TableOfContents,
		args.ViewerArgs.Sections,
	)
	return workbench.WorkbenchSidebarArgs{
		ID:         "thoughts-shared-sidebar",
		DefaultTab: workbench.SidebarTabFiles,
		Tabs:       workbench.DefaultSidebarTabs(),
		Workspaces: BuildThoughtsWorkspacesPanelModel(
			workspaces,
			args.WorkspaceContext.WorkspaceID,
		),
		Files: BuildFilesPanelModel(args.FilePath, document, args.FileTree),
	}
}

func BuildThoughtsDirectorySidebarArgs(
	args *DirectoryArgs,
	workspaceLists ...[]db.Workspace,
) workbench.WorkbenchSidebarArgs {
	workspaces := firstWorkspaceList(workspaceLists)
	return workbench.WorkbenchSidebarArgs{
		ID:         "thoughts-shared-sidebar",
		DefaultTab: workbench.SidebarTabFiles,
		Tabs:       workbench.DefaultSidebarTabs(),
		Workspaces: BuildThoughtsWorkspacesPanelModel(workspaces, ""),
		Files: BuildFilesPanelModel(
			args.Path,
			workbench.DocumentPanelModel{CurrentPath: args.Path},
			args.FileTree,
		),
	}
}

func firstWorkspaceList(workspaceLists [][]db.Workspace) []db.Workspace {
	if len(workspaceLists) == 0 {
		return nil
	}
	return workspaceLists[0]
}

func BuildThoughtsWorkspacesPanelModel(
	workspaces []db.Workspace,
	currentWorkspaceID string,
) workbench.WorkspacesPanelModel {
	if len(workspaces) == 0 {
		return workbench.WorkspacesPanelModel{
			EmptyLabel: "No workspaces yet.",
		}
	}
	currentWorkspaceID = strings.TrimSpace(currentWorkspaceID)
	roots := make([]workbench.WorkspaceRootItem, 0, len(workspaces))
	for _, workspace := range workspaces {
		label := strings.TrimSpace(workspace.Title)
		if label == "" {
			label = strings.TrimSpace(workspace.RootDocPath)
		}
		if label == "" {
			label = "Workspace"
		}
		roots = append(roots, workbench.WorkspaceRootItem{
			ID:            workspace.ID,
			Label:         label,
			Href:          workspaceThoughtsHref(workspace),
			Active:        workspace.ID == currentWorkspaceID,
			KindLabel:     workspace.WorkflowType,
			Timestamp:     workspace.UpdatedAt.Format("Jan 2 15:04"),
			Metadata:      workspaceMetadata(workspace),
			InitiallyOpen: workspace.ID == currentWorkspaceID,
		})
	}
	return workbench.WorkspacesPanelModel{
		Roots:         roots,
		CurrentRootID: currentWorkspaceID,
	}
}

func workspaceThoughtsHref(workspace db.Workspace) string {
	root := strings.TrimSpace(workspace.RootDocPath)
	if root == "" {
		return "/thoughts/"
	}
	return thoughtsHref(root)
}

func workspaceMetadata(workspace db.Workspace) []workbench.WorkspaceMetadataItem {
	items := make([]workbench.WorkspaceMetadataItem, 0, 2)
	if root := strings.TrimSpace(workspace.RootDocPath); root != "" {
		items = append(items, workbench.WorkspaceMetadataItem{Label: "Root", Value: root})
	}
	if cwd := strings.TrimSpace(workspace.Cwd.String); workspace.Cwd.Valid && cwd != "" {
		items = append(items, workbench.WorkspaceMetadataItem{Label: "CWD", Value: cwd})
	}
	return items
}

func BuildDocumentPanelModel(
	path string,
	toc []TocItem,
	sections []Section,
) workbench.DocumentPanelModel {
	model := workbench.DocumentPanelModel{CurrentPath: path}
	for _, item := range toc {
		model.TOC = append(
			model.TOC,
			workbench.DocumentTOCItem{ID: item.ID, Text: item.Text, Level: item.Level},
		)
	}
	for _, section := range sections {
		model.Sections = append(
			model.Sections,
			workbench.DocumentSectionItem{ID: section.ID, Title: section.Title, Level: 1},
		)
	}
	return model
}

func BuildFilesPanelModel(
	currentPath string,
	document workbench.DocumentPanelModel,
	nodes []FileTreeNode,
) workbench.FilesPanelModel {
	return workbench.NewFilesPanelModel(
		currentPath,
		document,
		buildFileTreeItems(nodes),
	)
}

func buildFileTreeItems(nodes []FileTreeNode) []workbench.FileTreeItem {
	out := make([]workbench.FileTreeItem, 0, len(nodes))
	for _, node := range nodes {
		action, fields := thoughtsSelectionAction(node)
		item := workbench.FileTreeItem{
			Name:         node.Name,
			Path:         node.Path,
			Href:         thoughtsHref(node.Path),
			FormAction:   action,
			HiddenFields: fields,
			IsDir:        node.IsDir,
			IsExpanded:   node.IsExpanded,
			IsActive:     node.IsActive,
			Children:     buildFileTreeItems(node.Children),
		}
		out = append(out, item)
	}
	return out
}

func thoughtsSelectionAction(node FileTreeNode) (string, map[string]string) {
	if node.IsDir {
		return "@post('/thoughts/actions/select-directory', {contentType: 'form'})", map[string]string{
			"dir_path": node.Path,
		}
	}
	return "@post('/thoughts/actions/select-document', {contentType: 'form'})", map[string]string{
		"doc_path": node.Path,
	}
}

func BuildWorkspaceDocTreeArgs(
	workspaceID string,
	currentPath string,
	entryMode workbench.DocEntryMode,
	rows []db.WorkspaceDoc,
) *workbench.WorkspaceDocTreeArgs {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil
	}
	args := &workbench.WorkspaceDocTreeArgs{
		WorkspaceID:  workspaceID,
		CurrentPath:  currentPath,
		EntryMode:    entryMode,
		Nodes:        buildWorkspaceDocTreeNodes(rows, currentPath),
		EmptyMessage: "Workspace docs will appear after the workspace sync runs.",
	}
	if len(rows) > 0 {
		args.RootPath = rows[0].DocPath
	}
	return args
}

func buildWorkspaceDocTreeNodes(
	rows []db.WorkspaceDoc,
	current string,
) []workbench.WorkspaceDocNode {
	current = strings.Trim(strings.TrimSpace(path.Clean("/"+current)), "/")
	type nodeRef struct {
		node     *workbench.WorkspaceDocNode
		children []string
	}
	refs := make(map[string]*nodeRef, len(rows))
	for _, row := range rows {
		rel := cleanWorkspaceDocRelPath(row.RelPath)
		label := strings.TrimSpace(row.Title)
		if label == "" {
			label = path.Base(rel)
			if rel == "." {
				label = path.Base(strings.Trim(row.DocPath, "/"))
			}
		}
		kind := workbench.WorkspaceDocKind(row.Kind)
		if kind != workbench.WorkspaceDocKindDir {
			kind = workbench.WorkspaceDocKindFile
		}
		docPath := strings.Trim(strings.TrimSpace(row.DocPath), "/")
		refs[rel] = &nodeRef{node: &workbench.WorkspaceDocNode{
			Path:     docPath,
			RelPath:  rel,
			Label:    label,
			Kind:     kind,
			IsActive: docPath == current,
		}}
	}
	roots := make([]string, 0)
	for rel, ref := range refs {
		parent := path.Dir(rel)
		if rel == "." {
			roots = append(roots, rel)
			continue
		}
		if parent == "." {
			if refs[parent] != nil {
				refs[parent].children = append(refs[parent].children, rel)
				continue
			}
			roots = append(roots, rel)
			continue
		}
		if refs[parent] == nil {
			roots = append(roots, rel)
			continue
		}
		refs[parent].children = append(refs[parent].children, rel)
		if ref.node.IsActive {
			refs[parent].node.IsExpanded = true
		}
	}
	var build func(rel string) workbench.WorkspaceDocNode
	build = func(rel string) workbench.WorkspaceDocNode {
		ref := refs[rel]
		sort.Strings(ref.children)
		for _, childRel := range ref.children {
			child := build(childRel)
			if child.IsActive || child.IsExpanded {
				ref.node.IsExpanded = true
			}
			ref.node.Children = append(ref.node.Children, child)
		}
		if len(ref.node.Children) > 0 {
			ref.node.IsExpanded = ref.node.IsExpanded ||
				ref.node.Kind == workbench.WorkspaceDocKindDir
		}
		return *ref.node
	}
	sort.Strings(roots)
	out := make([]workbench.WorkspaceDocNode, 0, len(roots))
	for _, root := range roots {
		out = append(out, build(root))
	}
	return out
}

func cleanWorkspaceDocRelPath(rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return "."
	}
	return strings.Trim(strings.TrimSpace(path.Clean("/"+rel)), "/")
}

func thoughtsHref(path string) string {
	path = strings.TrimPrefix(strings.TrimSpace(path), "/")
	if path == "" {
		return "/thoughts/"
	}
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return "/thoughts/" + strings.Join(parts, "/")
}
