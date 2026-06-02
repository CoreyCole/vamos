package chatsession

import (
	"encoding/json"
	"strconv"
	"strings"
)

type ProjectedMessage struct {
	ID      string `json:"id"`
	Seq     int64  `json:"seq"`
	Role    string `json:"role"`
	Content string `json:"content"`
	RunID   string `json:"run_id,omitempty"`
	Actor   string `json:"actor,omitempty"`
}

type ProjectedRun struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	StartedSeq   int64  `json:"started_seq,omitempty"`
	CompletedSeq int64  `json:"completed_seq,omitempty"`
	Summary      string `json:"summary,omitempty"`
}

type ProjectedToolEvent struct {
	ID      string `json:"id"`
	Seq     int64  `json:"seq"`
	Status  string `json:"status"`
	Name    string `json:"name,omitempty"`
	RunID   string `json:"run_id,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type ProjectedParticipant struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

type ProjectedArtifact struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	Seq  int64  `json:"seq"`
}

type ChatProjection struct {
	SessionID    string                 `json:"session_id"`
	LastSeq      int64                  `json:"last_seq"`
	Messages     []ProjectedMessage     `json:"messages"`
	Runs         []ProjectedRun         `json:"runs"`
	Tools        []ProjectedToolEvent   `json:"tools"`
	Participants []ProjectedParticipant `json:"participants"`
	Artifacts    []ProjectedArtifact    `json:"artifacts"`
	Tree         ChatTreeProjection     `json:"tree"`
}

type ChatTreeProjection struct {
	WorkspaceID       string              `json:"workspace_id"`
	CurrentSessionID  string              `json:"current_session_id"`
	SelectedNodeID    string              `json:"selected_node_id"`
	ActivePathNodeIDs []string            `json:"active_path_node_ids"`
	Nodes             []WorkspaceTreeNode `json:"nodes"`
}

type WorkspaceTreeNode struct {
	ID              string `json:"id"`
	ParentID        string `json:"parent_id,omitempty"`
	Depth           int    `json:"depth"`
	WorkspaceID     string `json:"workspace_id,omitempty"`
	SessionID       string `json:"session_id"`
	EventSeq        int64  `json:"event_seq"`
	Kind            string `json:"kind"`
	Label           string `json:"label"`
	Summary         string `json:"summary,omitempty"`
	Status          string `json:"status,omitempty"`
	ActivePath      bool   `json:"active_path"`
	Selected        bool   `json:"selected"`
	Branch          bool   `json:"branch"`
	AnnotationCount int    `json:"annotation_count"`
	UnresolvedCount int    `json:"unresolved_count"`
	CanFork         bool   `json:"can_fork"`
	CanPromote      bool   `json:"can_promote"`
	DocumentHref    string `json:"document_href,omitempty"`
	ChatHref        string `json:"chat_href,omitempty"`
}

type ChatBaseline struct {
	SessionID         string
	ParentSessionID   string
	ForkedFromSeq     int64
	MessagesJSON      json.RawMessage
	RunsJSON          json.RawMessage
	ArtifactsJSON     json.RawMessage
	ParticipantsJSON  json.RawMessage
	TopologyJSON      json.RawMessage
	SelectedStateJSON json.RawMessage
}

func ApplyEvent(proj ChatProjection, event ChatEvent) (ChatProjection, error) {
	proj.SessionID = event.SessionID
	if event.Seq > proj.LastSeq {
		proj.LastSeq = event.Seq
	}
	if proj.Tree.CurrentSessionID == "" {
		proj.Tree.CurrentSessionID = event.SessionID
	}
	payload := map[string]any{}
	if len(strings.TrimSpace(string(event.PayloadJSON))) > 0 {
		if err := json.Unmarshal(event.PayloadJSON, &payload); err != nil {
			return ChatProjection{}, err
		}
	}

	switch event.EventType {
	case EventMessageCreated, EventMessageStarted, EventMessageCheckpointed, EventMessageCompleted:
		msg := ProjectedMessage{
			ID:      stringFromPayload(payload, "id", nodeID(event)),
			Seq:     event.Seq,
			Role:    stringFromPayload(payload, "role", "message"),
			Content: stringFromPayload(payload, "content", ""),
			RunID:   firstNonEmpty(event.RunID, stringFromPayload(payload, "run_id", "")),
			Actor: firstNonEmpty(
				event.ActorParticipantID,
				stringFromPayload(payload, "actor", ""),
			),
		}
		proj.Messages = upsertMessage(proj.Messages, msg)
		proj.Tree.Nodes = upsertTreeNode(proj.Tree.Nodes, WorkspaceTreeNode{
			ID:         msg.ID,
			SessionID:  event.SessionID,
			EventSeq:   event.Seq,
			Kind:       "message",
			Label:      msg.Role,
			Summary:    msg.Content,
			ActivePath: true,
			CanFork:    true,
		})
	case EventRunStarted:
		id := firstNonEmpty(
			event.RunID,
			stringFromPayload(
				payload,
				"id",
				stringFromPayload(payload, "run_id", nodeID(event)),
			),
		)
		proj.Runs = upsertRun(
			proj.Runs,
			ProjectedRun{ID: id, Status: "running", StartedSeq: event.Seq},
		)
	case EventRunCompleted:
		id := firstNonEmpty(
			event.RunID,
			stringFromPayload(
				payload,
				"id",
				stringFromPayload(payload, "run_id", nodeID(event)),
			),
		)
		proj.Runs = upsertRun(
			proj.Runs,
			ProjectedRun{
				ID:           id,
				Status:       "complete",
				CompletedSeq: event.Seq,
				Summary:      stringFromPayload(payload, "summary", ""),
			},
		)
		if summary := stringFromPayload(payload, "summary", ""); summary != "" {
			proj.Tree.Nodes = upsertTreeNode(
				proj.Tree.Nodes,
				WorkspaceTreeNode{
					ID:         nodeID(event),
					SessionID:  event.SessionID,
					EventSeq:   event.Seq,
					Kind:       "run_result",
					Label:      "Run completed",
					Summary:    summary,
					ActivePath: true,
					CanFork:    true,
				},
			)
		}
	case EventToolStarted, EventToolUpdated, EventToolCompleted, EventToolFailed:
		status := "updated"
		switch event.EventType {
		case EventToolStarted:
			status = "running"
		case EventToolCompleted:
			status = "complete"
		case EventToolFailed:
			status = "failed"
		}
		tool := ProjectedToolEvent{
			ID: firstNonEmpty(
				stringFromPayload(payload, "tool_call_id", ""),
				stringFromPayload(payload, "id", nodeID(event)),
			),
			Seq:     event.Seq,
			Status:  status,
			Name:    stringFromPayload(payload, "tool_name", ""),
			RunID:   firstNonEmpty(event.RunID, stringFromPayload(payload, "run_id", "")),
			Summary: stringFromPayload(payload, "summary", ""),
		}
		proj.Tools = upsertTool(proj.Tools, tool)
	case EventFileWritten, EventFileEdited:
		artifact := ProjectedArtifact{
			Path: stringFromPayload(payload, "path", ""),
			Kind: strings.TrimPrefix(string(event.EventType), "file."),
			Seq:  event.Seq,
		}
		if artifact.Path != "" {
			proj.Artifacts = upsertArtifact(proj.Artifacts, artifact)
		}
	case EventRunFailed:
		id := firstNonEmpty(
			event.RunID,
			stringFromPayload(
				payload,
				"id",
				stringFromPayload(payload, "run_id", nodeID(event)),
			),
		)
		proj.Runs = upsertRun(
			proj.Runs,
			ProjectedRun{ID: id, Status: "failed", CompletedSeq: event.Seq},
		)
	case EventForkCreated, EventBaselineCopied, EventPromoted:
		proj.Tree.Nodes = upsertTreeNode(
			proj.Tree.Nodes,
			WorkspaceTreeNode{
				ID:        nodeID(event),
				SessionID: event.SessionID,
				EventSeq:  event.Seq,
				Kind:      string(event.EventType),
				Label:     string(event.EventType),
				Summary:   stringFromPayload(payload, "summary", ""),
				Branch:    event.EventType == EventForkCreated,
			},
		)
	default:
		if strings.HasPrefix(string(event.EventType), "artifact.") {
			artifact := ProjectedArtifact{
				Path: stringFromPayload(payload, "path", ""),
				Kind: stringFromPayload(payload, "kind", "artifact"),
				Seq:  event.Seq,
			}
			if artifact.Path != "" {
				proj.Artifacts = upsertArtifact(proj.Artifacts, artifact)
			}
		}
	}
	return proj, nil
}

func RebuildProjection(
	events []ChatEvent,
	baseline *ChatBaseline,
) (ChatProjection, error) {
	proj := ChatProjection{}
	if baseline != nil {
		proj.SessionID = baseline.SessionID
		proj.LastSeq = baseline.ForkedFromSeq
		_ = json.Unmarshal(defaultJSON(baseline.MessagesJSON), &proj.Messages)
		_ = json.Unmarshal(defaultJSON(baseline.RunsJSON), &proj.Runs)
		_ = json.Unmarshal(defaultJSON(baseline.ParticipantsJSON), &proj.Participants)
		_ = json.Unmarshal(defaultJSON(baseline.ArtifactsJSON), &proj.Artifacts)
		_ = json.Unmarshal(defaultJSON(baseline.TopologyJSON), &proj.Tree)
	}
	var err error
	for _, event := range events {
		proj, err = ApplyEvent(proj, event)
		if err != nil {
			return ChatProjection{}, err
		}
	}
	return proj, nil
}

func stringFromPayload(payload map[string]any, key, fallback string) string {
	v, ok := payload[key]
	if !ok || v == nil {
		return fallback
	}
	if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	return fallback
}

func nodeID(event ChatEvent) string {
	return event.SessionID + ":" + strconv.FormatInt(event.Seq, 10)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func upsertMessage(items []ProjectedMessage, item ProjectedMessage) []ProjectedMessage {
	for i := range items {
		if items[i].ID == item.ID {
			items[i] = item
			return items
		}
	}
	return append(items, item)
}

func upsertRun(items []ProjectedRun, item ProjectedRun) []ProjectedRun {
	for i := range items {
		if items[i].ID == item.ID {
			if item.Status != "" {
				items[i].Status = item.Status
			}
			if item.StartedSeq != 0 {
				items[i].StartedSeq = item.StartedSeq
			}
			if item.CompletedSeq != 0 {
				items[i].CompletedSeq = item.CompletedSeq
			}
			if item.Summary != "" {
				items[i].Summary = item.Summary
			}
			return items
		}
	}
	return append(items, item)
}

func upsertTool(items []ProjectedToolEvent, item ProjectedToolEvent) []ProjectedToolEvent {
	for i := range items {
		if items[i].ID == item.ID {
			if item.Seq != 0 {
				items[i].Seq = item.Seq
			}
			if item.Status != "" {
				items[i].Status = item.Status
			}
			if item.Name != "" {
				items[i].Name = item.Name
			}
			if item.RunID != "" {
				items[i].RunID = item.RunID
			}
			if item.Summary != "" {
				items[i].Summary = item.Summary
			}
			return items
		}
	}
	return append(items, item)
}

func upsertArtifact(
	items []ProjectedArtifact,
	item ProjectedArtifact,
) []ProjectedArtifact {
	for i := range items {
		if items[i].Path == item.Path {
			items[i] = item
			return items
		}
	}
	return append(items, item)
}

func upsertTreeNode(
	items []WorkspaceTreeNode,
	item WorkspaceTreeNode,
) []WorkspaceTreeNode {
	for i := range items {
		if items[i].ID == item.ID {
			items[i] = item
			return items
		}
	}
	return append(items, item)
}
