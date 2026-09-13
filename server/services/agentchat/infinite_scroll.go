package agentchat

import (
	"net/url"
	"strconv"
	"strings"
)

// Pattern A MorphMap (AgentChat).
const (
	agentChatScrollHostID        = "agent-chat-scroll-region"
	agentChatScrollItemsID       = "agent-chat-stable-transcript"
	agentChatScrollSentinelAbove = "agent-chat-scroll-sentinel-above"
	stableTranscriptInitialLimit = 50
)

// windowStableTranscript keeps the newest page of stable messages for first paint.
// Older pages load via InfiniteScroll above-edge (PatchAboveExpr → history endpoint).
func windowStableTranscript(messages []TranscriptMessage) (page []TranscriptMessage, hasMore bool, olderBefore string) {
	if len(messages) <= stableTranscriptInitialLimit {
		return messages, false, ""
	}
	start := len(messages) - stableTranscriptInitialLimit
	page = messages[start:]
	olderBefore = strings.TrimSpace(page[0].DOMID)
	if olderBefore == "" {
		return page, false, ""
	}
	return page, true, olderBefore
}

func applyStableTranscriptWindow(state TranscriptPaneState) TranscriptPaneState {
	page, hasMore, before := windowStableTranscript(state.Stable)
	state.Stable = page
	state.HasMoreOlder = hasMore
	state.OlderBefore = before
	return state
}

func agentChatHistoryPatchAboveExpr(threadID, olderBefore string) string {
	threadID = strings.TrimSpace(threadID)
	olderBefore = strings.TrimSpace(olderBefore)
	if threadID == "" || olderBefore == "" {
		return ""
	}
	return "@get('/agent-chat/thread/" + url.PathEscape(threadID) +
		"/history?before=" + url.QueryEscape(olderBefore) +
		"&limit=" + strconv.Itoa(stableTranscriptInitialLimit) + "')"
}
