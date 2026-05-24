package workspaces

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/db"
)

type WorkspaceErrorSource string

const (
	WorkspaceErrorSourceSwitch  WorkspaceErrorSource = "switch"
	WorkspaceErrorSourceManager WorkspaceErrorSource = "manager"
	WorkspaceErrorSourceLog     WorkspaceErrorSource = "log"
)

type WorkspaceErrorSeverity string

const (
	WorkspaceErrorSeverityWarn  WorkspaceErrorSeverity = "warn"
	WorkspaceErrorSeverityError WorkspaceErrorSeverity = "error"
)

type WorkspaceErrorEvent = db.WorkspaceErrorEvent

const workspaceErrorEventLimit = 100

type UpsertWorkspaceErrorEventParams struct {
	WorkspaceSlug string
	Source        WorkspaceErrorSource
	Severity      WorkspaceErrorSeverity
	Message       string
	Detail        string
	DedupeKey     string
	PayloadJSON   string
}

type ListRecentWorkspaceErrorEventsForWorkspaceParams struct {
	WorkspaceSlug string
	Limit         int64
}

type WorkspaceErrorEventStore interface {
	UpsertWorkspaceErrorEvent(ctx context.Context, arg UpsertWorkspaceErrorEventParams) (WorkspaceErrorEvent, error)
	ListRecentWorkspaceErrorEvents(ctx context.Context, limit int64) ([]WorkspaceErrorEvent, error)
	ListRecentWorkspaceErrorEventsForWorkspace(ctx context.Context, arg ListRecentWorkspaceErrorEventsForWorkspaceParams) ([]WorkspaceErrorEvent, error)
}

type SQLWorkspaceErrorEventStore struct{ queries *db.Queries }

func NewSQLWorkspaceErrorEventStore(queries *db.Queries) *SQLWorkspaceErrorEventStore {
	return &SQLWorkspaceErrorEventStore{queries: queries}
}

func (s *SQLWorkspaceErrorEventStore) UpsertWorkspaceErrorEvent(ctx context.Context, arg UpsertWorkspaceErrorEventParams) (WorkspaceErrorEvent, error) {
	payload := strings.TrimSpace(arg.PayloadJSON)
	if payload == "" {
		payload = "{}"
	}
	return s.queries.UpsertWorkspaceErrorEvent(ctx, db.UpsertWorkspaceErrorEventParams{
		WorkspaceSlug: arg.WorkspaceSlug,
		Source:        string(arg.Source),
		Severity:      string(arg.Severity),
		Message:       arg.Message,
		Detail:        arg.Detail,
		DedupeKey:     arg.DedupeKey,
		PayloadJson:   payload,
	})
}

func (s *SQLWorkspaceErrorEventStore) ListRecentWorkspaceErrorEvents(ctx context.Context, limit int64) ([]WorkspaceErrorEvent, error) {
	return s.queries.ListRecentWorkspaceErrorEvents(ctx, limit)
}

func (s *SQLWorkspaceErrorEventStore) ListRecentWorkspaceErrorEventsForWorkspace(ctx context.Context, arg ListRecentWorkspaceErrorEventsForWorkspaceParams) ([]WorkspaceErrorEvent, error) {
	return s.queries.ListRecentWorkspaceErrorEventsForWorkspace(ctx, db.ListRecentWorkspaceErrorEventsForWorkspaceParams{
		WorkspaceSlug: arg.WorkspaceSlug,
		Limit:         arg.Limit,
	})
}

type WorkspaceErrorPageModel struct {
	SelectedWorkspace string
	Events            []WorkspaceErrorEventView
	Workspaces        []ImplWorkspaceView
	ScanInFlight      bool
	ManagerURL        string
}

type WorkspaceErrorEventView struct {
	ID              int64
	WorkspaceSlug   string
	Source          string
	Severity        string
	Message         string
	Detail          string
	OccurrenceCount int64
	FirstSeenAt     time.Time
	LastSeenAt      time.Time
}

func mapWorkspaceErrorEventView(row WorkspaceErrorEvent) WorkspaceErrorEventView {
	return WorkspaceErrorEventView{
		ID:              row.ID,
		WorkspaceSlug:   row.WorkspaceSlug,
		Source:          row.Source,
		Severity:        row.Severity,
		Message:         row.Message,
		Detail:          row.Detail,
		OccurrenceCount: row.OccurrenceCount,
		FirstSeenAt:     row.FirstSeenAt,
		LastSeenAt:      row.LastSeenAt,
	}
}

func workspaceErrorsStreamPath(selected string) string {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return "/workspaces/errors/stream"
	}
	return "/workspaces/errors/stream?workspace=" + url.QueryEscape(selected)
}

func workspaceErrorTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.Format("Jan 2, 2006 · 3:04 PM")
}

func workspaceErrorWorkspaceView(views []ImplWorkspaceView, slug string) *ImplWorkspaceView {
	slug = strings.TrimSpace(slug)
	for _, view := range views {
		if workspaceViewSlug(view) == slug {
			viewCopy := view
			return &viewCopy
		}
		if child := workspaceErrorWorkspaceView(view.Children, slug); child != nil {
			return child
		}
	}
	return nil
}

func (h *Handler) isWorkspaceErrorScanInFlight(string) bool {
	return false
}

type WorkspaceErrorRecorder struct {
	Store    WorkspaceErrorEventStore
	Notifier WorkspaceLifecycleNotifier
}

type WorkspaceErrorRecordRequest struct {
	WorkspaceSlug string
	Source        WorkspaceErrorSource
	Severity      WorkspaceErrorSeverity
	Message       string
	Detail        string
	DedupeKey     string
	PayloadJSON   string
}

func (r *WorkspaceErrorRecorder) Record(ctx context.Context, req WorkspaceErrorRecordRequest) (WorkspaceErrorEvent, error) {
	if r == nil || r.Store == nil {
		return WorkspaceErrorEvent{}, nil
	}
	req.WorkspaceSlug = strings.TrimSpace(req.WorkspaceSlug)
	req.Message = strings.TrimSpace(req.Message)
	if req.WorkspaceSlug == "" || req.Source == "" || req.Message == "" {
		return WorkspaceErrorEvent{}, fmt.Errorf("workspace error event requires workspace slug, source, and message")
	}
	if req.Severity == "" {
		req.Severity = WorkspaceErrorSeverityError
	}
	req.Detail = strings.TrimSpace(req.Detail)
	req.DedupeKey = strings.TrimSpace(req.DedupeKey)
	if req.DedupeKey == "" {
		req.DedupeKey = workspaceErrorDedupeKey(req.WorkspaceSlug, string(req.Source), req.Message, req.Detail)
	}
	if req.DedupeKey == "" {
		return WorkspaceErrorEvent{}, fmt.Errorf("workspace error event dedupe key is empty")
	}
	event, err := r.Store.UpsertWorkspaceErrorEvent(ctx, UpsertWorkspaceErrorEventParams{
		WorkspaceSlug: req.WorkspaceSlug,
		Source:        req.Source,
		Severity:      req.Severity,
		Message:       req.Message,
		Detail:        req.Detail,
		DedupeKey:     req.DedupeKey,
		PayloadJSON:   req.PayloadJSON,
	})
	if err == nil && r.Notifier != nil {
		r.Notifier.Notify(req.WorkspaceSlug)
	}
	return event, err
}

func (r *WorkspaceErrorRecorder) RecordSwitchUnavailable(ctx context.Context, ws Workspace, redirectPath string) error {
	detail := fmt.Sprintf("status=%s url=%q redirect=%q error=%s", ws.Status, strings.TrimSpace(ws.URL), redirectPath, strings.TrimSpace(ws.Error))
	_, err := r.Record(ctx, WorkspaceErrorRecordRequest{
		WorkspaceSlug: ws.Slug,
		Source:        WorkspaceErrorSourceSwitch,
		Severity:      WorkspaceErrorSeverityWarn,
		Message:       "workspace unavailable during switch",
		Detail:        detail,
		DedupeKey:     workspaceErrorDedupeKey(ws.Slug, string(WorkspaceErrorSourceSwitch), string(ws.Status), strings.TrimSpace(ws.URL), redirectPath),
	})
	return err
}

func workspaceErrorDedupeKey(parts ...string) string {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ToLower(strings.Join(strings.Fields(part), " "))
		if part != "" {
			normalized = append(normalized, part)
		}
	}
	if len(normalized) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(normalized, "|")))
	return hex.EncodeToString(sum[:16])
}
