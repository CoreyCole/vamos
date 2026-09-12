package agentchat

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	conversationworkflow "github.com/CoreyCole/vamos/pkg/agents/workflows/conversation"
	"github.com/CoreyCole/vamos/pkg/db"
)

// AcceptedTurn is a persisted prompt + seeded live user message, before Temporal.
type AcceptedTurn struct {
	Thread  db.AgentThread
	Run     db.AgentRun
	Session *db.AgentSession
	// ThreadMail, when set, starts ThreadInbox via SignalWithStart instead of RunTurn.
	ThreadMail *conversation.ThreadMail
}

// StartAcceptedTurn starts Temporal for a previously accepted turn.
func (s *Service) StartAcceptedTurn(
	ctx context.Context,
	turn AcceptedTurn,
) (*db.AgentRun, error) {
	if turn.ThreadMail != nil {
		if err := s.StartAcceptedThreadMail(ctx, *turn.ThreadMail); err != nil {
			return nil, err
		}
		return &turn.Run, nil
	}
	return s.startRun(ctx, turn.Thread, turn.Run)
}

// StartAcceptedThreadMail signals ThreadInbox for an already-inserted op.
// Does not re-insert the op (AcceptResumeFreeformThread already did).
func (s *Service) StartAcceptedThreadMail(
	ctx context.Context,
	mail conversation.ThreadMail,
) error {
	if s.temporal == nil {
		return fmt.Errorf("temporal not configured")
	}
	threadID := strings.TrimSpace(mail.ThreadID)
	opID := strings.TrimSpace(mail.OpID)
	if threadID == "" || opID == "" {
		return fmt.Errorf("thread_id and op_id are required")
	}
	workflowID := conversation.ThreadWorkflowID(threadID)
	if _, err := s.temporal.SignalWithStartWorkflow(
		ctx,
		workflowID,
		conversation.ThreadMailSignal,
		mail,
		conversationworkflow.ThreadInboxWorkflow,
		conversation.ThreadWorkflowInput{ThreadID: threadID},
	); err != nil {
		_ = s.queries.DeleteAgentThreadOp(ctx, db.DeleteAgentThreadOpParams{
			ThreadID: threadID,
			OpID:     opID,
		})
		return err
	}
	return nil
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

// AcceptResumeFreeformThread persists thread-mail op + queued run and seeds the
// pending user live event without SignalWithStart (Temporal starts async).
func (s *Service) AcceptResumeFreeformThread(
	ctx context.Context,
	userEmail, threadID, prompt string,
	attachments ...[]AttachedPath,
) (*AcceptedTurn, error) {
	if s.temporal == nil {
		return nil, fmt.Errorf("temporal not configured")
	}
	threadID = strings.TrimSpace(threadID)
	prompt = strings.TrimSpace(prompt)
	userEmail = strings.TrimSpace(userEmail)
	if threadID == "" {
		return nil, fmt.Errorf("thread_id is required")
	}
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	thread, err := s.queries.GetAgentThreadForUser(ctx, db.GetAgentThreadForUserParams{
		ID:        threadID,
		UserEmail: userEmail,
	})
	if err != nil {
		return nil, err
	}
	if err := GuardHumanCompose(thread); err != nil {
		return nil, err
	}
	if err := GuardEnqueueDestination(thread, EnqueueMail{FromKind: EnqueueFromUser}); err != nil {
		return nil, err
	}

	opID := uuid.NewString()
	speakerID := speakerAgentIDForMail(thread, EnqueueFromUser, "")
	attached := flattenAttachedPaths(attachments)
	rows, err := s.queries.InsertAgentThreadOp(ctx, db.InsertAgentThreadOpParams{
		ThreadID: thread.ID,
		OpID:     opID,
		SpeakerAgentID: sql.NullString{
			String: speakerID,
			Valid:  speakerID != "",
		},
		FromKind:      EnqueueFromUser,
		FromAgentID:   sql.NullString{},
		FromUserEmail: userEmail,
		Body:          prompt,
	})
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, fmt.Errorf("duplicate thread op")
	}

	mail := conversation.ThreadMail{
		ThreadID:       thread.ID,
		OpID:           opID,
		SpeakerAgentID: speakerID,
		FromKind:       EnqueueFromUser,
		FromUserEmail:  userEmail,
		Body:           prompt,
		Attachments:    attachmentPaths(attached),
	}
	run, err := s.createQueuedRun(ctx, s.queries, thread, mail, speakerID)
	if err != nil {
		_ = s.queries.DeleteAgentThreadOp(ctx, db.DeleteAgentThreadOpParams{
			ThreadID: thread.ID,
			OpID:     opID,
		})
		return nil, err
	}
	if err := s.seedPendingUserPrompt(thread, run); err != nil {
		_ = s.queries.DeleteAgentThreadOp(ctx, db.DeleteAgentThreadOpParams{
			ThreadID: thread.ID,
			OpID:     opID,
		})
		return nil, err
	}
	return &AcceptedTurn{
		Thread:     thread,
		Run:        run,
		ThreadMail: &mail,
	}, nil
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

// acceptEmbeddedFreeformResumeV2 is the workbench_v2=1 accept path for freeform /
// SharedThreadChat resume: persist+seed, fat-morph live transcript, composer
// reset, then async ThreadInbox SignalWithStart.
func (h *Handler) acceptEmbeddedFreeformResumeV2(
	c echo.Context,
	userEmail, threadID, prompt string,
	attachments []AttachedPath,
) error {
	turn, err := h.service.AcceptResumeFreeformThread(
		c.Request().Context(),
		userEmail,
		threadID,
		prompt,
		attachments,
	)
	if err != nil {
		return echo.NewHTTPError(resumeComposeHTTPStatus(err), err.Error())
	}
	if err := h.service.ClearThreadDraft(
		c.Request().Context(),
		userEmail,
		threadID,
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	runID := turn.Run.ID
	if turn.Run.WorkspaceID.Valid {
		if err := h.service.PersistEmbeddedChatSelection(
			c.Request().Context(),
			userEmail,
			EmbeddedChatSelection{
				WorkspaceID: turn.Run.WorkspaceID.String,
				ThreadID:    threadID,
				RunID:       runID,
				Scope:       EmbeddedChatSelectionScopeFreeform,
			},
		); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	if err := h.patchLiveTranscriptSendAccept(
		sse,
		threadID,
		freeformForkAction(threadID),
	); err != nil {
		return err
	}
	h.startAcceptedTurnAsync(turn)
	return nil
}
