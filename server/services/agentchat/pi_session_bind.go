package agentchat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/CoreyCole/vamos/pkg/db"
)

func WritePiSessionHeader(absPath, sessionID, cwd string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	if info, err := os.Stat(absPath); err == nil && info.Size() > 0 {
		return nil
	}
	header := piSessionHeader{
		Type:      "session",
		ID:        sessionID,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Cwd:       cwd,
	}
	raw, err := json.Marshal(header)
	if err != nil {
		return err
	}
	return os.WriteFile(absPath, append(raw, '\n'), 0o644)
}

func (s *Service) createAgentThread(
	ctx context.Context,
	q db.Querier,
	params db.CreateAgentThreadParams,
) (db.AgentThread, error) {
	params = s.attachPlanDirRel(ctx, params)
	thread, err := q.CreateAgentThread(ctx, params)
	if err != nil {
		return db.AgentThread{}, err
	}
	if err := s.writeThreadPiSession(ctx, q, thread); err != nil {
		return db.AgentThread{}, err
	}
	return thread, nil
}

func (s *Service) writeThreadPiSession(
	ctx context.Context,
	q db.Querier,
	thread db.AgentThread,
) error {
	piID := strings.TrimSpace(thread.PiSessionID)
	if piID == "" {
		return nil
	}
	if strings.TrimSpace(thread.RoomKind) == RoomKindPairwise {
		return nil
	}
	absPath, cwd, err := s.threadPiSessionAbs(thread)
	if err != nil {
		return err
	}
	if err := WritePiSessionHeader(absPath, piID, cwd); err != nil {
		return err
	}
	if q == nil {
		return nil
	}
	_, err = q.UpsertAgentSessionIndex(ctx, db.UpsertAgentSessionIndexParams{
		ID:                uuid.NewString(),
		IdentityKind:      "global_pi",
		ArtifactPath:      nullableString(absPath),
		Agent:             defaultAgentSessionAgent,
		ExternalSessionID: nullableString(piID),
		Cwd:               nullableString(cwd),
		ProjectionState:   "unassigned",
		ProjectedThreadID: nullableString(thread.ID),
	})
	return err
}

func (s *Service) threadPiSessionAbs(thread db.AgentThread) (string, string, error) {
	piID := strings.TrimSpace(thread.PiSessionID)
	room, err := RoomIdentityFromThread(
		s.thoughtsRoot,
		thread,
		strings.TrimSpace(thread.AgentSlug.String),
	)
	if err == nil && strings.TrimSpace(s.thoughtsRoot) != "" {
		abs, err := EnsureRoomPiSessionJSONL(s.thoughtsRoot, room, piID)
		if err != nil {
			return "", "", err
		}
		cwd, err := RoomCwdAbs(s.thoughtsRoot, room)
		if err != nil {
			return "", "", err
		}
		return abs, cwd, nil
	}
	cwd := strings.TrimSpace(thread.Cwd)
	abs := filepath.Join(cwd, ".vamos", "sessions", "pi", piID+".jsonl")
	return abs, cwd, nil
}
