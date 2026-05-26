package agentchat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

func (s *Service) GetThreadWorkspaceContext(ctx context.Context, userEmail, threadID string) (ThreadWorkspaceContext, error) {
	thread, err := s.queries.GetAgentThreadForUser(ctx, db.GetAgentThreadForUserParams{
		ID:        strings.TrimSpace(threadID),
		UserEmail: userEmail,
	})
	if err != nil {
		return ThreadWorkspaceContext{}, err
	}
	rows, err := s.queries.ListThreadWorkspaceAssociations(ctx, thread.ID)
	if err != nil {
		return ThreadWorkspaceContext{}, err
	}
	out := ThreadWorkspaceContext{Thread: thread, Related: []db.Workspace{}}
	for _, row := range rows {
		workspace := db.Workspace{
			ID:                row.WorkspaceID,
			UserEmail:         row.UserEmail,
			Title:             row.Title,
			RootDocPath:       row.RootDocPath,
			Cwd:               row.Cwd,
			WorkflowType:      row.WorkflowType,
			WorkflowStateJson: row.WorkflowStateJson,
			Source:            row.Source,
			SelectedThreadID:  row.SelectedThreadID,
			SelectedDocPath:   row.SelectedDocPath,
			CurrentSessionID:  row.CurrentSessionID,
			CurrentBranchID:   row.CurrentBranchID,
			CreatedAt:         row.CreatedAt_2,
			UpdatedAt:         row.UpdatedAt,
			ArchivedAt:        row.ArchivedAt,
		}
		if row.IsPrimary == 1 {
			copy := workspace
			out.Primary = &copy
			continue
		}
		out.Related = append(out.Related, workspace)
	}
	return out, nil
}

func (s *Service) SetThreadPrimaryWorkspace(ctx context.Context, threadID, workspaceID, source string) error {
	threadID = strings.TrimSpace(threadID)
	workspaceID = strings.TrimSpace(workspaceID)
	if threadID == "" || workspaceID == "" {
		return fmt.Errorf("thread id and workspace id are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := s.queries.WithTx(tx)
	if err := q.DemoteThreadPrimaryWorkspaces(ctx, threadID); err != nil {
		return err
	}
	if err := q.UpsertThreadWorkspaceAssociation(ctx, db.UpsertThreadWorkspaceAssociationParams{
		ThreadID:    threadID,
		WorkspaceID: workspaceID,
		IsPrimary:   1,
		Role:        string(ThreadWorkspaceRolePrimary),
		AdoptedFrom: strings.TrimSpace(source),
	}); err != nil {
		return err
	}
	if err := s.attachThreadRunsAndSessionsToWorkspace(ctx, tx, threadID, workspaceID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) AddThreadRelatedWorkspace(ctx context.Context, threadID, workspaceID, source string) error {
	return s.queries.UpsertThreadWorkspaceAssociation(ctx, db.UpsertThreadWorkspaceAssociationParams{
		ThreadID:    strings.TrimSpace(threadID),
		WorkspaceID: strings.TrimSpace(workspaceID),
		IsPrimary:   0,
		Role:        string(ThreadWorkspaceRoleRelated),
		AdoptedFrom: strings.TrimSpace(source),
	})
}

func (s *Service) AttachThreadRunsAndSessionsToPrimary(ctx context.Context, threadID, workspaceID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.attachThreadRunsAndSessionsToWorkspace(ctx, tx, threadID, workspaceID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) ResolvePrimaryWorkspaceForThread(ctx context.Context, userEmail, threadID string) (db.Workspace, bool, error) {
	workspace, err := s.queries.GetPrimaryWorkspaceForThread(ctx, db.GetPrimaryWorkspaceForThreadParams{
		ThreadID:  strings.TrimSpace(threadID),
		UserEmail: strings.TrimSpace(userEmail),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.Workspace{}, false, nil
		}
		return db.Workspace{}, false, err
	}
	return workspace, true, nil
}

func (s *Service) threadHasWorkspaceAssociation(ctx context.Context, threadID, workspaceID string) (bool, error) {
	threadID = strings.TrimSpace(threadID)
	workspaceID = strings.TrimSpace(workspaceID)
	if threadID == "" || workspaceID == "" {
		return false, nil
	}
	return s.queries.ThreadHasWorkspaceAssociation(ctx, db.ThreadHasWorkspaceAssociationParams{ThreadID: threadID, WorkspaceID: workspaceID})
}
