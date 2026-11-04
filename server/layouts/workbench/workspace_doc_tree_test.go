package workbench

import (
	"bytes"
	"strings"
	"testing"
)

func TestWorkspaceDocTreeRendersDirectoryButtonsAndFileAnchors(t *testing.T) {
	args := WorkspaceDocTreeArgs{
		WorkspaceID: "ws-1",
		EntryMode:   DocEntryModeAgentChat,
		Nodes: []WorkspaceDocNode{{
			Path:       "plans/demo",
			RelPath:    ".",
			Label:      "demo",
			Kind:       WorkspaceDocKindDir,
			IsExpanded: true,
			Children: []WorkspaceDocNode{{
				Path:     "plans/demo/design.md",
				RelPath:  "design.md",
				Label:    "design.md",
				Kind:     WorkspaceDocKindFile,
				Href:     "/thoughts/plans/demo/design.md",
				IsActive: true,
			}},
		}},
	}
	var buf bytes.Buffer
	if err := WorkspaceDocTree(args).Render(t.Context(), &buf); err != nil {
		t.Fatalf("render error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{
		`id="workspace-doc-tree"`,
		`data-doc-scroll-container="workspace"`,
		`type="button"`,
		`demo`,
		`href="/thoughts/plans/demo/design.md"`,
		`data-current-doc-tree-item="true"`,
		`data-scroll-into-view__smooth__vnearest`,
		`workspaceDocTreeNode_plans_demo`,
		`data-on:click="$workspaceDocTreeNode_plans_demo = !$workspaceDocTreeNode_plans_demo"`,
		`data-show="$workspaceDocTreeNode_plans_demo"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered tree missing %q in:\n%s", want, html)
		}
	}
	if strings.Contains(html, `href="/thoughts/plans/demo"`) {
		t.Fatalf("workspace doc tree should not render directory anchors:\n%s", html)
	}
	for _, notWant := range []string{
		`action="/thoughts/actions/select-document"`,
		"data-on:submit",
		`name="doc_path"`,
	} {
		if strings.Contains(html, notWant) {
			t.Fatalf("rendered tree contains %q in:\n%s", notWant, html)
		}
	}
}
