package chatsession

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"github.com/CoreyCole/vamos/pkg/db"
)

type ForkSessionInput struct {
	WorkspaceID     string
	ParentID        string
	ForkedAtSeq     int64
	ForkedNodeID    string
	ActorEmail      string
	Prompt          string
	WorkflowID      string
	WorkflowNodeID  string
	WorkflowAttempt int
}

func (s *Service) ForkSession(
	ctx context.Context,
	input ForkSessionInput,
) (db.ChatSession, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return db.ChatSession{}, err
	}
	defer tx.Rollback()
	child, err := ForkSessionWithQueries(ctx, s.q.WithTx(tx), input)
	if err != nil {
		return db.ChatSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return db.ChatSession{}, err
	}
	return child, nil
}

func ForkSessionWithQueries(
	ctx context.Context,
	q *db.Queries,
	input ForkSessionInput,
) (db.ChatSession, error) {
	parent, err := q.GetChatSession(ctx, strings.TrimSpace(input.ParentID))
	if err != nil {
		return db.ChatSession{}, err
	}
	forkedAtSeq := input.ForkedAtSeq
	if forkedAtSeq <= 0 {
		forkedAtSeq = parent.CurrentProjectionSeq
	}
	if forkedAtSeq <= 0 {
		events, err := q.ListChatSessionEventsAfter(
			ctx,
			db.ListChatSessionEventsAfterParams{
				SessionID: parent.ID,
				AfterSeq:  0,
				Limit:     100000,
			},
		)
		if err != nil {
			return db.ChatSession{}, err
		}
		for _, event := range events {
			if event.Seq > forkedAtSeq {
				forkedAtSeq = event.Seq
			}
		}
	}
	parentProj, err := snapshotWithQueriesThrough(ctx, q, parent.ID, forkedAtSeq)
	if err != nil {
		return db.ChatSession{}, err
	}
	child, err := CreateSessionWithQueries(ctx, q, CreateSessionInput{
		WorkspaceID:     parent.WorkspaceID,
		ActorEmail:      firstNonEmpty(input.ActorEmail, parent.CreatedByUserEmail),
		ParentSessionID: parent.ID,
		ForkedFromSeq:   forkedAtSeq,
		BranchID:        uuid.NewString(),
		WorkflowID:      firstNonEmpty(input.WorkflowID, parent.WorkflowID.String),
		WorkflowNodeID: firstNonEmpty(
			input.WorkflowNodeID,
			parent.WorkflowNodeID.String,
		),
		WorkflowAttempt: firstNonZero(input.WorkflowAttempt, int(parent.WorkflowAttempt)),
		TopologyKind:    TopologyFork,
	})
	if err != nil {
		return db.ChatSession{}, err
	}
	selectedState := marshalPayload(map[string]any{
		"forked_node_id": strings.TrimSpace(input.ForkedNodeID),
		"prompt":         strings.TrimSpace(input.Prompt),
	})
	if _, err := q.CreateChatSessionBaseline(ctx, db.CreateChatSessionBaselineParams{
		SessionID:                 child.ID,
		ParentSessionID:           parent.ID,
		ForkedFromSeq:             forkedAtSeq,
		BaselineProjectionVersion: 1,
		MessagesJson:              string(mustJSON(parentProj.Messages)),
		RunsJson:                  string(mustJSON(parentProj.Runs)),
		ArtifactsJson:             string(mustJSON(parentProj.Artifacts)),
		ParticipantsJson:          string(mustJSON(parentProj.Participants)),
		TopologyJson:              string(mustJSON(parentProj.Tree)),
		SelectedStateJson:         string(selectedState),
	}); err != nil {
		return db.ChatSession{}, err
	}
	if _, err := appendEventTx(ctx, q, AppendEventInput{
		SessionID:   child.ID,
		EventType:   EventForkCreated,
		PayloadJSON: forkPayload(input, parent.ID, child.ID, forkedAtSeq),
	}); err != nil {
		return db.ChatSession{}, err
	}
	if _, err := appendEventTx(ctx, q, AppendEventInput{
		SessionID:   child.ID,
		EventType:   EventBaselineCopied,
		PayloadJSON: baselineCopiedPayload(parentProj, parent.ID, forkedAtSeq),
	}); err != nil {
		return db.ChatSession{}, err
	}
	return child, nil
}

func snapshotWithQueriesThrough(
	ctx context.Context,
	q *db.Queries,
	sessionID string,
	throughSeq int64,
) (ChatProjection, error) {
	events, err := q.ListChatSessionEventsThrough(
		ctx,
		db.ListChatSessionEventsThroughParams{
			SessionID:  sessionID,
			ThroughSeq: throughSeq,
		},
	)
	if err != nil {
		return ChatProjection{}, err
	}
	baseline, err := q.GetChatSessionBaseline(ctx, sessionID)
	var baselinePtr *ChatBaseline
	if err == nil {
		baselinePtr = &ChatBaseline{
			SessionID:         baseline.SessionID,
			ParentSessionID:   baseline.ParentSessionID,
			ForkedFromSeq:     baseline.ForkedFromSeq,
			MessagesJSON:      json.RawMessage(baseline.MessagesJson),
			RunsJSON:          json.RawMessage(baseline.RunsJson),
			ArtifactsJSON:     json.RawMessage(baseline.ArtifactsJson),
			ParticipantsJSON:  json.RawMessage(baseline.ParticipantsJson),
			TopologyJSON:      json.RawMessage(baseline.TopologyJson),
			SelectedStateJSON: json.RawMessage(baseline.SelectedStateJson),
		}
	}
	return RebuildProjection(eventsFromRows(events), baselinePtr)
}

func forkPayload(
	input ForkSessionInput,
	parentID, childID string,
	forkedAtSeq int64,
) json.RawMessage {
	return marshalPayload(map[string]any{
		"parent_session_id": parentID,
		"child_session_id":  childID,
		"forked_from_seq":   forkedAtSeq,
		"forked_node_id":    strings.TrimSpace(input.ForkedNodeID),
		"summary":           "Fork created",
	})
}

func baselineCopiedPayload(
	proj ChatProjection,
	parentID string,
	forkedAtSeq int64,
) json.RawMessage {
	return marshalPayload(map[string]any{
		"parent_session_id": parentID,
		"forked_from_seq":   forkedAtSeq,
		"messages":          len(proj.Messages),
		"runs":              len(proj.Runs),
		"artifacts":         len(proj.Artifacts),
		"participants":      len(proj.Participants),
		"summary":           "Baseline copied",
	})
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
