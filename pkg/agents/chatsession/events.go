package chatsession

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

func (s *Service) AppendEvent(
	ctx context.Context,
	input AppendEventInput,
) (ChatEvent, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ChatEvent{}, err
	}
	defer tx.Rollback()

	event, err := appendEventTx(ctx, s.q.WithTx(tx), input)
	if err != nil {
		return ChatEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return ChatEvent{}, err
	}
	return event, nil
}

func AppendEventWithQueries(
	ctx context.Context,
	q *db.Queries,
	input AppendEventInput,
) (ChatEvent, error) {
	return appendEventTx(ctx, q, input)
}

func appendEventTx(
	ctx context.Context,
	q *db.Queries,
	input AppendEventInput,
) (ChatEvent, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	if err := q.EnsureChatSessionSequence(ctx, sessionID); err != nil {
		return ChatEvent{}, err
	}
	seq, err := q.ReserveChatSessionSeq(ctx, sessionID)
	if err != nil {
		return ChatEvent{}, err
	}
	row, err := q.AppendChatSessionEvent(ctx, db.AppendChatSessionEventParams{
		SessionID:          sessionID,
		Seq:                seq,
		EventType:          string(input.EventType),
		ActorParticipantID: nullString(input.ActorParticipantID),
		CommandID:          nullString(input.CommandID),
		RunID:              nullString(input.RunID),
		PayloadJson:        string(defaultJSON(input.PayloadJSON)),
	})
	if err != nil {
		return ChatEvent{}, err
	}
	event := eventFromRow(row)
	if err := updateProjectionTx(ctx, q, event); err != nil {
		return ChatEvent{}, err
	}
	return event, nil
}

func updateProjectionTx(ctx context.Context, q *db.Queries, event ChatEvent) error {
	row, err := q.GetChatSessionProjection(ctx, event.SessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	proj, err := projectionFromRow(row)
	if err != nil {
		return err
	}
	proj, err = ApplyEvent(proj, event)
	if err != nil {
		return err
	}
	_, err = q.UpsertChatSessionProjection(ctx, projectionParams(proj))
	if err != nil {
		return err
	}
	return q.UpdateChatSessionProjectionSeq(ctx, db.UpdateChatSessionProjectionSeqParams{
		ID:                   event.SessionID,
		CurrentProjectionSeq: proj.LastSeq,
	})
}

func eventFromRow(row db.ChatSessionEvent) ChatEvent {
	return ChatEvent{
		SessionID:          row.SessionID,
		Seq:                row.Seq,
		EventType:          EventType(row.EventType),
		ActorParticipantID: row.ActorParticipantID.String,
		CommandID:          row.CommandID.String,
		RunID:              row.RunID.String,
		PayloadJSON:        []byte(row.PayloadJson),
	}
}
