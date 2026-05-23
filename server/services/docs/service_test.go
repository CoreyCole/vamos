package docs

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/layouts/workbench"
)

type fakeWorkspaceDocLister struct{ rows []db.WorkspaceDoc }

func (f fakeWorkspaceDocLister) ListWorkspaceDocs(
	context.Context,
	string,
) ([]db.WorkspaceDoc, error) {
	return f.rows, nil
}

func TestListWorkspaceDocTreeBuildsNestedActiveTree(t *testing.T) {
	service := NewServiceWithWorkspaceDocs(
		nil,
		nil,
		fakeWorkspaceDocLister{rows: []db.WorkspaceDoc{
			workspaceDoc("ws-1", "plans/demo", ".", "dir", "demo"),
			workspaceDoc("ws-1", "plans/demo/reviews", "reviews", "dir", "reviews"),
			workspaceDoc(
				"ws-1",
				"plans/demo/reviews/review.md",
				"reviews/review.md",
				"file",
				"review.md",
			),
			workspaceDoc(
				"ws-1",
				"plans/demo/design.md",
				"design.md",
				"file",
				"design.md",
			),
		}},
	)
	got, err := service.ListWorkspaceDocTree(
		context.Background(),
		"ws-1",
		DocPath("plans/demo/reviews/review.md"),
	)
	if err != nil {
		t.Fatalf("ListWorkspaceDocTree() error = %v", err)
	}
	if got == nil || got.WorkspaceID != "ws-1" {
		t.Fatalf("ListWorkspaceDocTree() = %#v", got)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].RelPath != "." {
		t.Fatalf("root nodes = %#v", got.Nodes)
	}
	root := got.Nodes[0]
	if !root.IsExpanded {
		t.Fatalf("root should expand around active descendant: %#v", root)
	}
	if labels := childLabels(
		root,
	); !reflect.DeepEqual(
		labels,
		[]string{"design.md", "reviews"},
	) {
		t.Fatalf("root child labels = %#v", labels)
	}
	reviews := root.Children[1]
	if !reviews.IsExpanded || len(reviews.Children) != 1 ||
		!reviews.Children[0].IsActive {
		t.Fatalf("active review branch not expanded/active: %#v", reviews)
	}
}

func workspaceDoc(workspaceID, docPath, relPath, kind, title string) db.WorkspaceDoc {
	return db.WorkspaceDoc{
		WorkspaceID: workspaceID,
		DocPath:     docPath,
		RelPath:     relPath,
		Kind:        kind,
		Title:       title,
		UpdatedAt:   time.Now(),
	}
}

func childLabels(node workbench.WorkspaceDocNode) []string {
	labels := make([]string, 0, len(node.Children))
	for _, child := range node.Children {
		labels = append(labels, child.Label)
	}
	return labels
}
