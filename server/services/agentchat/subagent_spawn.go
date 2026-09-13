package agentchat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/CoreyCole/vamos/pkg/db"
)

const (
	subagentCardDOMIDPrefix = "subagent-card-"
	subagentCardVariant     = "subagent-card"
)

// SpawnSubagentInput creates a child thread under parent via the fork path
// (ParentThreadID + inherited email/cwd/plan_dir_rel/room_kind). Optional
// ArtifactPath attaches a disk session; Prompt steers the child via ThreadMail.
type SpawnSubagentInput struct {
	UserEmail      string
	ParentThreadID string
	Title          string
	Prompt         string
	ArtifactPath   string
	SourceEntryID  string
}

// AttachSubagentInput upserts agent_sessions.artifact_path + projected_thread_id
// onto an existing child (disk SoT under thoughts/agents/{slug}/sessions).
type AttachSubagentInput struct {
	UserEmail      string
	ParentThreadID string
	ChildThreadID  string
	ArtifactPath   string
}

func subagentCardDOMID(childThreadID string) string {
	return subagentCardDOMIDPrefix + strings.TrimSpace(childThreadID)
}

// SpawnSubagent forks a child thread under parent. Does not write room_kind=subagent.
func (s *Service) SpawnSubagent(
	ctx context.Context,
	in SpawnSubagentInput,
) (db.AgentThread, error) {
	userEmail := strings.TrimSpace(in.UserEmail)
	parentID := strings.TrimSpace(in.ParentThreadID)
	if userEmail == "" || parentID == "" {
		return db.AgentThread{}, fmt.Errorf("user_email and parent_thread_id are required")
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = strings.TrimSpace(in.Prompt)
	}
	if title == "" {
		title = "Subagent"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return db.AgentThread{}, err
	}
	defer func() { _ = tx.Rollback() }()
	q := s.queries.WithTx(tx)

	parent, err := q.GetAgentThreadForUser(ctx, db.GetAgentThreadForUserParams{
		ID:        parentID,
		UserEmail: userEmail,
	})
	if err != nil {
		return db.AgentThread{}, err
	}

	var child db.AgentThread
	if entryID := strings.TrimSpace(in.SourceEntryID); entryID != "" {
		entry, entryErr := q.GetAgentEntry(ctx, db.GetAgentEntryParams{
			LineageID: parent.LineageID,
			EntryID:   entryID,
		})
		if entryErr != nil {
			return db.AgentThread{}, entryErr
		}
		child, _, err = s.createForkThreadRecord(ctx, q, parent, entry, title)
		if err != nil {
			return db.AgentThread{}, err
		}
	} else {
		child, err = s.createSpawnChildThreadRecord(ctx, q, parent, title)
		if err != nil {
			return db.AgentThread{}, err
		}
	}

	if err := s.inheritParentRoomKind(ctx, q, parent, child); err != nil {
		return db.AgentThread{}, err
	}
	child, err = q.GetAgentThread(ctx, child.ID)
	if err != nil {
		return db.AgentThread{}, err
	}

	if path := strings.TrimSpace(in.ArtifactPath); path != "" {
		if _, err := s.upsertSubagentSession(ctx, q, userEmail, child, path); err != nil {
			return db.AgentThread{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return db.AgentThread{}, err
	}

	if prompt := strings.TrimSpace(in.Prompt); prompt != "" {
		if _, err := s.EnqueueThreadMail(ctx, EnqueueThreadMailInput{
			ThreadID:      child.ID,
			FromKind:      "user",
			FromUserEmail: userEmail,
			Body:          prompt,
		}); err != nil {
			return child, err
		}
	}

	s.notifyThreadScope(ctx, parent.ID, PatchLiveTranscript)
	return child, nil
}

// AttachSubagentSession upserts artifact_path + projected_thread_id for a child
// of parent. Profile pane stays a file browser; this only binds the session row.
func (s *Service) AttachSubagentSession(
	ctx context.Context,
	in AttachSubagentInput,
) (db.AgentSession, error) {
	userEmail := strings.TrimSpace(in.UserEmail)
	parentID := strings.TrimSpace(in.ParentThreadID)
	childID := strings.TrimSpace(in.ChildThreadID)
	artifactPath := strings.TrimSpace(in.ArtifactPath)
	if userEmail == "" || parentID == "" || childID == "" || artifactPath == "" {
		return db.AgentSession{}, fmt.Errorf(
			"user_email, parent_thread_id, child_thread_id, and artifact_path are required",
		)
	}

	parent, err := s.queries.GetAgentThreadForUser(ctx, db.GetAgentThreadForUserParams{
		ID:        parentID,
		UserEmail: userEmail,
	})
	if err != nil {
		return db.AgentSession{}, err
	}
	child, err := s.queries.GetAgentThread(ctx, childID)
	if err != nil {
		return db.AgentSession{}, err
	}
	if !child.ParentThreadID.Valid ||
		strings.TrimSpace(child.ParentThreadID.String) != parent.ID {
		return db.AgentSession{}, fmt.Errorf("child thread is not under parent")
	}

	session, err := s.upsertSubagentSession(ctx, s.queries, userEmail, child, artifactPath)
	if err != nil {
		return db.AgentSession{}, err
	}
	s.notifyThreadScope(ctx, parent.ID, PatchLiveTranscript)
	return session, nil
}

func (s *Service) createSpawnChildThreadRecord(
	ctx context.Context,
	q *db.Queries,
	parent db.AgentThread,
	title string,
) (db.AgentThread, error) {
	params := db.CreateAgentThreadParams{
		ID:             uuid.NewString(),
		UserEmail:      parent.UserEmail,
		Title:          truncateTitle(title),
		Cwd:            parent.Cwd,
		LineageID:      parent.LineageID,
		ProjectID:      parent.ProjectID,
		PlanDirRel:     parent.PlanDirRel,
		HeadEntryID:    sql.NullString{},
		ParentThreadID: sql.NullString{String: parent.ID, Valid: true},
	}
	return q.CreateAgentThread(ctx, s.attachPlanDirRel(ctx, params))
}

// inheritParentRoomKind copies shared room_kind so /threads/{child} opens via
// GetAgentThreadForUser. Never writes room_kind=subagent (AI-471). bot_home /
// pairwise unique keys are not copied — only room_kind.
func (s *Service) inheritParentRoomKind(
	ctx context.Context,
	q *db.Queries,
	parent, child db.AgentThread,
) error {
	kind := strings.TrimSpace(parent.RoomKind)
	switch kind {
	case "":
		return nil
	case RoomKindPlan:
		return q.BindAgentThreadPlan(ctx, db.BindAgentThreadPlanParams{
			AgentID: parent.AgentID,
			Cwd:     child.Cwd,
			Title:   child.Title,
			ID:      child.ID,
		})
	case RoomKindBotHome, RoomKindPairwise:
		return q.SetAgentThreadRoomKind(ctx, db.SetAgentThreadRoomKindParams{
			RoomKind: kind,
			ID:       child.ID,
		})
	default:
		return fmt.Errorf("cannot inherit room_kind %q", kind)
	}
}

func (s *Service) upsertSubagentSession(
	ctx context.Context,
	q *db.Queries,
	userEmail string,
	child db.AgentThread,
	artifactPath string,
) (db.AgentSession, error) {
	artifactPath = strings.TrimSpace(artifactPath)
	if artifactPath == "" {
		return db.AgentSession{}, fmt.Errorf("artifact_path is required")
	}
	pathNull := nullString(artifactPath)
	existing, err := q.GetAgentSessionByPath(ctx, pathNull)
	if err == nil {
		if err := q.UpdateAgentSessionProjectedThread(
			ctx,
			db.UpdateAgentSessionProjectedThreadParams{
				ProjectedThreadID: nullString(child.ID),
				ArtifactPath:      pathNull,
				ID:                existing.ID,
			},
		); err != nil {
			return db.AgentSession{}, err
		}
		return q.GetAgentSession(ctx, existing.ID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return db.AgentSession{}, err
	}
	return q.CreateAgentSession(ctx, db.CreateAgentSessionParams{
		ID:                  uuid.NewString(),
		IdentityKind:        "global_pi",
		ArtifactPath:        pathNull,
		PlanDir:             child.PlanDirRel,
		ExternalSessionID:   sql.NullString{},
		ParentSessionID:     sql.NullString{},
		Cwd:                 nullString(child.Cwd),
		ProjectionState:     "needs_hydration",
		ProjectedThreadID:   nullString(child.ID),
		IndexedByUserEmail:  normalizeSessionOwnerEmail(userEmail),
		AttachedWorkspaceID: sql.NullString{},
		ImportedHeadEntryID: sql.NullString{},
		LastError:           sql.NullString{},
		MetadataJson:        sql.NullString{},
	})
}

func (s *Service) deriveSubagentLiveCards(
	ctx context.Context,
	parent db.AgentThread,
) []TranscriptMessage {
	if s == nil || s.queries == nil || strings.TrimSpace(parent.ID) == "" {
		return nil
	}
	children, err := s.queries.ListAgentThreadsByParentThreadID(
		ctx,
		sql.NullString{String: parent.ID, Valid: true},
	)
	if err != nil || len(children) == 0 {
		return nil
	}
	cards := make([]TranscriptMessage, 0, len(children))
	for _, child := range children {
		status := s.subagentCardStatus(ctx, child)
		domID := subagentCardDOMID(child.ID)
		cards = append(cards, TranscriptMessage{
			DOMID:         domID,
			EntryID:       domID,
			Variant:       subagentCardVariant,
			Role:          "assistant",
			Title:         firstNonEmpty(strings.TrimSpace(child.Title), "Subagent"),
			HeaderCode:    "Open",
			HeaderHref:    "/threads/" + child.ID,
			HeaderSummary: status,
			Content:       status,
		})
	}
	return cards
}

func (s *Service) subagentCardStatus(ctx context.Context, child db.AgentThread) string {
	sessions, err := s.queries.ListAgentSessionsByProjectedThreadID(
		ctx,
		nullString(child.ID),
	)
	if err == nil && len(sessions) > 0 {
		state := strings.TrimSpace(sessions[0].ProjectionState)
		if state != "" {
			return state
		}
	}
	if s.queries != nil {
		run, err := s.queries.GetLatestAgentRunByThread(ctx, child.ID)
		if err == nil {
			status := strings.TrimSpace(run.Status)
			if status != "" {
				return status
			}
		}
	}
	return "ready"
}

// mergeSubagentCardsIntoLive appends derived cards after live SoT items so they
// ride BuildLiveTranscriptState (2719aad keep-or-promote): empty live reducer
// still shows cards; never a new transcript entry type / BotDMChip.
func mergeSubagentCardsIntoLive(live LiveTranscriptView, cards []TranscriptMessage) LiveTranscriptView {
	if len(cards) == 0 {
		return live
	}
	out := make([]TranscriptMessage, 0, len(live.Items)+len(cards))
	out = append(out, live.Items...)
	out = append(out, cards...)
	live.Items = out
	return live
}

// SteerSubagent enqueues ThreadMail → ThreadInbox SignalWithStart on the child.
func (s *Service) SteerSubagent(
	ctx context.Context,
	userEmail, childThreadID, body string,
) (EnqueueThreadMailResult, error) {
	userEmail = strings.TrimSpace(userEmail)
	childThreadID = strings.TrimSpace(childThreadID)
	body = strings.TrimSpace(body)
	if userEmail == "" || childThreadID == "" || body == "" {
		return EnqueueThreadMailResult{}, fmt.Errorf("user_email, child_thread_id, and body are required")
	}
	if _, err := s.queries.GetAgentThreadForUser(ctx, db.GetAgentThreadForUserParams{
		ID:        childThreadID,
		UserEmail: userEmail,
	}); err != nil {
		return EnqueueThreadMailResult{}, err
	}
	return s.EnqueueThreadMail(ctx, EnqueueThreadMailInput{
		ThreadID:      childThreadID,
		FromKind:      "user",
		FromUserEmail: userEmail,
		Body:          body,
	})
}
