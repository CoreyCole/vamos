package markdown

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

const workspaceResolverListLimit = 500

type DBWorkspaceResolver struct {
	queries      *db.Queries
	thoughtsRoot string
}

func NewDBWorkspaceResolver(
	queries *db.Queries,
	thoughtsRoot string,
) *DBWorkspaceResolver {
	return &DBWorkspaceResolver{queries: queries, thoughtsRoot: thoughtsRoot}
}

func (r *DBWorkspaceResolver) ListWorkspaces(
	ctx context.Context,
	limit int64,
) ([]db.Workspace, error) {
	if r == nil || r.queries == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}
	return r.queries.ListWorkspaces(ctx, limit)
}

func (r *DBWorkspaceResolver) ListWorkspaceDocs(
	ctx context.Context,
	workspaceID string,
) ([]db.WorkspaceDoc, error) {
	if r == nil || r.queries == nil || strings.TrimSpace(workspaceID) == "" {
		return nil, nil
	}
	return r.queries.ListWorkspaceDocs(ctx, workspaceID)
}

func (r *DBWorkspaceResolver) ResolveWorkspaceForDocument(
	ctx context.Context,
	userEmail, documentPath string,
) (DocumentWorkspaceContext, error) {
	if r == nil || r.queries == nil {
		return DocumentWorkspaceContext{}, nil
	}
	docAbs, err := r.resolveDocPath(documentPath)
	if err != nil {
		return DocumentWorkspaceContext{}, err
	}
	_ = userEmail
	rows, err := r.queries.ListWorkspaces(ctx, workspaceResolverListLimit)
	if err != nil {
		return DocumentWorkspaceContext{}, err
	}

	var matches []db.Workspace
	bestLen := -1
	for _, workspace := range rows {
		root, err := resolveExistingOrCleanPath(workspace.RootDocPath)
		if err != nil || !pathWithinRoot(docAbs, root) {
			continue
		}
		score := len(root)
		if score > bestLen {
			matches = []db.Workspace{workspace}
			bestLen = score
		} else if score == bestLen {
			matches = append(matches, workspace)
		}
	}
	if len(matches) == 0 {
		return DocumentWorkspaceContext{}, nil
	}
	if len(matches) > 1 {
		return DocumentWorkspaceContext{Ambiguous: true}, nil
	}

	root, err := resolveExistingOrCleanPath(matches[0].RootDocPath)
	if err != nil {
		return DocumentWorkspaceContext{}, err
	}
	rel, err := filepath.Rel(root, docAbs)
	if err != nil {
		return DocumentWorkspaceContext{}, err
	}
	return DocumentWorkspaceContext{
		WorkspaceID:  matches[0].ID,
		RootDocPath:  matches[0].RootDocPath,
		RelativePath: filepath.ToSlash(rel),
		Attached:     true,
	}, nil
}

func (r *DBWorkspaceResolver) resolveDocPath(documentPath string) (string, error) {
	path := strings.TrimSpace(documentPath)
	if path == "" {
		return "", fmt.Errorf("document path is required")
	}
	if strings.HasPrefix(path, "thoughts/") {
		path = strings.TrimPrefix(path, "thoughts/")
	}
	if filepath.IsAbs(path) {
		return resolveExistingOrCleanPath(path)
	}
	base := strings.TrimSpace(r.thoughtsRoot)
	if base == "" {
		return "", fmt.Errorf("thoughts root is required")
	}
	return resolveExistingOrCleanPath(filepath.Join(base, filepath.FromSlash(path)))
}

func resolveExistingOrCleanPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		abs = resolved
	}
	return filepath.Clean(abs), nil
}

func pathWithinRoot(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
		rel != ".."
}
