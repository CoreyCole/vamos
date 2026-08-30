package agentchat

import (
	"context"
	"database/sql"
	"strings"
	"sync/atomic"
	"time"

	"github.com/CoreyCole/vamos/pkg/db"
)

var draftOperationOrder atomic.Int64

func nextDraftOperationOrder() int64 {
	for {
		next := time.Now().UnixNano()
		current := draftOperationOrder.Load()
		if next <= current {
			next = current + 1
		}
		if draftOperationOrder.CompareAndSwap(current, next) {
			return next
		}
	}
}

func (s *Service) GetThreadDraft(
	ctx context.Context,
	userEmail, threadID string,
) (string, error) {
	if strings.TrimSpace(userEmail) == "" {
		return "", nil
	}
	draft, err := s.queries.GetAgentThreadDraft(ctx, db.GetAgentThreadDraftParams{
		UserEmail: strings.TrimSpace(userEmail),
		ThreadID:  strings.TrimSpace(threadID),
	})
	if err == sql.ErrNoRows {
		return "", nil
	}
	return draft, err
}

func (s *Service) SaveThreadDraft(
	ctx context.Context,
	userEmail, threadID, content string,
	operationOrder int64,
) error {
	if _, err := s.queries.GetSharedAgentThread(
		ctx,
		strings.TrimSpace(threadID),
	); err != nil {
		return err
	}
	return s.queries.UpsertAgentThreadDraft(ctx, db.UpsertAgentThreadDraftParams{
		UserEmail:      strings.TrimSpace(userEmail),
		ThreadID:       strings.TrimSpace(threadID),
		Content:        content,
		OperationOrder: operationOrder,
	})
}

func (s *Service) ClearThreadDraft(
	ctx context.Context,
	userEmail, threadID string,
) error {
	return s.queries.ClearAgentThreadDraft(ctx, db.ClearAgentThreadDraftParams{
		UserEmail:      strings.TrimSpace(userEmail),
		ThreadID:       strings.TrimSpace(threadID),
		OperationOrder: nextDraftOperationOrder(),
	})
}
