package chatcmd

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
)

type ChatNDJSONEvent struct {
	Type             string          `json:"type"`
	Seq              int64           `json:"seq,omitempty"`
	EventType        string          `json:"event_type,omitempty"`
	Status           string          `json:"status,omitempty"`
	Ref              ChatRunRef      `json:"ref,omitempty"`
	Response         string          `json:"response,omitempty"`
	Error            string          `json:"error,omitempty"`
	Reason           string          `json:"reason,omitempty"`
	QRSPIResult      json.RawMessage `json:"qrspi_result,omitempty"`
	LatestThreadID   string          `json:"latest_thread_id,omitempty"`
	LatestWebURL     string          `json:"latest_web_url,omitempty"`
	InfluencesLatest bool            `json:"influences_latest,omitempty"`
}

func WriteNDJSON(w io.Writer, event ChatNDJSONEvent) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(event)
}

func ndjsonFromChatEvent(event chatsession.ChatEvent, ref ChatRunRef) ChatNDJSONEvent {
	compactRef := ChatRunRef{RunID: firstNonBlank(event.RunID, ref.RunID), ThreadID: ref.ThreadID, ChatSessionID: ref.ChatSessionID, WebURL: ref.WebURL}
	return ChatNDJSONEvent{Type: "event", Seq: event.Seq, EventType: string(event.EventType), Ref: compactRef}
}

func terminalNDJSON(status string, ref ChatRunRef, snapshot snapshotResponse, runID, errText string) ChatNDJSONEvent {
	ref.RunID = firstNonBlank(runID, ref.RunID)
	return ChatNDJSONEvent{
		Type:     "result",
		Status:   status,
		Ref:      ref,
		Response: finalAssistantResponse(snapshot.Projection, ref.RunID),
		Error:    strings.TrimSpace(errText),
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
