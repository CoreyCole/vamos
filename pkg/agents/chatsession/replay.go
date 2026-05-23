package chatsession

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/CoreyCole/vamos/pkg/db"
)

func (s *Service) Snapshot(
	ctx context.Context,
	sessionID string,
) (ChatProjection, error) {
	row, err := s.q.GetChatSessionProjection(ctx, sessionID)
	if err == nil {
		return projectionFromRow(row)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ChatProjection{}, err
	}
	events, err := s.EventsAfter(ctx, sessionID, 0, 100000)
	if err != nil {
		return ChatProjection{}, err
	}
	proj, err := RebuildProjection(events, nil)
	if err != nil {
		return ChatProjection{}, err
	}
	if err := s.saveProjection(ctx, proj); err != nil {
		return ChatProjection{}, err
	}
	return proj, nil
}

func (s *Service) EventsAfter(
	ctx context.Context,
	sessionID string,
	afterSeq, limit int64,
) ([]ChatEvent, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.q.ListChatSessionEventsAfter(ctx, db.ListChatSessionEventsAfterParams{
		SessionID: sessionID,
		AfterSeq:  afterSeq,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	return eventsFromRows(rows), nil
}

func (s *Service) saveProjection(ctx context.Context, proj ChatProjection) error {
	_, err := s.q.UpsertChatSessionProjection(ctx, projectionParams(proj))
	if err != nil {
		return err
	}
	return s.q.UpdateChatSessionProjectionSeq(
		ctx,
		db.UpdateChatSessionProjectionSeqParams{
			ID:                   proj.SessionID,
			CurrentProjectionSeq: proj.LastSeq,
		},
	)
}

func projectionParams(proj ChatProjection) db.UpsertChatSessionProjectionParams {
	return db.UpsertChatSessionProjectionParams{
		SessionID:        proj.SessionID,
		LastSeq:          proj.LastSeq,
		MessagesJson:     string(mustJSON(proj.Messages)),
		RunsJson:         string(mustJSON(proj.Runs)),
		ParticipantsJson: string(mustJSON(proj.Participants)),
		ArtifactsJson:    string(mustJSON(proj.Artifacts)),
		TopologyJson:     string(mustJSON(proj.Tree)),
	}
}

func projectionFromRow(row db.ChatSessionProjection) (ChatProjection, error) {
	proj := ChatProjection{SessionID: row.SessionID, LastSeq: row.LastSeq}
	if err := json.Unmarshal([]byte(row.MessagesJson), &proj.Messages); err != nil {
		return ChatProjection{}, err
	}
	if err := json.Unmarshal([]byte(row.RunsJson), &proj.Runs); err != nil {
		return ChatProjection{}, err
	}
	if err := json.Unmarshal(
		[]byte(row.ParticipantsJson),
		&proj.Participants,
	); err != nil {
		return ChatProjection{}, err
	}
	if err := json.Unmarshal([]byte(row.ArtifactsJson), &proj.Artifacts); err != nil {
		return ChatProjection{}, err
	}
	if err := json.Unmarshal([]byte(row.TopologyJson), &proj.Tree); err != nil {
		return ChatProjection{}, err
	}
	return proj, nil
}

func eventsFromRows(rows []db.ChatSessionEvent) []ChatEvent {
	events := make([]ChatEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, eventFromRow(row))
	}
	return events
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 || string(b) == "null" {
		return json.RawMessage(`[]`)
	}
	return b
}
