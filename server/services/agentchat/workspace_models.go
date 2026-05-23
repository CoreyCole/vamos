package agentchat

import (
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

type WorkspaceWorkflowType string

const (
	WorkspaceWorkflowFreeform WorkspaceWorkflowType = "freeform"
	WorkspaceWorkflowQRSPI    WorkspaceWorkflowType = "qrspi"
)

type WorkspaceSource string

const (
	WorkspaceSourceWeb      WorkspaceSource = "web"
	WorkspaceSourceTerminal WorkspaceSource = "terminal"
	WorkspaceSourceImported WorkspaceSource = "imported"
)

type AgentSessionSource string

const (
	AgentSessionSourceTerminal AgentSessionSource = "terminal"
	AgentSessionSourceWeb      AgentSessionSource = "web"
	AgentSessionSourceAdopted  AgentSessionSource = "adopted"
)

type WorkspaceCreateInput struct {
	UserEmail     string
	Title         string
	RootDocPath  string
	Cwd           string
	WorkflowType  WorkspaceWorkflowType
	WorkflowState json.RawMessage
	Source        WorkspaceSource
}

type SessionImportInput struct {
	SessionPath          string
	Source               AgentSessionSource
	ExplicitWorkspaceID  string
	ExplicitWorkspaceDir string
	UserEmail            string
}

//nolint:tagliatelle // Internal import API uses existing snake_case JSON fields.
type SessionImportResult struct {
	SessionID         string               `json:"session_id"`
	WorkspaceID       string               `json:"workspace_id,omitempty"`
	ThreadID          string               `json:"thread_id,omitempty"`
	ImportedHeadEntry string               `json:"imported_head_entry,omitempty"`
	Status            string               `json:"status"`
	Diverged          bool                 `json:"diverged"`
	Stats             PiSessionImportStats `json:"stats,omitempty"`
}

type AppendWorkspaceEventInput struct {
	WorkspaceID     string
	EventType       string
	ActorEmail      string
	ActorType       string
	ThreadID        string
	SessionID       string
	RunID           string
	ArtifactRelPath string
	DocPath    string
	CommentID       string
	PayloadJSON     string
	EventKey        string
}

func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

var _ = db.Workspace{}
