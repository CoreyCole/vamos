package agentchat

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type TerminalMetadataEventType string

const (
	TerminalMetadataEventSessionStart    TerminalMetadataEventType = "session_start"
	TerminalMetadataEventAgentStart      TerminalMetadataEventType = "agent_start"
	TerminalMetadataEventAgentEnd        TerminalMetadataEventType = "agent_end"
	TerminalMetadataEventQRSPIResult     TerminalMetadataEventType = "qrspi_result"
	TerminalMetadataEventSessionShutdown TerminalMetadataEventType = "session_shutdown"
)

type TerminalMetadataEvent struct {
	SchemaVersion int                         `json:"schema_version"`
	EventID       string                      `json:"event_id"`
	EventType     TerminalMetadataEventType   `json:"event_type"`
	EventTime     time.Time                   `json:"event_time"`
	Writer        TerminalMetadataWriter      `json:"writer"`
	Pi            TerminalMetadataPi          `json:"pi"`
	Project       TerminalMetadataProject     `json:"project,omitempty"`
	Workspace     TerminalMetadataWorkspace   `json:"workspace,omitempty"`
	Plan          TerminalMetadataPlan        `json:"plan,omitempty"`
	QRSPI         *TerminalMetadataQRSPI      `json:"qrspi,omitempty"`
	Source        TerminalMetadataEventSource `json:"source,omitempty"`
}

type TerminalMetadataWriter struct {
	Name     string `json:"name,omitempty"`
	Version  string `json:"version,omitempty"`
	PID      int    `json:"pid,omitempty"`
	Hostname string `json:"hostname,omitempty"`
}

type TerminalMetadataPi struct {
	SessionID   string `json:"session_id,omitempty"`
	SessionFile string `json:"session_file,omitempty"`
	Cwd         string `json:"cwd,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Model       string `json:"model,omitempty"`
}

type TerminalMetadataProject struct {
	ID         string `json:"id,omitempty"`
	Repository string `json:"repository,omitempty"`
	GitRoot    string `json:"git_root,omitempty"`
	Branch     string `json:"branch,omitempty"`
	Commit     string `json:"commit,omitempty"`
}

type TerminalMetadataWorkspace struct {
	Kind         string `json:"kind,omitempty"`
	WorkspaceID  string `json:"workspace_id,omitempty"`
	AbsolutePath string `json:"absolute_path,omitempty"`
	Role         string `json:"role,omitempty"`
}

type TerminalMetadataPlan struct {
	PlanDir                 string `json:"plan_dir,omitempty"`
	ImplementationWorkspace string `json:"implementation_workspace,omitempty"`
}

type TerminalMetadataQRSPI struct {
	Stage          string          `json:"stage,omitempty"`
	Status         string          `json:"status,omitempty"`
	Outcome        string          `json:"outcome,omitempty"`
	Artifact       string          `json:"artifact,omitempty"`
	WorkflowNodeID string          `json:"workflow_node_id,omitempty"`
	ResultJSON     json.RawMessage `json:"result_json,omitempty"`
	ResultYAML     string          `json:"result_yaml,omitempty"`
	RawResult      string          `json:"raw_result,omitempty"`
	RawResultHash  string          `json:"raw_result_hash,omitempty"`
	Summary        json.RawMessage `json:"summary,omitempty"`
}

type TerminalMetadataEventSource struct {
	MessageEntryID       string `json:"message_entry_id,omitempty"`
	ParentEntryID        string `json:"parent_entry_id,omitempty"`
	AssistantMessageHash string `json:"assistant_message_hash,omitempty"`
}

type TerminalMetadataProjection struct {
	ArtifactPath      string
	ExternalSessionID string
	Cwd               string
	PlanDir           string
	WorkspaceID       string
	WorkflowNodeID    string
	FileSize          int64
	FileMtime         time.Time
	LastIndexedOffset int64
	MetadataJSON      string
	QRSPIResult       *QRSPIResultProjection
}

type QRSPIResultProjection struct {
	ID                  string
	SourceEventID       string
	SessionID           string
	SessionArtifactPath string
	PlanDir             string
	WorkflowNodeID      string
	Stage               string
	Status              string
	Outcome             string
	Artifact            string
	ResultJSON          string
	EventTime           time.Time
}

func ParseTerminalMetadataEvent(line []byte) (TerminalMetadataEvent, error) {
	line = []byte(strings.TrimSpace(string(line)))
	if len(line) == 0 {
		return TerminalMetadataEvent{}, errors.New("terminal metadata event line is empty")
	}
	var event TerminalMetadataEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return TerminalMetadataEvent{}, fmt.Errorf("parse terminal metadata event: %w", err)
	}
	if err := validateTerminalMetadataEvent(event); err != nil {
		return TerminalMetadataEvent{}, err
	}
	return event, nil
}

func NormalizeTerminalMetadataEvent(event TerminalMetadataEvent) (TerminalMetadataProjection, error) {
	if err := validateTerminalMetadataEvent(event); err != nil {
		return TerminalMetadataProjection{}, err
	}

	metadataJSON, err := terminalMetadataSummaryJSON(event)
	if err != nil {
		return TerminalMetadataProjection{}, err
	}

	planDir := normalizeTerminalMetadataPlanDir(event.Plan.PlanDir)
	if planDir == "" {
		planDir = normalizeTerminalMetadataPlanDir(event.Pi.Cwd)
	}
	workflowNodeID := strings.TrimSpace(event.QRSPIWorkflowNodeID())
	projection := TerminalMetadataProjection{
		ArtifactPath:      strings.TrimSpace(event.Pi.SessionFile),
		ExternalSessionID: strings.TrimSpace(event.Pi.SessionID),
		Cwd:               strings.TrimSpace(event.Pi.Cwd),
		PlanDir:           planDir,
		WorkspaceID:       strings.TrimSpace(event.Workspace.WorkspaceID),
		WorkflowNodeID:    workflowNodeID,
		MetadataJSON:      metadataJSON,
	}

	if event.EventType == TerminalMetadataEventQRSPIResult || event.QRSPI != nil {
		if event.QRSPI == nil {
			return TerminalMetadataProjection{}, errors.New("qrspi metadata event missing qrspi payload")
		}
		if planDir == "" {
			return TerminalMetadataProjection{}, errors.New("qrspi metadata event missing plan_dir")
		}
		resultJSON, err := terminalMetadataQRSPIResultJSON(*event.QRSPI)
		if err != nil {
			return TerminalMetadataProjection{}, err
		}
		projection.QRSPIResult = &QRSPIResultProjection{
			ID:                  "pi-metadata:" + strings.TrimSpace(event.EventID),
			SourceEventID:       strings.TrimSpace(event.EventID),
			SessionID:           strings.TrimSpace(event.Pi.SessionID),
			SessionArtifactPath: strings.TrimSpace(event.Pi.SessionFile),
			PlanDir:             planDir,
			WorkflowNodeID:      workflowNodeID,
			Stage:               strings.TrimSpace(event.QRSPI.Stage),
			Status:              strings.TrimSpace(event.QRSPI.Status),
			Outcome:             strings.TrimSpace(event.QRSPI.Outcome),
			Artifact:            strings.TrimSpace(event.QRSPI.Artifact),
			ResultJSON:          resultJSON,
			EventTime:           event.EventTime,
		}
	}

	return projection, nil
}

func validateTerminalMetadataEvent(event TerminalMetadataEvent) error {
	if event.SchemaVersion != 1 {
		return fmt.Errorf("unsupported terminal metadata schema_version %d", event.SchemaVersion)
	}
	if strings.TrimSpace(event.EventID) == "" {
		return errors.New("terminal metadata event_id is required")
	}
	if strings.TrimSpace(string(event.EventType)) == "" {
		return errors.New("terminal metadata event_type is required")
	}
	if event.EventTime.IsZero() {
		return errors.New("terminal metadata event_time is required")
	}
	return nil
}

func (event TerminalMetadataEvent) QRSPIWorkflowNodeID() string {
	if event.QRSPI != nil && strings.TrimSpace(event.QRSPI.WorkflowNodeID) != "" {
		return event.QRSPI.WorkflowNodeID
	}
	if event.QRSPI != nil && strings.TrimSpace(event.QRSPI.Stage) != "" {
		return event.QRSPI.Stage
	}
	return ""
}

func terminalMetadataSummaryJSON(event TerminalMetadataEvent) (string, error) {
	summary := map[string]any{
		"schema_version": event.SchemaVersion,
		"event_id":       event.EventID,
		"event_type":     event.EventType,
		"event_time":     event.EventTime,
		"writer":         event.Writer,
		"pi":             event.Pi,
		"project":        event.Project,
		"workspace":      event.Workspace,
		"plan":           event.Plan,
		"source":         event.Source,
	}
	if event.QRSPI != nil {
		summary["qrspi"] = event.QRSPI
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		return "", fmt.Errorf("encode terminal metadata summary: %w", err)
	}
	return string(encoded), nil
}

func terminalMetadataQRSPIResultJSON(qrspi TerminalMetadataQRSPI) (string, error) {
	if len(qrspi.ResultJSON) > 0 && json.Valid(qrspi.ResultJSON) {
		return string(qrspi.ResultJSON), nil
	}
	encoded, err := json.Marshal(qrspi)
	if err != nil {
		return "", fmt.Errorf("encode qrspi metadata result: %w", err)
	}
	return string(encoded), nil
}

func normalizeTerminalMetadataPlanDir(raw string) string {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return ""
	}
	clean = filepath.ToSlash(filepath.Clean(clean))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	if strings.HasPrefix(clean, "thoughts/") {
		root := planDirectoryRoot(clean)
		if root != "" {
			return filepath.ToSlash(root)
		}
		return clean
	}
	marker := "/thoughts/"
	if idx := strings.LastIndex(clean, marker); idx >= 0 {
		candidate := "thoughts/" + clean[idx+len(marker):]
		if root := planDirectoryRoot(candidate); root != "" {
			return filepath.ToSlash(root)
		}
		return candidate
	}
	return ""
}
