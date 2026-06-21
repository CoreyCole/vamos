package pickleball

import "time"

type AppState string

const (
	AppStateIdle       AppState = "idle"
	AppStateGenerating AppState = "generating"
	AppStateBuilding   AppState = "building"
	AppStateSucceeded  AppState = "succeeded"
	AppStateFailed     AppState = "failed"
)

type PickleballSession struct {
	ID              string    `json:"id"`
	UserEmail       string    `json:"user_email"`
	WorkspacePath   string    `json:"workspace_path"`
	CurrentBuildID  string    `json:"current_build_id"`
	LastGoodBuildID string    `json:"last_good_build_id"`
	State           AppState  `json:"state"`
	ActiveRunID     string    `json:"active_run_id"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	LogTail         string    `json:"log_tail,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type PromptRequest struct {
	SessionID string
	Prompt    string
	UserEmail string
}

type PromptAccepted struct {
	SessionID string
	RunID     string
	State     AppState
}

type BuildSnapshot struct {
	BuildID          string    `json:"build_id"`
	ParentBuildID    string    `json:"parent_build_id,omitempty"`
	PromptSummary    string    `json:"prompt_summary"`
	Mode             string    `json:"mode"`
	Status           string    `json:"status"`
	SnapshotPath     string    `json:"snapshot_path"`
	ManifestPath     string    `json:"manifest_path"`
	HTMLThoughtsPath string    `json:"html_thoughts_path"`
	CSVThoughtsPath  string    `json:"csv_thoughts_path"`
	SourceHash       string    `json:"source_hash"`
	HTMLHash         string    `json:"html_hash"`
	CSVHash          string    `json:"csv_hash"`
	CreatedAt        time.Time `json:"created_at"`
}

type PickleballViewModel struct {
	SessionID      string
	State          AppState
	Current        *BuildSnapshot
	LastGood       *BuildSnapshot
	ErrorMessage   string
	LogTail        string
	PromptExamples []string
	Share          ShareModel
}

type ShareModel struct {
	CanWebShare    bool
	PreviewURL     string
	CSVDownloadURL string
}
