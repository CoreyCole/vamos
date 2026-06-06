package agentchat

import "encoding/json"

type ChatStartRequest struct {
	ProjectID string `json:"project_id"`
	Prompt    string `json:"prompt"`
}

type ChatSteerRequest struct {
	ThreadID string `json:"thread_id"`
	Prompt   string `json:"prompt"`
}

type ChatRunRef struct {
	ProjectID     string `json:"project_id,omitempty"`
	WorkspaceID   string `json:"workspace_id"`
	ThreadID      string `json:"thread_id"`
	RunID         string `json:"run_id"`
	ChatSessionID string `json:"chat_session_id"`
	WebURL        string `json:"web_url"`
	CWD           string `json:"cwd,omitempty"`
	EventAfter    int64  `json:"event_after"`
}

type ChatSteerDisposition struct {
	InfluencesLatest bool
	LatestThreadID   string
	LatestWebURL     string
	Reason           string
}

type ChatAPIResponse struct {
	Type             string          `json:"type"`
	Ref              ChatRunRef      `json:"ref,omitempty"`
	Error            string          `json:"error,omitempty"`
	Reason           string          `json:"reason,omitempty"`
	LatestThreadID   string          `json:"latest_thread_id,omitempty"`
	LatestWebURL     string          `json:"latest_web_url,omitempty"`
	InfluencesLatest bool            `json:"influences_latest,omitempty"`
	QRSPIResult      json.RawMessage `json:"qrspi_result,omitempty"`
}
