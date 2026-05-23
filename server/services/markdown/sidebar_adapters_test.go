package markdown

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/layouts/workbench"
)

func TestBuildThoughtsSidebarArgsDefaultsToFilesAndNestsDocument(t *testing.T) {
	args := &PageArgs{
		FilePath: "creative-mode-agent/plans/demo/design.md",
		TableOfContents: []TocItem{{
			ID:    "goal",
			Text:  "Goal",
			Level: 1,
		}},
		ViewerArgs: ViewerArgs{Sections: []Section{{
			ID:    "summary",
			Title: "Summary",
		}}},
		FileTree: []FileTreeNode{{
			Name:     "design.md",
			Path:     "creative-mode-agent/plans/demo/design.md",
			IsActive: true,
		}},
	}

	workspaces := []db.Workspace{
		{
			ID:           "other-workspace",
			Title:        "Other workspace",
			RootDocPath:  "creative-mode-agent/plans/other",
			WorkflowType: "freeform",
			UpdatedAt:    time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
		},
		{
			ID:           "current-workspace",
			Title:        "Current workspace",
			RootDocPath:  "creative-mode-agent/plans/demo",
			Cwd:          sql.NullString{String: "/tmp/demo", Valid: true},
			WorkflowType: "qrspi",
			UpdatedAt:    time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC),
		},
	}
	args.WorkspaceContext.WorkspaceID = "current-workspace"

	sidebar := BuildThoughtsSidebarArgs(args, workspaces)
	if sidebar.DefaultTab != workbench.SidebarTabFiles {
		t.Fatalf(
			"DefaultTab = %q, want %q",
			sidebar.DefaultTab,
			workbench.SidebarTabFiles,
		)
	}
	if got := len(sidebar.Tabs); got != 2 {
		t.Fatalf("len(Tabs) = %d, want 2", got)
	}
	if got := sidebar.Files.Document.TOC[0].Text; got != "Goal" {
		t.Fatalf("Files.Document.TOC[0].Text = %q, want Goal", got)
	}
	if got := sidebar.Files.Document.Sections[0].Title; got != "Summary" {
		t.Fatalf("Files.Document.Sections[0].Title = %q, want Summary", got)
	}
	node := sidebar.Files.Nodes[0]
	if node.Href == "" {
		t.Fatal("thoughts file should keep href fallback")
	}
	if !node.IsActive {
		t.Fatal("thoughts file tree should highlight the current document on first paint")
	}
	if node.FormAction != "@post('/thoughts/actions/select-document', {contentType: 'form'})" {
		t.Fatalf("file FormAction = %q", node.FormAction)
	}
	if got := node.HiddenFields["doc_path"]; got != "creative-mode-agent/plans/demo/design.md" {
		t.Fatalf("doc_path = %q", got)
	}
	if got := len(sidebar.Workspaces.Roots); got != 2 {
		t.Fatalf("len(Workspaces.Roots) = %d, want 2", got)
	}
	if sidebar.Workspaces.Roots[0].Active {
		t.Fatal("non-current workspace should not be active")
	}
	current := sidebar.Workspaces.Roots[1]
	if !current.Active {
		t.Fatal("current workspace should be highlighted")
	}
	if got := sidebar.Workspaces.Roots[0].Href; got != "/thoughts/creative-mode-agent/plans/other" {
		t.Fatalf("other workspace href = %q, want thoughts root link", got)
	}
	if current.Href != "/thoughts/creative-mode-agent/plans/demo" {
		t.Fatalf("current workspace href = %q, want thoughts root link", current.Href)
	}
	if current.Metadata[0].Value != "creative-mode-agent/plans/demo" {
		t.Fatalf("current workspace root metadata = %#v", current.Metadata)
	}
}

func TestBuildThoughtsDirectorySidebarArgsUsesDirectorySelectionForms(t *testing.T) {
	folderSidebar := BuildThoughtsDirectorySidebarArgs(&DirectoryArgs{
		Path: "creative-mode-agent/plans/demo",
		FileTree: []FileTreeNode{{
			Name:  "demo",
			Path:  "creative-mode-agent/plans/demo",
			IsDir: true,
		}},
	})
	folder := folderSidebar.Files.Nodes[0]
	if folder.FormAction != "@post('/thoughts/actions/select-directory', {contentType: 'form'})" {
		t.Fatalf("dir FormAction = %q", folder.FormAction)
	}
	if got := folder.HiddenFields["dir_path"]; got != "creative-mode-agent/plans/demo" {
		t.Fatalf("dir_path = %q", got)
	}
}

func TestThoughtsWorkspacesRowsUseThoughtsHrefs(t *testing.T) {
	sidebar := BuildThoughtsWorkspacesPanelModel([]db.Workspace{{
		ID:          "ws-1",
		Title:       "Demo workspace",
		RootDocPath: "owner/plans/demo",
		UpdatedAt:   time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC),
	}}, "ws-1")

	if got := sidebar.Roots[0].Href; got != "/thoughts/owner/plans/demo" {
		t.Fatalf("workspace href = %q, want thoughts root link", got)
	}
	if strings.Contains(sidebar.Roots[0].Href, "/agent-chat") {
		t.Fatalf(
			"workspace href = %q, must not target retired agent-chat pages",
			sidebar.Roots[0].Href,
		)
	}
}

func TestBuildThoughtsWorkspacesPanelModelBlankRootFallsBackToThoughts(t *testing.T) {
	sidebar := BuildThoughtsWorkspacesPanelModel([]db.Workspace{{
		ID:        "blank-root",
		Title:     "Blank root",
		UpdatedAt: time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC),
	}}, "")

	if got := sidebar.Roots[0].Href; got != "/thoughts/" {
		t.Fatalf("blank root workspace href = %q, want /thoughts/", got)
	}
}

func TestBuildThoughtsDirectorySidebarArgsUsesFilesWithEmptyDocument(t *testing.T) {
	sidebar := BuildThoughtsDirectorySidebarArgs(&DirectoryArgs{
		Path: "creative-mode-agent/plans/demo",
		FileTree: []FileTreeNode{{
			Name:  "design.md",
			Path:  "creative-mode-agent/plans/demo/design.md",
			IsDir: false,
		}},
	})

	if sidebar.DefaultTab != workbench.SidebarTabFiles {
		t.Fatalf(
			"DefaultTab = %q, want %q",
			sidebar.DefaultTab,
			workbench.SidebarTabFiles,
		)
	}
	if got := len(sidebar.Tabs); got != 2 {
		t.Fatalf("len(Tabs) = %d, want 2", got)
	}
	if got := sidebar.Files.Document.CurrentPath; got != "creative-mode-agent/plans/demo" {
		t.Fatalf("Files.Document.CurrentPath = %q, want directory path", got)
	}
	if len(sidebar.Files.Document.TOC) != 0 || len(sidebar.Files.Document.Sections) != 0 {
		t.Fatalf(
			"directory document = %#v, want no outline content",
			sidebar.Files.Document,
		)
	}
}
