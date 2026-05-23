package chatsession

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

type WorkspaceLoadInput struct {
	URLSessionID           string
	UserLastSelectedID     string
	WorkspaceCurrentID     string
	RunningSessionID       string
	LatestUpdatedSessionID string
}

func (s *Service) ActivePath(
	ctx context.Context,
	workspaceID string,
	currentSessionID string,
) ([]db.ChatSession, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	sessionID := strings.TrimSpace(currentSessionID)
	if sessionID == "" {
		return nil, nil
	}
	path := []db.ChatSession{}
	seen := map[string]struct{}{}
	for sessionID != "" {
		if _, ok := seen[sessionID]; ok {
			break
		}
		seen[sessionID] = struct{}{}
		session, err := s.q.GetChatSession(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if workspaceID != "" && strings.TrimSpace(session.WorkspaceID) != workspaceID {
			return nil, errors.New("chat session does not belong to workspace")
		}
		path = append(path, session)
		if !session.ParentSessionID.Valid {
			break
		}
		sessionID = strings.TrimSpace(session.ParentSessionID.String)
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path, nil
}

func SelectionForWorkspaceLoad(
	input WorkspaceLoadInput,
) (sessionID, reason string, err error) {
	choices := []struct {
		id     string
		reason string
	}{
		{input.URLSessionID, "url"},
		{input.UserLastSelectedID, "user_last_selected"},
		{input.WorkspaceCurrentID, "workspace_current"},
		{input.RunningSessionID, "running"},
		{input.LatestUpdatedSessionID, "latest_updated"},
	}
	for _, choice := range choices {
		if id := strings.TrimSpace(choice.id); id != "" {
			return id, choice.reason, nil
		}
	}
	return "", "empty", nil
}

func (s *Service) PromoteSession(
	ctx context.Context,
	workspaceID string,
	sessionID string,
	actorEmail string,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	session, err := q.GetChatSession(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return err
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		workspaceID = session.WorkspaceID
	}
	if strings.TrimSpace(session.WorkspaceID) != workspaceID {
		return errors.New("chat session does not belong to workspace")
	}
	if err := q.UpdateWorkspaceCurrentSession(ctx, db.UpdateWorkspaceCurrentSessionParams{
		ID:               workspaceID,
		CurrentSessionID: sql.NullString{String: session.ID, Valid: true},
		CurrentBranchID: sql.NullString{
			String: session.BranchID,
			Valid:  strings.TrimSpace(session.BranchID) != "",
		},
	}); err != nil {
		return err
	}
	_, err = appendEventTx(ctx, q, AppendEventInput{
		SessionID:          session.ID,
		EventType:          EventPromoted,
		ActorParticipantID: actorEmail,
		PayloadJSON: marshalPayload(map[string]any{
			"workspace_id": workspaceID,
			"session_id":   session.ID,
			"actor":        strings.TrimSpace(actorEmail),
			"summary":      "Promoted to active path",
		}),
	})
	if err != nil {
		return err
	}
	return tx.Commit()
}
