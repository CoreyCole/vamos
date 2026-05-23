package workspace

import "strings"

type PatchScope string

const (
	PatchResource       PatchScope = "workspace-resource"
	PatchLiveTranscript PatchScope = "live-transcript"
)

type StreamSignal struct {
	Cursor int64
	Scope  PatchScope
}

func ScopeForPersistedEvent(eventType string) PatchScope {
	return PatchResource
}

func NeedsCatchup(since, currentCursor int64) bool {
	return since != currentCursor
}

func SidebarVisibleByDefault(hasSelectedThread bool) bool {
	return !hasSelectedThread
}

func IsLiveConversationEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "message_start",
		"message_update",
		"message_end",
		"tool_execution_start",
		"tool_execution_update",
		"tool_execution_end":
		return true
	default:
		return false
	}
}
