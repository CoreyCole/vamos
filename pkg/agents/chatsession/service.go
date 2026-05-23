package chatsession

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"github.com/CoreyCole/vamos/pkg/db"
)

type Service struct {
	db *sql.DB
	q  *db.Queries
}

func NewService(database *sql.DB, queries *db.Queries) *Service {
	if queries == nil && database != nil {
		queries = db.New(database)
	}
	return &Service{db: database, q: queries}
}

func (s *Service) CreateSession(
	ctx context.Context,
	input CreateSessionInput,
) (db.ChatSession, error) {
	return CreateSessionWithQueries(ctx, s.q, input)
}

func CreateSessionWithQueries(
	ctx context.Context,
	q *db.Queries,
	input CreateSessionInput,
) (db.ChatSession, error) {
	topology := input.TopologyKind
	if topology == "" {
		topology = TopologyRoot
	}
	branchID := strings.TrimSpace(input.BranchID)
	if branchID == "" {
		branchID = uuid.NewString()
	}
	attempt := input.WorkflowAttempt
	if attempt < 0 {
		attempt = 0
	}
	return q.CreateChatSession(ctx, db.CreateChatSessionParams{
		ID:                 uuid.NewString(),
		WorkspaceID:        strings.TrimSpace(input.WorkspaceID),
		CreatedByUserEmail: strings.TrimSpace(input.ActorEmail),
		ParentSessionID:    nullString(input.ParentSessionID),
		ForkedFromSeq:      nullInt64(input.ForkedFromSeq, input.ForkedFromSeq > 0),
		BranchID:           branchID,
		WorkflowID:         nullString(input.WorkflowID),
		WorkflowNodeID:     nullString(input.WorkflowNodeID),
		WorkflowAttempt:    int64(attempt),
		TopologyKind:       string(topology),
	})
}

func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func nullInt64(value int64, valid bool) sql.NullInt64 {
	return sql.NullInt64{Int64: value, Valid: valid}
}

func defaultJSON(raw json.RawMessage) json.RawMessage {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func marshalPayload(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
