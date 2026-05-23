package agentchat

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

type DocumentWorkspaceOpenResult struct {
	Workspace db.Workspace
	RelPath   string
}

func (s *Service) OpenDocumentWorkspace(
	ctx context.Context,
	userEmail, documentPath string,
) (DocumentWorkspaceOpenResult, error) {
	if strings.TrimSpace(userEmail) == "" {
		return DocumentWorkspaceOpenResult{}, errors.New("user email is required")
	}
	docAbs, err := s.resolveThoughtsDocPath(documentPath)
	if err != nil {
		return DocumentWorkspaceOpenResult{}, err
	}
	info, err := os.Stat(docAbs)
	if err != nil {
		return DocumentWorkspaceOpenResult{}, err
	}
	if info.IsDir() {
		return DocumentWorkspaceOpenResult{}, errors.New("document path is a directory")
	}

	root := s.nearestAgentContextRoot(docAbs)
	if root == "" {
		root = filepath.Dir(docAbs)
	}
	root, err = ValidateWorkspaceRootDocPath(root, s.thoughtsRoot, userEmail)
	if err != nil {
		return DocumentWorkspaceOpenResult{}, err
	}
	rel, err := filepath.Rel(root, docAbs)
	if err != nil {
		return DocumentWorkspaceOpenResult{}, err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "../") || rel == ".." {
		return DocumentWorkspaceOpenResult{}, errors.New(
			"document path escapes workspace root",
		)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DocumentWorkspaceOpenResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	q := s.queries.WithTx(tx)

	workspace, err := q.FindWorkspaceByRootDocPathForUser(
		ctx,
		db.FindWorkspaceByRootDocPathForUserParams{
			UserEmail:    userEmail,
			RootDocPath: root,
		},
	)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return DocumentWorkspaceOpenResult{}, err
		}
		workspaceID, err := NewWorkspaceID()
		if err != nil {
			return DocumentWorkspaceOpenResult{}, err
		}
		workspace, err = q.CreateWorkspace(ctx, db.CreateWorkspaceParams{
			ID:        workspaceID,
			UserEmail: userEmail,
			Title: validateWorkspaceTitle(
				"Chat: " + filepath.Base(docAbs),
			),
			RootDocPath:         root,
			Cwd:                  nullString(root),
			WorkflowType:         string(WorkspaceWorkflowFreeform),
			WorkflowStateJson:    sql.NullString{},
			Source:               string(WorkspaceSourceWeb),
			SelectedThreadID:     sql.NullString{},
			SelectedDocPath: nullString(rel),
		})
		if err != nil {
			return DocumentWorkspaceOpenResult{}, err
		}
	} else if err := q.UpdateWorkspaceSelectedDoc(ctx, db.UpdateWorkspaceSelectedDocParams{
		ID:                   workspace.ID,
		SelectedDocPath: nullString(rel),
	}); err != nil {
		return DocumentWorkspaceOpenResult{}, err
	}

	if _, err := s.AppendWorkspaceEvent(ctx, q, AppendWorkspaceEventInput{
		WorkspaceID:  workspace.ID,
		EventType:    "doc_selected",
		ActorEmail:   userEmail,
		ActorType:    "user",
		DocPath: rel,
		EventKey:     "doc_selected:" + rel,
	}); err != nil {
		return DocumentWorkspaceOpenResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return DocumentWorkspaceOpenResult{}, err
	}
	return DocumentWorkspaceOpenResult{Workspace: workspace, RelPath: rel}, nil
}

func (s *Service) resolveThoughtsDocPath(documentPath string) (string, error) {
	path := strings.TrimSpace(documentPath)
	if path == "" {
		return "", errors.New("document path is required")
	}
	base, err := resolveWorkspacePath(s.thoughtsRoot)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(filepath.ToSlash(path), "thoughts/") {
		path = strings.TrimPrefix(filepath.ToSlash(path), "thoughts/")
	}
	var candidate string
	if filepath.IsAbs(path) {
		candidate = path
	} else {
		candidate = filepath.Join(base, filepath.FromSlash(path))
	}
	resolved, err := resolveWorkspacePath(candidate)
	if err != nil {
		return "", err
	}
	if !pathWithinRoot(resolved, base) {
		return "", errors.New("document path is outside thoughts root")
	}
	return resolved, nil
}

func (s *Service) runDocumentContext(ctx context.Context, run db.AgentRun) string {
	workspaceID := strings.TrimSpace(run.WorkspaceID.String)
	if !run.WorkspaceID.Valid || workspaceID == "" {
		return ""
	}
	workspace, err := s.queries.GetWorkspace(ctx, workspaceID)
	if err != nil || !workspace.SelectedDocPath.Valid {
		return ""
	}
	rel, err := ValidateWorkspaceRelPath(
		run.RootDocPath,
		workspace.SelectedDocPath.String,
	)
	if err != nil {
		return ""
	}
	absPath := filepath.Join(run.RootDocPath, filepath.FromSlash(rel))
	return "The user is chatting from an open document in the doc pane. " +
		"Before answering, read this file so the document and local AGENTS.md " +
		"instructions are loaded through normal tool context.\n\n" +
		"File: `" + absPath + "`"
}

func (s *Service) nearestAgentContextRoot(documentAbs string) string {
	base, err := resolveWorkspacePath(s.thoughtsRoot)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(documentAbs)
	for {
		for _, name := range []string{"AGENTS.md", "agents.md"} {
			if info, err := os.Stat(
				filepath.Join(dir, name),
			); err == nil &&
				!info.IsDir() {
				return dir
			}
		}
		if sameFilesystemPath(dir, base) || !pathWithinRoot(dir, base) {
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
