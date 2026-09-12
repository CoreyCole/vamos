package agentchat

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/starfederation/datastar-go/datastar"

	"github.com/CoreyCole/vamos/pkg/db"
)

// AcceptedTurn is a persisted prompt + seeded live user message, before Temporal.
type AcceptedTurn struct {
	Thread  db.AgentThread
	Run     db.AgentRun
	Session *db.AgentSession
}

// StartAcceptedTurn starts Temporal for a previously accepted turn.
func (s *Service) StartAcceptedTurn(
	ctx context.Context,
	turn AcceptedTurn,
) (*db.AgentRun, error) {
	return s.startRun(ctx, turn.Thread, turn.Run)
}

// AcceptResumeWorkspaceThread persists a resume prompt and seeds the pending
// user live event without starting Temporal.
func (s *Service) AcceptResumeWorkspaceThread(
	ctx context.Context,
	workspaceID, userEmail, threadID, prompt string,
	attachments ...[]AttachedPath,
) (*AcceptedTurn, error) {
	thread, run, session, err := s.resumeWorkspaceThreadAccepted(
		ctx,
		workspaceID,
		userEmail,
		threadID,
		prompt,
		attachments...,
	)
	if err != nil {
		return nil, err
	}
	if thread == nil || run == nil {
		return nil, err
	}
	turn := &AcceptedTurn{Thread: *thread, Run: *run, Session: session}
	return turn, nil
}

// AcceptStartWorkspaceThread persists a new workspace thread prompt and seeds
// the pending user live event without starting Temporal.
func (s *Service) AcceptStartWorkspaceThread(
	ctx context.Context,
	workspaceID, userEmail, prompt string,
	attachments ...[]AttachedPath,
) (*AcceptedTurn, error) {
	thread, run, session, err := s.startWorkspaceThreadAccepted(
		ctx,
		workspaceID,
		userEmail,
		prompt,
		attachments...,
	)
	if err != nil {
		return nil, err
	}
	if thread == nil || run == nil {
		return nil, err
	}
	turn := &AcceptedTurn{Thread: *thread, Run: *run, Session: session}
	return turn, nil
}

// patchLiveTranscriptSendAccept fat-morphs #agent-chat-live-transcript with the
// seeded user bubble plus #agent-chat-working, then clears $chatDraft and runs
// resetAndFocusComposerScript. Live transcript only — no messages chrome morph.
func (h *Handler) patchLiveTranscriptSendAccept(
	sse *datastar.ServerSentEventGenerator,
	threadID, forkAction string,
) error {
	threadID = strings.TrimSpace(threadID)
	live, cursor := h.service.buildLiveTranscript(threadID)
	state := TranscriptPaneState{
		Cursor:      cursor,
		Live:        live,
		ShowWorking: true,
	}
	if err := sse.PatchElementTempl(
		LiveTranscriptRegion(threadID, state, forkAction),
	); err != nil {
		return err
	}
	return h.resetAndFocusEmbeddedComposer(sse)
}

func (h *Handler) startAcceptedTurnAsync(turn *AcceptedTurn) {
	if h == nil || h.service == nil || turn == nil {
		return
	}
	go func(accepted AcceptedTurn) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if _, err := h.service.StartAcceptedTurn(ctx, accepted); err != nil {
			log.Printf(
				"agentchat: async start run %s failed: %v",
				accepted.Run.ID,
				err,
			)
		}
	}(*turn)
}
