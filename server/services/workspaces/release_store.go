package workspaces

import (
	"context"
	"database/sql"
	"errors"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/pkg/release"
)

type CreateReleaseQueueItemParams struct {
	ID                   string
	DefinitionID         release.DefinitionID
	DefinitionVersion    string
	WorkflowID           runtime.WorkflowID
	WorkflowVersion      string
	FlowID               release.FlowID
	SourceSlug           string
	TargetLane           string
	ExpectedSourceCommit string
	ExpectedTargetCommit string
	CurrentNodeID        runtime.NodeID
	ActorEmail           string
	PayloadJSON          string
}

type AppendReleaseQueueEventParams struct {
	ItemID      string
	Level       string
	NodeID      runtime.NodeID
	Message     string
	PayloadJSON string
}

type ReleaseQueueStore interface {
	CreateReleaseQueueItem(ctx context.Context, arg CreateReleaseQueueItemParams) (ReleaseQueueItem, error)
	GetReleaseQueueItem(ctx context.Context, id string) (ReleaseQueueItem, error)
	ListActiveReleaseQueueItems(ctx context.Context) ([]ReleaseQueueItem, error)
	ListRecentReleaseQueueItems(ctx context.Context, limit int) ([]ReleaseQueueItem, error)
	ClaimNextPendingReleaseQueueItem(ctx context.Context) (ReleaseQueueItem, bool, error)
	MarkReleaseQueueItemRunning(ctx context.Context, id string, node runtime.NodeID) error
	MarkReleaseQueueItemTerminal(ctx context.Context, id string, status ReleaseQueueStatus, errMsg string) error
	AppendReleaseQueueEvent(ctx context.Context, arg AppendReleaseQueueEventParams) error
	ListReleaseQueueEvents(ctx context.Context, itemID string, limit int) ([]ReleaseQueueEvent, error)
}

type SQLReleaseQueueStore struct{ queries *db.Queries }

func NewSQLReleaseQueueStore(queries *db.Queries) *SQLReleaseQueueStore {
	return &SQLReleaseQueueStore{queries: queries}
}

func (s *SQLReleaseQueueStore) CreateReleaseQueueItem(ctx context.Context, arg CreateReleaseQueueItemParams) (ReleaseQueueItem, error) {
	payload := arg.PayloadJSON
	if payload == "" {
		payload = "{}"
	}
	row, err := s.queries.CreateReleaseQueueItem(ctx, db.CreateReleaseQueueItemParams{
		ID: arg.ID, DefinitionID: string(arg.DefinitionID), DefinitionVersion: arg.DefinitionVersion,
		WorkflowID: string(arg.WorkflowID), WorkflowVersion: arg.WorkflowVersion, FlowID: string(arg.FlowID),
		SourceSlug: arg.SourceSlug, TargetLane: arg.TargetLane, ExpectedSourceCommit: arg.ExpectedSourceCommit,
		ExpectedTargetCommit: arg.ExpectedTargetCommit, Status: string(ReleaseQueueStatusPending), CurrentNodeID: string(arg.CurrentNodeID),
		ActorEmail: arg.ActorEmail, PayloadJson: payload,
	})
	if err != nil {
		return ReleaseQueueItem{}, err
	}
	return mapReleaseQueueItem(row), nil
}

func (s *SQLReleaseQueueStore) GetReleaseQueueItem(ctx context.Context, id string) (ReleaseQueueItem, error) {
	row, err := s.queries.GetReleaseQueueItem(ctx, id)
	if err != nil {
		return ReleaseQueueItem{}, err
	}
	return mapReleaseQueueItem(row), nil
}

func (s *SQLReleaseQueueStore) ListActiveReleaseQueueItems(ctx context.Context) ([]ReleaseQueueItem, error) {
	rows, err := s.queries.ListActiveReleaseQueueItems(ctx)
	if err != nil {
		return nil, err
	}
	return mapReleaseQueueItems(rows), nil
}

func (s *SQLReleaseQueueStore) ListRecentReleaseQueueItems(ctx context.Context, limit int) ([]ReleaseQueueItem, error) {
	rows, err := s.queries.ListRecentReleaseQueueItems(ctx, int64(limit))
	if err != nil {
		return nil, err
	}
	return mapReleaseQueueItems(rows), nil
}

func (s *SQLReleaseQueueStore) ClaimNextPendingReleaseQueueItem(ctx context.Context) (ReleaseQueueItem, bool, error) {
	row, err := s.queries.ClaimNextPendingReleaseQueueItem(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return ReleaseQueueItem{}, false, nil
	}
	if err != nil {
		return ReleaseQueueItem{}, false, err
	}
	return mapReleaseQueueItem(row), true, nil
}

func (s *SQLReleaseQueueStore) MarkReleaseQueueItemRunning(ctx context.Context, id string, node runtime.NodeID) error {
	_, err := s.queries.MarkReleaseQueueItemRunning(ctx, db.MarkReleaseQueueItemRunningParams{ID: id, CurrentNodeID: string(node)})
	return err
}

func (s *SQLReleaseQueueStore) MarkReleaseQueueItemTerminal(ctx context.Context, id string, status ReleaseQueueStatus, errMsg string) error {
	_, err := s.queries.MarkReleaseQueueItemTerminal(ctx, db.MarkReleaseQueueItemTerminalParams{ID: id, Status: string(status), ErrorMessage: errMsg})
	return err
}

func (s *SQLReleaseQueueStore) AppendReleaseQueueEvent(ctx context.Context, arg AppendReleaseQueueEventParams) error {
	payload := arg.PayloadJSON
	if payload == "" {
		payload = "{}"
	}
	_, err := s.queries.AppendReleaseQueueEvent(ctx, db.AppendReleaseQueueEventParams{ItemID: arg.ItemID, Level: arg.Level, NodeID: string(arg.NodeID), Message: arg.Message, PayloadJson: payload})
	return err
}

func (s *SQLReleaseQueueStore) ListReleaseQueueEvents(ctx context.Context, itemID string, limit int) ([]ReleaseQueueEvent, error) {
	rows, err := s.queries.ListReleaseQueueEvents(ctx, db.ListReleaseQueueEventsParams{ItemID: itemID, Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	events := make([]ReleaseQueueEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, mapReleaseQueueEvent(row))
	}
	return events, nil
}

func mapReleaseQueueItems(rows []db.ReleaseQueueItem) []ReleaseQueueItem {
	items := make([]ReleaseQueueItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapReleaseQueueItem(row))
	}
	return items
}

func mapReleaseQueueItem(row db.ReleaseQueueItem) ReleaseQueueItem {
	item := ReleaseQueueItem{ID: row.ID, DefinitionID: release.DefinitionID(row.DefinitionID), DefinitionVersion: row.DefinitionVersion, WorkflowID: runtime.WorkflowID(row.WorkflowID), WorkflowVersion: row.WorkflowVersion, FlowID: release.FlowID(row.FlowID), SourceSlug: row.SourceSlug, TargetLane: row.TargetLane, ExpectedSourceCommit: row.ExpectedSourceCommit, ExpectedTargetCommit: row.ExpectedTargetCommit, Status: ReleaseQueueStatus(row.Status), CurrentNodeID: runtime.NodeID(row.CurrentNodeID), ActorEmail: row.ActorEmail, ErrorMessage: row.ErrorMessage, PayloadJSON: row.PayloadJson, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.StartedAt.Valid {
		t := row.StartedAt.Time
		item.StartedAt = &t
	}
	if row.FinishedAt.Valid {
		t := row.FinishedAt.Time
		item.FinishedAt = &t
	}
	return item
}

func mapReleaseQueueEvent(row db.ReleaseQueueEvent) ReleaseQueueEvent {
	return ReleaseQueueEvent{ID: row.ID, ItemID: row.ItemID, Level: row.Level, NodeID: runtime.NodeID(row.NodeID), Message: row.Message, PayloadJSON: row.PayloadJson, CreatedAt: row.CreatedAt}
}
