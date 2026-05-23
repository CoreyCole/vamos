package runtime

import "encoding/json"

type ParseContext struct {
	WorkflowType   string
	ExpectedNodeID NodeID
	RunID          string
	ThreadID       string
	SessionID      string
	HeadEntryID    string
	SessionPath    string
}

type ResultParser interface {
	Parse(output string, ctx ParseContext) (any, error)
	CorrectionPrompt(err error, attempt int) string
}

type ResultConverter interface {
	ToWorkflowResult(result any, ctx ParseContext) (WorkflowResult, error)
}

type WorkflowResult struct {
	WorkflowType    string          `json:"workflow_type"`
	SourceNodeID    NodeID          `json:"source_node_id"`
	Status          ResultStatus    `json:"status"`
	Summary         string          `json:"summary"`
	PrimaryArtifact string          `json:"primary_artifact,omitempty"`
	Artifacts       []ArtifactRef   `json:"artifacts,omitempty"`
	DisplayNext     string          `json:"display_next,omitempty"`
	Workspace       string          `json:"workspace,omitempty"`
	Policy          json.RawMessage `json:"policy,omitempty"`
	ConfigDisplay   string          `json:"config_display,omitempty"`
	Outcome         ResultOutcome   `json:"outcome,omitempty"`
	Evidence        EvidenceRef     `json:"evidence"`
	Raw             json.RawMessage `json:"raw,omitempty"`
}

type ArtifactRef struct {
	Role string `json:"role,omitempty"`
	Path string `json:"path"`
}

type EvidenceRef struct {
	RunID       string `json:"run_id,omitempty"`
	ThreadID    string `json:"thread_id,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	HeadEntryID string `json:"head_entry_id,omitempty"`
	SessionPath string `json:"session_path,omitempty"`
}
