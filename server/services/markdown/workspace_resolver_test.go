//go:build !integration || unit
// +build !integration unit

package markdown

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	pkgdb "github.com/CoreyCole/vamos/pkg/db"
	servicedb "github.com/CoreyCole/vamos/server/services/db"
)

func TestResolveWorkspaceForDocumentLongestPrefix(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	thoughtsRoot := filepath.Join(root, "thoughts")
	planRoot := filepath.Join(thoughtsRoot, "creative-mode-agent", "plans", "plan-a")
	nestedRoot := filepath.Join(planRoot, "research")
	docPath := filepath.Join(nestedRoot, "finding.md")
	mustMkdirAll(t, nestedRoot)
	mustWriteFile(t, docPath, []byte("# Finding"))

	queries := newWorkspaceResolverTestQueries(t)
	createResolverWorkspace(t, queries, "workspace-parent", "user@example.com", planRoot)
	createResolverWorkspace(
		t,
		queries,
		"workspace-nested",
		"user@example.com",
		nestedRoot,
	)

	got, err := NewDBWorkspaceResolver(queries, thoughtsRoot).ResolveWorkspaceForDocument(
		ctx,
		"user@example.com",
		"thoughts/creative-mode-agent/plans/plan-a/research/finding.md",
	)
	if err != nil {
		t.Fatalf("ResolveWorkspaceForDocument() error = %v", err)
	}
	if !got.Attached || got.WorkspaceID != "workspace-nested" ||
		got.RelativePath != "finding.md" ||
		got.Ambiguous {
		t.Fatalf(
			"ResolveWorkspaceForDocument() = %+v, want nested workspace attachment",
			got,
		)
	}
}

func TestResolveWorkspaceForDocumentSharedWorkspacesIgnoreUserEmail(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	thoughtsRoot := filepath.Join(root, "thoughts")
	planRoot := filepath.Join(thoughtsRoot, "creative-mode-agent", "plans", "plan-a")
	docPath := filepath.Join(planRoot, "design.md")
	mustMkdirAll(t, planRoot)
	mustWriteFile(t, docPath, []byte("# Design"))

	queries := newWorkspaceResolverTestQueries(t)
	resolver := NewDBWorkspaceResolver(queries, thoughtsRoot)
	got, err := resolver.ResolveWorkspaceForDocument(
		ctx,
		"user@example.com",
		"thoughts/creative-mode-agent/plans/plan-a/design.md",
	)
	if err != nil {
		t.Fatalf("ResolveWorkspaceForDocument(no owner) error = %v", err)
	}
	if got.Attached || got.Ambiguous {
		t.Fatalf("ResolveWorkspaceForDocument(no owner) = %+v, want no attachment", got)
	}

	createResolverWorkspace(t, queries, "workspace-a", "other-user@example.com", planRoot)
	got, err = resolver.ResolveWorkspaceForDocument(
		ctx,
		"user@example.com",
		"thoughts/creative-mode-agent/plans/plan-a/design.md",
	)
	if err != nil {
		t.Fatalf("ResolveWorkspaceForDocument(shared workspace) error = %v", err)
	}
	if !got.Attached || got.WorkspaceID != "workspace-a" || got.Ambiguous {
		t.Fatalf(
			"ResolveWorkspaceForDocument(shared workspace) = %+v, want shared attachment",
			got,
		)
	}

	createResolverWorkspace(t, queries, "workspace-b", "third-user@example.com", planRoot)
	got, err = resolver.ResolveWorkspaceForDocument(
		ctx,
		"user@example.com",
		"thoughts/creative-mode-agent/plans/plan-a/design.md",
	)
	if err != nil {
		t.Fatalf("ResolveWorkspaceForDocument(ambiguous) error = %v", err)
	}
	if got.Attached || !got.Ambiguous {
		t.Fatalf(
			"ResolveWorkspaceForDocument(ambiguous) = %+v, want explicit ambiguity",
			got,
		)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func newWorkspaceResolverTestQueries(t *testing.T) *pkgdb.Queries {
	t.Helper()
	service, err := servicedb.NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return service.Queries
}

func createResolverWorkspace(
	t *testing.T,
	queries *pkgdb.Queries,
	id, userEmail, artifactRoot string,
) {
	t.Helper()
	_, err := queries.CreateWorkspace(context.Background(), pkgdb.CreateWorkspaceParams{
		ID:                id,
		UserEmail:         userEmail,
		Title:             id,
		RootDocPath:       artifactRoot,
		Cwd:               sql.NullString{String: artifactRoot, Valid: true},
		WorkflowType:      "freeform",
		WorkflowStateJson: sql.NullString{String: "{}", Valid: true},
		Source:            "web",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace(%s) error = %v", id, err)
	}
}
