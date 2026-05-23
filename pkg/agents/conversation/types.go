package conversation

import (
	"fmt"
	"time"
)

type RunTrigger string

const (
	RunTriggerSend   RunTrigger = "send"
	RunTriggerResume RunTrigger = "resume"
	RunTriggerFork   RunTrigger = "fork"
)

const (
	EventCheckpoint  = "checkpoint"
	EventRunComplete = "run_complete"
	EventRunFailed   = "run_failed"
)

type SnapshotHeader struct {
	SessionID       string `json:"session_id"`
	ParentSessionID string `json:"parent_session_id,omitempty"`
	Cwd             string `json:"cwd"`
}

type SnapshotEntry struct {
	LineageID     string    `json:"lineage_id"`
	EntryID       string    `json:"entry_id"`
	ParentEntryID string    `json:"parent_entry_id,omitempty"`
	EntryType     string    `json:"entry_type"`
	Timestamp     time.Time `json:"timestamp"`
	OriginOrder   int64     `json:"origin_order"`
	PayloadJSON   string    `json:"payload_json"`
}

type Snapshot struct {
	Header      SnapshotHeader  `json:"header"`
	LineageID   string          `json:"lineage_id"`
	HeadEntryID string          `json:"head_entry_id,omitempty"`
	Entries     []SnapshotEntry `json:"entries"`
}

type SnapshotRef struct {
	LineageID   string `json:"lineage_id"`
	HeadEntryID string `json:"head_entry_id,omitempty"`
	SessionPath string `json:"session_path,omitempty"`
}

type RunInput struct {
	WorkspaceID            string      `json:"workspace_id"`
	SessionID              string      `json:"session_id,omitempty"`
	ChatSessionID          string      `json:"chat_session_id,omitempty"`
	RunID                  string      `json:"run_id"`
	ThreadID               string      `json:"thread_id"`
	Trigger                RunTrigger  `json:"trigger"`
	Prompt                 string      `json:"prompt"`
	Context                string      `json:"context,omitempty"`
	Cwd                    string      `json:"cwd"`
	RootDocPath            string      `json:"root_doc_path"`
	ThinkingLevel          string      `json:"thinking_level"`
	CallbackEndpoint       string      `json:"callback_endpoint"`
	SnapshotLoaderEndpoint string      `json:"snapshot_loader_endpoint,omitempty"`
	SnapshotRef            SnapshotRef `json:"snapshot_ref"`
}

type Checkpoint struct {
	WorkspaceID   string          `json:"workspace_id,omitempty"`
	SessionID     string          `json:"session_id,omitempty"`
	ChatSessionID string          `json:"chat_session_id,omitempty"`
	RunID         string          `json:"run_id"`
	ThreadID      string          `json:"thread_id"`
	HeadEntryID   string          `json:"head_entry_id,omitempty"`
	TurnIndex     int             `json:"turn_index"`
	Header        SnapshotHeader  `json:"header"`
	NewEntries    []SnapshotEntry `json:"new_entries"`
	EventKey      string          `json:"event_key,omitempty"`
}

type EventEnvelope struct {
	WorkspaceID   string `json:"workspace_id,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	ChatSessionID string `json:"chat_session_id,omitempty"`
	RunID         string `json:"run_id"`
	ThreadID      string `json:"thread_id"`
	EventType     string `json:"event_type"`
	PayloadJSON   string `json:"payload_json"`
	EventKey      string `json:"event_key,omitempty"`
}

type RunResult struct {
	WorkspaceID   string `json:"workspace_id,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	ChatSessionID string `json:"chat_session_id,omitempty"`
	RunID         string `json:"run_id"`
	ThreadID      string `json:"thread_id"`
	HeadEntryID   string `json:"head_entry_id,omitempty"`
	SessionPath   string `json:"session_path"`
	RootDocPath   string `json:"root_doc_path"`
	MetadataJSON  string `json:"metadata_json"`
	EventKey      string `json:"event_key,omitempty"`
}

type RunFailure struct {
	WorkspaceID   string `json:"workspace_id,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	ChatSessionID string `json:"chat_session_id,omitempty"`
	RunID         string `json:"run_id"`
	ThreadID      string `json:"thread_id"`
	HeadEntryID   string `json:"head_entry_id,omitempty"`
	SessionPath   string `json:"session_path"`
	RootDocPath   string `json:"root_doc_path"`
	ErrorMessage  string `json:"error_message"`
	EventKey      string `json:"event_key,omitempty"`
}

type ActivityFailureInput struct {
	WorkspaceID   string `json:"workspace_id,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	ChatSessionID string `json:"chat_session_id,omitempty"`
	RunID         string `json:"run_id"`
	ThreadID      string `json:"thread_id"`
	RootDocPath   string `json:"root_doc_path"`
	ErrorMessage  string `json:"error_message"`
	EventKey      string `json:"event_key,omitempty"`
}

func NewActivityFailureInput(input RunInput, err error) ActivityFailureInput {
	errorMessage := "run conversation turn failed"
	if err != nil {
		errorMessage = fmt.Sprintf("run conversation turn failed: %v", err)
	}
	return ActivityFailureInput{
		WorkspaceID:   input.WorkspaceID,
		SessionID:     input.SessionID,
		ChatSessionID: input.ChatSessionID,
		RunID:         input.RunID,
		ThreadID:      input.ThreadID,
		RootDocPath:   input.RootDocPath,
		ErrorMessage:  errorMessage,
		EventKey:      input.RunID + ":run_failed",
	}
}
