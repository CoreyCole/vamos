package agentchat

import (
	"context"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Service) AppendWorkspaceEvent(
	ctx context.Context,
	q *db.Queries,
	input AppendWorkspaceEventInput,
) (db.WorkspaceEvent, error) {
	actorType := strings.TrimSpace(input.ActorType)
	if actorType == "" {
		actorType = "system"
	}
	event, err := q.CreateWorkspaceEvent(ctx, db.CreateWorkspaceEventParams{
		WorkspaceID: strings.TrimSpace(input.WorkspaceID),
		EventType:   strings.TrimSpace(input.EventType),
		ActorEmail:  nullString(input.ActorEmail),
		ActorType:   actorType,
		ThreadID:    nullString(input.ThreadID),
		SessionID:   nullString(input.SessionID),
		RunID:       nullString(input.RunID),
		DocPath: nullString(
			firstNonEmpty(input.DocPath, input.ArtifactRelPath),
		),
		CommentID:   nullString(input.CommentID),
		PayloadJson: nullString(input.PayloadJSON),
		EventKey:    nullString(input.EventKey),
	})
	if err == nil {
		return event, nil
	}
	if input.EventKey != "" && isUniqueConstraintError(err) {
		return q.GetWorkspaceEventByKey(ctx, db.GetWorkspaceEventByKeyParams{
			WorkspaceID: strings.TrimSpace(input.WorkspaceID),
			EventKey:    nullString(input.EventKey),
		})
	}
	return db.WorkspaceEvent{}, err
}
