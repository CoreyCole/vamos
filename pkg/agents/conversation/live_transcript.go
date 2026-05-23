package conversation

import (
	"encoding/json"
	"fmt"
	"strings"
)

type LiveTurnItemKind string

const (
	LiveTurnUserMessage      LiveTurnItemKind = "user_message"
	LiveTurnAssistantMessage LiveTurnItemKind = "assistant_message"
	LiveTurnThinking         LiveTurnItemKind = "thinking"
	LiveTurnToolCall         LiveTurnItemKind = "tool_call"
	LiveTurnToolExecution    LiveTurnItemKind = "tool_execution"
	LiveTurnToolResult       LiveTurnItemKind = "tool_result"
)

type ToolExecutionStatus string

const (
	ToolExecutionPending ToolExecutionStatus = "pending"
	ToolExecutionRunning ToolExecutionStatus = "running"
	ToolExecutionDone    ToolExecutionStatus = "done"
	ToolExecutionError   ToolExecutionStatus = "error"
)

type LiveTurnItem struct {
	Key         string
	Kind        LiveTurnItemKind
	Role        string
	MessageJSON json.RawMessage
	ToolCallID  string
	ToolName    string
	Status      ToolExecutionStatus
	ResultJSON  json.RawMessage
	IsFinal     bool
}

type LiveTurnState struct {
	RunID string
	Items []LiveTurnItem
}

type LiveTurnReducer struct {
	runID              string
	items              []LiveTurnItem
	indexByKey         map[string]int
	nextMessageIndex   int
	currentMessageKey  string
	currentMessageRole string
}

type liveMessageEnvelope struct {
	Message json.RawMessage `json:"message"`
}

type liveMessageMeta struct {
	Role       string `json:"role"`
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
}

type liveToolExecutionStartPayload struct {
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	Args       json.RawMessage `json:"args"`
}

type liveToolExecutionUpdatePayload struct {
	ToolCallID    string          `json:"toolCallId"`
	ToolName      string          `json:"toolName"`
	Args          json.RawMessage `json:"args"`
	PartialResult json.RawMessage `json:"partialResult"`
}

type liveToolExecutionEndPayload struct {
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	Result     json.RawMessage `json:"result"`
	IsError    bool            `json:"isError"`
}

func SemanticMessageFromEnvelope(env EventEnvelope) (role, content string, ok bool) {
	if env.EventType != "message_end" {
		return "", "", false
	}
	var payload liveMessageEnvelope
	if err := json.Unmarshal(
		[]byte(env.PayloadJSON),
		&payload,
	); err != nil ||
		len(payload.Message) == 0 {
		return "", "", false
	}
	var message struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}
	if err := json.Unmarshal(payload.Message, &message); err != nil {
		return "", "", false
	}
	role = strings.TrimSpace(message.Role)
	content = liveContentString(message.Content)
	return role, content, role != "" && strings.TrimSpace(content) != ""
}

func liveContentString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, part := range v {
			if text := liveContentString(part); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	case map[string]any:
		if text, ok := v["text"]; ok {
			return liveContentString(text)
		}
		if content, ok := v["content"]; ok {
			return liveContentString(content)
		}
	}
	return ""
}

func NewLiveTurnReducer() *LiveTurnReducer {
	reducer := &LiveTurnReducer{}
	reducer.Reset("")
	return reducer
}

func (r *LiveTurnReducer) Apply(env EventEnvelope) (bool, error) {
	runID := strings.TrimSpace(env.RunID)
	if r.indexByKey == nil {
		r.Reset(runID)
	} else if runID != r.runID {
		r.Reset(runID)
	}

	switch env.EventType {
	case "message_start", "message_update", "message_end":
		return r.applyMessageEvent(env)
	case "tool_execution_start", "tool_execution_update", "tool_execution_end":
		return r.applyToolExecutionEvent(env)
	default:
		return false, nil
	}
}

func (r *LiveTurnReducer) Snapshot() LiveTurnState {
	state := LiveTurnState{RunID: r.runID, Items: make([]LiveTurnItem, len(r.items))}
	for i, item := range r.items {
		state.Items[i] = cloneLiveTurnItem(item)
	}
	return state
}

func (r *LiveTurnReducer) Reset(runID string) {
	r.runID = strings.TrimSpace(runID)
	r.items = nil
	r.indexByKey = make(map[string]int)
	r.nextMessageIndex = 0
	r.currentMessageKey = ""
	r.currentMessageRole = ""
}

func (r *LiveTurnReducer) applyMessageEvent(env EventEnvelope) (bool, error) {
	var payload liveMessageEnvelope
	if err := json.Unmarshal([]byte(env.PayloadJSON), &payload); err != nil {
		return false, err
	}
	if len(payload.Message) == 0 {
		return false, nil
	}

	var meta liveMessageMeta
	if err := json.Unmarshal(payload.Message, &meta); err != nil {
		return false, err
	}

	role := strings.TrimSpace(meta.Role)
	kind, ok := liveTurnMessageKind(role)
	if !ok {
		return false, nil
	}

	key := r.currentMessageKey
	if env.EventType == "message_end" {
		if key == "" || r.currentMessageRole != role {
			key = r.appendItem(LiveTurnItem{Key: r.nextMessageKey(), Kind: kind})
		}
	} else if key == "" || r.currentMessageRole != role {
		key = r.appendItem(LiveTurnItem{Key: r.nextMessageKey(), Kind: kind})
		r.currentMessageKey = key
		r.currentMessageRole = role
	}

	r.upsertItem(key, LiveTurnItem{
		Key:         key,
		Kind:        kind,
		Role:        role,
		MessageJSON: cloneRawMessage(payload.Message),
		ToolCallID:  strings.TrimSpace(meta.ToolCallID),
		ToolName:    strings.TrimSpace(meta.ToolName),
		IsFinal:     env.EventType == "message_end",
	})

	if env.EventType == "message_end" && r.currentMessageKey == key {
		r.currentMessageKey = ""
		r.currentMessageRole = ""
	}

	return true, nil
}

func (r *LiveTurnReducer) applyToolExecutionEvent(env EventEnvelope) (bool, error) {
	item := LiveTurnItem{Kind: LiveTurnToolExecution}
	var args json.RawMessage
	var result json.RawMessage

	switch env.EventType {
	case "tool_execution_start":
		var payload liveToolExecutionStartPayload
		if err := json.Unmarshal([]byte(env.PayloadJSON), &payload); err != nil {
			return false, err
		}
		item.ToolCallID = strings.TrimSpace(payload.ToolCallID)
		item.ToolName = strings.TrimSpace(payload.ToolName)
		item.Status = ToolExecutionPending
		args = payload.Args
	case "tool_execution_update":
		var payload liveToolExecutionUpdatePayload
		if err := json.Unmarshal([]byte(env.PayloadJSON), &payload); err != nil {
			return false, err
		}
		item.ToolCallID = strings.TrimSpace(payload.ToolCallID)
		item.ToolName = strings.TrimSpace(payload.ToolName)
		item.Status = ToolExecutionRunning
		args = payload.Args
		result = payload.PartialResult
	case "tool_execution_end":
		var payload liveToolExecutionEndPayload
		if err := json.Unmarshal([]byte(env.PayloadJSON), &payload); err != nil {
			return false, err
		}
		item.ToolCallID = strings.TrimSpace(payload.ToolCallID)
		item.ToolName = strings.TrimSpace(payload.ToolName)
		item.Status = ToolExecutionDone
		if payload.IsError {
			item.Status = ToolExecutionError
		}
		item.IsFinal = true
		result = payload.Result
	default:
		return false, nil
	}

	if item.ToolCallID == "" {
		return false, nil
	}

	if len(args) > 0 {
		item.MessageJSON = cloneRawMessage(args)
	}
	if len(result) > 0 {
		item.ResultJSON = cloneRawMessage(result)
	}

	r.upsertToolExecutionItem(toolExecutionKey(item.ToolCallID), item)
	return true, nil
}

func (r *LiveTurnReducer) appendItem(item LiveTurnItem) string {
	key := strings.TrimSpace(item.Key)
	if key == "" {
		key = r.nextMessageKey()
	}
	item.Key = key
	item.MessageJSON = cloneRawMessage(item.MessageJSON)
	item.ResultJSON = cloneRawMessage(item.ResultJSON)
	if r.indexByKey == nil {
		r.indexByKey = make(map[string]int)
	}
	r.indexByKey[key] = len(r.items)
	r.items = append(r.items, item)
	return key
}

func (r *LiveTurnReducer) upsertItem(key string, item LiveTurnItem) {
	if idx, ok := r.indexByKey[key]; ok {
		item.Key = key
		item.MessageJSON = cloneRawMessage(item.MessageJSON)
		item.ResultJSON = cloneRawMessage(item.ResultJSON)
		r.items[idx] = item
		return
	}
	r.appendItem(item)
}

func (r *LiveTurnReducer) upsertToolExecutionItem(key string, item LiveTurnItem) {
	if idx, ok := r.indexByKey[key]; ok {
		existing := r.items[idx]
		if item.ToolName == "" {
			item.ToolName = existing.ToolName
		}
		if len(item.MessageJSON) == 0 {
			item.MessageJSON = existing.MessageJSON
		}
		if len(item.ResultJSON) == 0 {
			item.ResultJSON = existing.ResultJSON
		}
		if item.Status == "" {
			item.Status = existing.Status
		}
		if item.ToolCallID == "" {
			item.ToolCallID = existing.ToolCallID
		}
		if !item.IsFinal {
			item.IsFinal = existing.IsFinal
		}
	}
	item.Key = key
	item.Kind = LiveTurnToolExecution
	r.upsertItem(key, item)
}

func (r *LiveTurnReducer) nextMessageKey() string {
	key := fmt.Sprintf("message-%03d", r.nextMessageIndex)
	r.nextMessageIndex++
	return key
}

func liveTurnMessageKind(role string) (LiveTurnItemKind, bool) {
	switch strings.TrimSpace(role) {
	case "user":
		return LiveTurnUserMessage, true
	case "assistant":
		return LiveTurnAssistantMessage, true
	case "toolResult":
		return LiveTurnToolResult, true
	default:
		return "", false
	}
}

func toolExecutionKey(toolCallID string) string {
	return "tool-" + strings.TrimSpace(toolCallID)
}

func cloneLiveTurnItem(item LiveTurnItem) LiveTurnItem {
	item.MessageJSON = cloneRawMessage(item.MessageJSON)
	item.ResultJSON = cloneRawMessage(item.ResultJSON)
	return item
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
