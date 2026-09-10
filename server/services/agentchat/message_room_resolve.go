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

var (
	ErrMessageRoomUnknownSlug  = errors.New("unknown agent slug")
	ErrMessageRoomArchivedSlug = errors.New("archived agent slug")
	ErrMessageRoomSelf         = errors.New("cannot message_room to self")
	ErrMessageRoomNonPlanPath  = errors.New("message_room to is not a plan directory")
	ErrMessageRoomThreadUUID   = errors.New("message_room to cannot be a thread UUID")
	ErrMessageRoomBotHome      = errors.New(
		"cannot enqueue agent-to-agent mail into a bot home",
	)
)

type MessageRoomResolveInput struct {
	To             string
	FromAgentID    string
	OriginThreadID string
}

type MessageRoomResolveResult struct {
	ThreadID       string
	SpeakerAgentID string
	RoomKind       string
}

func (s *Service) ResolveMessageRoom(
	ctx context.Context,
	in MessageRoomResolveInput,
) (MessageRoomResolveResult, error) {
	to := strings.TrimSpace(in.To)
	if to == "" {
		return MessageRoomResolveResult{}, fmt.Errorf("to is required")
	}
	fromID := strings.TrimSpace(in.FromAgentID)
	if fromID == "" {
		return MessageRoomResolveResult{}, fmt.Errorf("from_agent_id is required")
	}
	if looksLikeThreadUUID(to) {
		return MessageRoomResolveResult{}, ErrMessageRoomThreadUUID
	}
	if isBotHomeDestination(to) {
		return MessageRoomResolveResult{}, ErrMessageRoomBotHome
	}

	fromAgent, err := s.lookupFromAgent(ctx, fromID)
	if err != nil {
		return MessageRoomResolveResult{}, err
	}

	if strings.HasPrefix(to, "thoughts/") || strings.Contains(to, "/plans/") {
		return s.resolvePlanDestination(ctx, to)
	}

	if err := validateSlug(to); err != nil {
		return MessageRoomResolveResult{}, ErrMessageRoomNonPlanPath
	}
	if to == strings.TrimSpace(fromAgent.Slug) {
		return MessageRoomResolveResult{}, ErrMessageRoomSelf
	}
	dest, err := s.queries.GetAgentBySlug(ctx, to)
	if errors.Is(err, sql.ErrNoRows) {
		return MessageRoomResolveResult{}, ErrMessageRoomUnknownSlug
	}
	if err != nil {
		return MessageRoomResolveResult{}, err
	}
	if dest.ArchivedAt.Valid {
		return MessageRoomResolveResult{}, ErrMessageRoomArchivedSlug
	}

	originID := strings.TrimSpace(in.OriginThreadID)
	if originID != "" {
		origin, err := s.queries.GetAgentThread(ctx, originID)
		if err == nil && origin.RoomKind == RoomKindPairwise {
			a := strings.TrimSpace(origin.PairAgentIDA.String)
			b := strings.TrimSpace(origin.PairAgentIDB.String)
			if (fromAgent.ID == a || fromAgent.ID == b) &&
				(dest.ID == a || dest.ID == b) {
				return MessageRoomResolveResult{
					ThreadID:       origin.ID,
					SpeakerAgentID: dest.ID,
					RoomKind:       RoomKindPairwise,
				}, nil
			}
		}
	}

	threadID, err := s.ensurePairwiseThread(ctx, fromAgent, dest)
	if err != nil {
		return MessageRoomResolveResult{}, err
	}
	return MessageRoomResolveResult{
		ThreadID:       threadID,
		SpeakerAgentID: dest.ID,
		RoomKind:       RoomKindPairwise,
	}, nil
}

func (s *Service) lookupFromAgent(ctx context.Context, fromID string) (db.Agent, error) {
	agent, err := s.queries.GetAgent(ctx, fromID)
	if err == nil {
		return agent, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return db.Agent{}, err
	}
	agent, err = s.queries.GetAgentBySlug(ctx, fromID)
	if errors.Is(err, sql.ErrNoRows) {
		return db.Agent{}, ErrMessageRoomUnknownSlug
	}
	return agent, err
}

func (s *Service) resolvePlanDestination(
	ctx context.Context,
	to string,
) (MessageRoomResolveResult, error) {
	canon := to
	if !strings.HasPrefix(canon, "thoughts/") {
		canon = "thoughts/" + strings.TrimPrefix(canon, "/")
	}
	if _, err := CanonicalThoughtsPath(canon); err != nil {
		return MessageRoomResolveResult{}, ErrMessageRoomNonPlanPath
	}
	threadID, err := s.EnsureSharedThreadForDoc(ctx, canon, "shared")
	if err != nil {
		return MessageRoomResolveResult{}, err
	}
	if strings.TrimSpace(threadID) == "" {
		return MessageRoomResolveResult{}, ErrMessageRoomNonPlanPath
	}
	thread, err := s.queries.GetAgentThread(ctx, threadID)
	if err != nil {
		return MessageRoomResolveResult{}, err
	}
	if thread.RoomKind == RoomKindBotHome {
		return MessageRoomResolveResult{}, ErrMessageRoomBotHome
	}
	speaker := strings.TrimSpace(thread.AgentID.String)
	if thread.PlanDirRel.Valid {
		if row, err := s.queries.GetPlanWorkspace(
			ctx,
			thread.PlanDirRel.String,
		); err == nil {
			if id := strings.TrimSpace(row.LeadAgentID.String); id != "" {
				speaker = id
			}
		} else if !errors.Is(
			err,
			sql.ErrNoRows,
		) {
			return MessageRoomResolveResult{}, err
		}
	}
	return MessageRoomResolveResult{
		ThreadID:       thread.ID,
		SpeakerAgentID: speaker,
		RoomKind:       RoomKindPlan,
	}, nil
}

func (s *Service) ensurePairwiseThread(
	ctx context.Context,
	fromAgent, dest db.Agent,
) (string, error) {
	leftSlug, rightSlug, err := CanonicalPairSlugs(fromAgent.Slug, dest.Slug)
	if err != nil {
		return "", err
	}
	left, right := fromAgent, dest
	if leftSlug == dest.Slug {
		left, right = dest, fromAgent
	}
	existing, err := s.queries.GetPairwiseThread(ctx, db.GetPairwiseThreadParams{
		PairAgentIDA: sql.NullString{String: left.ID, Valid: true},
		PairAgentIDB: sql.NullString{String: right.ID, Valid: true},
	})
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	cwd := ""
	if strings.TrimSpace(s.thoughtsRoot) != "" {
		if err := SeedPairwiseTree(s.thoughtsRoot, leftSlug, rightSlug); err != nil {
			return "", err
		}
		abs, err := RoomCwdAbs(s.thoughtsRoot, RoomIdentity{
			Kind:  RoomKindPairwise,
			PairA: leftSlug,
			PairB: rightSlug,
		})
		if err != nil {
			return "", err
		}
		cwd = abs
	}
	threadID := uuid.NewString()
	if _, err := s.queries.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:        threadID,
		UserEmail: "shared",
		Title:     leftSlug + " / " + rightSlug,
		Cwd:       cwd,
		LineageID: uuid.NewString(),
		ProjectID: "",
	}); err != nil {
		return "", err
	}
	if err := s.queries.BindAgentThreadPairwise(ctx, db.BindAgentThreadPairwiseParams{
		PairAgentIDA: sql.NullString{String: left.ID, Valid: true},
		PairAgentIDB: sql.NullString{String: right.ID, Valid: true},
		Cwd:          cwd,
		Title:        leftSlug + " / " + rightSlug,
		ID:           threadID,
	}); err != nil {
		return "", err
	}
	return threadID, nil
}

func looksLikeThreadUUID(to string) bool {
	_, err := uuid.Parse(strings.TrimSpace(to))
	return err == nil
}

func isBotHomeDestination(to string) bool {
	trimmed := strings.TrimSpace(to)
	slash := strings.ToLower(trimmed)
	if strings.Contains(slash, "/rooms/dm/") {
		return true
	}
	canon := slash
	if !strings.HasPrefix(canon, "thoughts/") && strings.HasPrefix(canon, "agents/") {
		canon = "thoughts/" + canon
	}
	return strings.HasPrefix(canon, "thoughts/agents/")
}
