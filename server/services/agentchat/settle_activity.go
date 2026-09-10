package agentchat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/CoreyCole/vamos/pkg/db"
)

type SettleHotRoomInput struct {
	ThreadID        string            `json:"thread_id"`
	UsageHot        bool              `json:"usage_hot"`
	Timestamp       string            `json:"timestamp,omitempty"`
	HandoffBody     string            `json:"handoff_body"`
	Files           []string          `json:"files"`
	SpeakerSlug     string            `json:"speaker_slug,omitempty"`
	RoleMemoryFiles map[string]string `json:"role_memory_files,omitempty"`
}

type SettleHotRoomResult struct {
	Rotated    bool   `json:"rotated"`
	Timestamp  string `json:"timestamp,omitempty"`
	HandoffRel string `json:"handoff_rel,omitempty"`
	HistoryRel string `json:"history_rel,omitempty"`
	CutEntryID string `json:"cut_entry_id,omitempty"`
}

func (s *Service) SettleHotRoom(
	ctx context.Context,
	input SettleHotRoomInput,
) (SettleHotRoomResult, error) {
	if strings.TrimSpace(input.ThreadID) == "" {
		return SettleHotRoomResult{}, fmt.Errorf("thread_id is required")
	}
	thread, err := s.queries.GetAgentThread(ctx, input.ThreadID)
	if err != nil {
		return SettleHotRoomResult{}, err
	}
	room, err := RoomIdentityFromThread(s.thoughtsRoot, thread, input.SpeakerSlug)
	if err != nil {
		return SettleHotRoomResult{}, err
	}
	if room.Kind == RoomKindBotHome {
		if err := writeSpeakerRoleMemories(
			s.thoughtsRoot,
			room.SpeakerSlug,
			input.RoleMemoryFiles,
		); err != nil {
			return SettleHotRoomResult{}, err
		}
	}
	if !input.UsageHot {
		return SettleHotRoomResult{}, nil
	}

	rotated, err := RotateRoomCurrentJSONL(s.thoughtsRoot, room, RoomHandoffSpec{
		Timestamp: input.Timestamp,
		Body:      input.HandoffBody,
		Files:     input.Files,
	})
	if err != nil {
		return SettleHotRoomResult{}, err
	}

	cutID, err := s.insertHandoffCutRow(ctx, thread, rotated)
	if err != nil {
		return SettleHotRoomResult{}, err
	}
	return SettleHotRoomResult{
		Rotated:    true,
		Timestamp:  rotated.Timestamp,
		HandoffRel: rotated.HandoffRel,
		HistoryRel: rotated.HistoryRel,
		CutEntryID: cutID,
	}, nil
}

func writeSpeakerRoleMemories(
	thoughtsRoot, slug string,
	files map[string]string,
) error {
	if len(files) == 0 {
		return nil
	}
	if err := validateSlug(slug); err != nil {
		return err
	}
	homeRel := "thoughts/agents/" + slug
	homeAbs, err := AbsFromThoughtsRel(thoughtsRoot, homeRel)
	if err != nil {
		return err
	}
	for name, body := range files {
		clean := filepath.ToSlash(strings.TrimSpace(name))
		clean = strings.TrimPrefix(clean, "/")
		if clean == "" || strings.Contains(clean, "..") {
			return fmt.Errorf("invalid role memory path %q", name)
		}
		abs := filepath.Join(homeAbs, filepath.FromSlash(clean))
		rel, err := filepath.Rel(homeAbs, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("role memory path escapes speaker home: %s", name)
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) insertHandoffCutRow(
	ctx context.Context,
	thread db.AgentThread,
	rotated RoomRotateResult,
) (string, error) {
	parentID := ""
	if thread.HeadEntryID.Valid {
		parentID = strings.TrimSpace(thread.HeadEntryID.String)
	}
	originOrder := s.nextOriginOrder(ctx, thread)
	entryID := uuid.NewString()
	payload, err := json.Marshal(map[string]string{
		"type":        "handoff",
		"id":          entryID,
		"timestamp":   rotated.Timestamp,
		"handoffPath": rotated.HandoffRel,
		"historyPath": rotated.HistoryRel,
		"body":        "handoff",
	})
	if err != nil {
		return "", err
	}

	s.callbackWriteMu.Lock()
	defer s.callbackWriteMu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	q := s.queries.WithTx(tx)
	if err := q.CreateAgentEntry(ctx, db.CreateAgentEntryParams{
		LineageID: thread.LineageID,
		EntryID:   entryID,
		ParentEntryID: sql.NullString{
			String: parentID,
			Valid:  parentID != "",
		},
		EntryType:        "handoff",
		OriginOrder:      originOrder,
		PayloadJson:      string(payload),
		OriginThreadID:   thread.ID,
		SessionTimestamp: time.Now().UTC(),
	}); err != nil {
		return "", err
	}
	if err := q.UpdateAgentThreadHead(ctx, db.UpdateAgentThreadHeadParams{
		HeadEntryID: sql.NullString{String: entryID, Valid: true},
		ID:          thread.ID,
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return entryID, nil
}
