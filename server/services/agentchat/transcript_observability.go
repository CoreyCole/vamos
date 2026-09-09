package agentchat

import (
	"encoding/json"
	"path"
	"strings"
	"unicode"

	"github.com/CoreyCole/vamos/pkg/agents/conversation"
)

const (
	inboundA2AAvatarBg  = "bg-sky-700"
	inboundA2ANameColor = "text-sky-300"
)

func headerLinkLabel(msg TranscriptMessage) string {
	if strings.TrimSpace(msg.HeaderCode) != "" {
		return msg.HeaderCode
	}
	return strings.TrimSpace(msg.HeaderHref)
}

func secondaryLinkLabel(msg TranscriptMessage) string {
	if strings.TrimSpace(msg.SecondaryLinkLabel) != "" {
		return msg.SecondaryLinkLabel
	}
	return strings.TrimSpace(msg.SecondaryHref)
}

func transcriptArtifactHref(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "://") || path.IsAbs(value) {
		return ""
	}
	cleaned := path.Clean(strings.ReplaceAll(value, "\\", "/"))
	cleaned = strings.TrimPrefix(cleaned, "./")
	if strings.HasPrefix(cleaned, "../") || cleaned == ".." || cleaned == "." {
		return ""
	}
	return cleaned
}

func roomCutHandoffHref(timestamp, handoffPath string) string {
	if href := transcriptArtifactHref(handoffPath); href != "" {
		return href
	}
	ts := strings.TrimSpace(timestamp)
	if ts == "" {
		return ""
	}
	return path.Join("handoffs", ts+".md")
}

func roomCutHistoryHref(timestamp, historyPath string) string {
	if href := transcriptArtifactHref(historyPath); href != "" {
		return href
	}
	ts := strings.TrimSpace(timestamp)
	if ts == "" {
		return ""
	}
	return path.Join("history", ts+".jsonl")
}

func slugAuthorInitial(slug string) string {
	for _, r := range strings.TrimSpace(slug) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "A"
}

func applySpeakerAttribution(items []TranscriptMessage, userEmail, fromAgentSlug string) {
	slug := strings.TrimSpace(fromAgentSlug)
	email := strings.TrimSpace(userEmail)
	for i := range items {
		if slug != "" {
			items[i].Role = "assistant"
			items[i].AuthorName = slug
			items[i].AuthorInitial = slugAuthorInitial(slug)
			items[i].AvatarBg = inboundA2AAvatarBg
			items[i].NameColor = inboundA2ANameColor
			continue
		}
		if items[i].Role == "user" && email != "" {
			items[i].AuthorName = email
		}
	}
}

func (s *Service) cutDetailTranscriptItems(
	domID, entryID, title, body, timestamp, handoffPath, historyPath string,
) []TranscriptMessage {
	msg := s.newDetailTranscriptMessage(domID, entryID, title, body, false, true)
	switch title {
	case "handoff":
		href := roomCutHandoffHref(timestamp, handoffPath)
		msg.HeaderHref = href
		msg.HeaderCode = href
		hist := roomCutHistoryHref(timestamp, historyPath)
		msg.SecondaryHref = hist
		msg.SecondaryLinkLabel = hist
	case "compaction":
		if href := transcriptArtifactHref(handoffPath); href != "" {
			msg.HeaderHref = href
			msg.HeaderCode = href
		}
	}
	msg.HideBodyWhenCollapsed = strings.TrimSpace(body) != ""
	if msg.HideBodyWhenCollapsed {
		msg.Collapsible = true
	}
	return []TranscriptMessage{msg}
}

func (s *Service) liveCutTranscriptItems(
	domID, entryID string,
	item conversation.LiveTurnItem,
) []TranscriptMessage {
	var payload struct {
		Timestamp   string `json:"timestamp"`
		HandoffPath string `json:"handoffPath"`
		HistoryPath string `json:"historyPath"`
		Content     any    `json:"content"`
		Body        string `json:"body"`
	}
	if len(item.MessageJSON) > 0 {
		_ = json.Unmarshal(item.MessageJSON, &payload)
	}
	body := strings.TrimSpace(payload.Body)
	if body == "" {
		body = strings.TrimSpace(extractContentText(payload.Content))
	}
	title := string(item.Kind)
	return s.cutDetailTranscriptItems(
		domID,
		entryID,
		title,
		body,
		payload.Timestamp,
		payload.HandoffPath,
		payload.HistoryPath,
	)
}
