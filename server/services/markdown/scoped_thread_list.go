package markdown

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/agents/roster"
	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/agenthome"
)

// emptyScopeComposer is set by agentchat (init) so N=0 lands use AgentChatComposer
// without an import cycle.
var emptyScopeComposer func(action, attachedDoc, modeLabel string) templ.Component

func SetEmptyScopeComposer(fn func(action, attachedDoc, modeLabel string) templ.Component) {
	emptyScopeComposer = fn
}

func renderScopedThreadListOrComposer(
	rows []agenthome.ConversationRowArgs,
	kind, id, attachedDoc string,
) templ.Component {
	if len(rows) == 0 {
		if emptyScopeComposer != nil {
			return emptyScopeComposer(
				emptyScopeComposerAction(kind, id),
				attachedDoc,
				emptyScopeModeLabel(kind, id, attachedDoc),
			)
		}
		return templ.NopComponent
	}
	return ScopedThreadList(rows)
}

func (s *Service) requireKnownBot(slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return echo.NewHTTPError(http.StatusNotFound, "agent not found")
	}
	if s.roster == nil {
		return nil
	}
	_, err := s.roster.Get(slug)
	if errors.Is(err, roster.ErrNotFound) || errors.Is(err, roster.ErrArchived) {
		return echo.NewHTTPError(http.StatusNotFound, "agent not found")
	}
	return err
}

func (s *Service) botScopedConversationRows(
	ctx context.Context,
	slug string,
) ([]agenthome.ConversationRowArgs, error) {
	if s.queries == nil {
		return nil, nil
	}
	threads, err := s.queries.ListAgentThreadsByAgentSlug(ctx, sql.NullString{
		String: slug,
		Valid:  slug != "",
	})
	if err != nil {
		return nil, err
	}
	rows := make([]agenthome.ConversationRowArgs, 0, len(threads))
	for _, thread := range threads {
		if strings.EqualFold(thread.RoomKind, "pairwise") {
			continue
		}
		if thread.ParentThreadID.Valid &&
			strings.TrimSpace(thread.ParentThreadID.String) != "" {
			continue
		}
		if !jsonlIsScopedListRow(s.botThreadJSONLPath(ctx, slug, thread)) {
			continue
		}
		rows = append(rows, s.conversationRowForBotThread(ctx, slug, thread))
	}
	return rows, nil
}

func (s *Service) conversationRowForBotThread(
	ctx context.Context,
	slug string,
	thread db.AgentThread,
) agenthome.ConversationRowArgs {
	title := strings.TrimSpace(thread.Title)
	if title == "" {
		title = slug
	}
	preview := lastJSONLPreview(s.botThreadJSONLPath(ctx, slug, thread))
	return agenthome.ConversationRowArgs{
		ID:          "scoped-thread-row-" + thread.ID,
		Href:        "/threads/" + thread.ID,
		Title:       title,
		Preview:     preview.Text,
		Time:        rosterPlanTime(preview.Time),
		Initial:     scopedRowInitial(title),
		AccentClass: "bg-fuchsia-500/90",
		TestID:      "scoped-thread-row",
	}
}

func (s *Service) botThreadJSONLPath(
	ctx context.Context,
	slug string,
	thread db.AgentThread,
) string {
	current := filepath.Join(
		s.basePath, "agents", slug, "sessions", "current.jsonl",
	)
	path := resolveScopedJSONLPath(current, thread.PiSessionID)
	s.persistPiSessionFromPath(ctx, thread, path)
	return path
}

func (s *Service) planScopedConversationRows(
	ctx context.Context,
	roomID, artifact string,
) ([]agenthome.ConversationRowArgs, error) {
	if s.queries == nil {
		return nil, nil
	}
	rel, err := s.resolvePlanDirRelForRoom(ctx, roomID, artifact)
	if err != nil {
		return nil, err
	}
	if rel == "" {
		if root, ok := InferWorkspaceRoot(s.basePath, artifact); ok {
			rel = root
		} else {
			rel = strings.ReplaceAll(strings.TrimSpace(roomID), "--", "/")
		}
	}
	rel = strings.Trim(strings.TrimPrefix(filepath.ToSlash(rel), "thoughts/"), "/")
	seen := map[string]struct{}{}
	var threads []db.AgentThread
	for _, cand := range []string{rel, "thoughts/" + rel} {
		if cand == "" || cand == "thoughts/" {
			continue
		}
		got, err := s.queries.ListAgentThreadsByPlanDirRel(ctx, sql.NullString{
			String: cand,
			Valid:  true,
		})
		if err != nil {
			return nil, err
		}
		for _, thread := range got {
			if _, ok := seen[thread.ID]; ok {
				continue
			}
			seen[thread.ID] = struct{}{}
			threads = append(threads, thread)
		}
	}
	rows := make([]agenthome.ConversationRowArgs, 0, len(threads))
	for _, thread := range threads {
		if !strings.EqualFold(strings.TrimSpace(thread.RoomKind), "plan") {
			continue
		}
		if thread.ParentThreadID.Valid &&
			strings.TrimSpace(thread.ParentThreadID.String) != "" {
			continue
		}
		if !jsonlIsScopedListRow(s.planThreadJSONLPath(ctx, rel, thread)) {
			continue
		}
		rows = append(rows, s.conversationRowForPlanThread(ctx, rel, thread))
	}
	return rows, nil
}

func (s *Service) conversationRowForPlanThread(
	ctx context.Context,
	planDirRel string,
	thread db.AgentThread,
) agenthome.ConversationRowArgs {
	title := strings.TrimSpace(thread.Title)
	if title == "" {
		title = filepath.Base(filepath.ToSlash(planDirRel))
	}
	preview := lastJSONLPreview(s.planThreadJSONLPath(ctx, planDirRel, thread))
	return agenthome.ConversationRowArgs{
		ID:          "scoped-thread-row-" + thread.ID,
		Href:        "/threads/" + thread.ID,
		Title:       title,
		Preview:     preview.Text,
		Time:        rosterPlanTime(preview.Time),
		Initial:     scopedRowInitial(title),
		AccentClass: "bg-sky-500/80",
		TestID:      "scoped-thread-row",
	}
}

func (s *Service) planThreadJSONLPath(
	ctx context.Context,
	planDirRel string,
	thread db.AgentThread,
) string {
	rel := strings.TrimPrefix(filepath.ToSlash(planDirRel), "thoughts/")
	current := filepath.Join(
		s.basePath, filepath.FromSlash(rel), ".vamos", "sessions", "current.jsonl",
	)
	path := resolveScopedJSONLPath(current, thread.PiSessionID)
	s.persistPiSessionFromPath(ctx, thread, path)
	return path
}

func (s *Service) freeformScopedConversationRows(
	ctx context.Context,
) ([]agenthome.ConversationRowArgs, error) {
	if s.queries == nil {
		return nil, nil
	}
	threads, err := s.queries.ListAgentThreadsFreeform(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]agenthome.ConversationRowArgs, 0, len(threads))
	for _, thread := range threads {
		if strings.EqualFold(thread.RoomKind, "pairwise") {
			continue
		}
		if thread.ParentThreadID.Valid &&
			strings.TrimSpace(thread.ParentThreadID.String) != "" {
			continue
		}
		if !jsonlIsScopedListRow(s.freeformThreadJSONLPath(ctx, thread)) {
			continue
		}
		rows = append(rows, s.conversationRowForFreeformThread(ctx, thread))
	}
	return rows, nil
}

func (s *Service) conversationRowForFreeformThread(
	ctx context.Context,
	thread db.AgentThread,
) agenthome.ConversationRowArgs {
	title := strings.TrimSpace(thread.Title)
	if title == "" {
		title = "Untitled"
	}
	preview := lastJSONLPreview(s.freeformThreadJSONLPath(ctx, thread))
	return agenthome.ConversationRowArgs{
		ID:          "scoped-thread-row-" + thread.ID,
		Href:        "/threads/" + thread.ID,
		Title:       title,
		Preview:     preview.Text,
		Time:        rosterPlanTime(preview.Time),
		Initial:     scopedRowInitial(title),
		AccentClass: "bg-emerald-500/90",
		TestID:      "scoped-thread-row",
	}
}

func (s *Service) freeformThreadJSONLPath(
	ctx context.Context,
	thread db.AgentThread,
) string {
	cwd := strings.TrimSpace(thread.Cwd)
	current := filepath.Join(cwd, ".vamos", "sessions", "current.jsonl")
	path := resolveScopedJSONLPath(current, thread.PiSessionID)
	s.persistPiSessionFromPath(ctx, thread, path)
	return path
}

func (s *Service) persistPiSessionFromPath(
	ctx context.Context,
	thread db.AgentThread,
	path string,
) {
	if s == nil || s.queries == nil {
		return
	}
	if strings.TrimSpace(thread.PiSessionID) != "" {
		return
	}
	if !strings.Contains(filepath.ToSlash(path), "/pi/") {
		return
	}
	piID := piSessionIDFromJSONLPath(path)
	if piID == "" || piID == "current" {
		return
	}
	_ = s.queries.SetAgentThreadPiSessionID(ctx, db.SetAgentThreadPiSessionIDParams{
		PiSessionID: piID,
		ID:          thread.ID,
	})
}

func scopedRowInitial(title string) string {
	for _, r := range strings.TrimSpace(title) {
		return strings.ToUpper(string(r))
	}
	return "?"
}
