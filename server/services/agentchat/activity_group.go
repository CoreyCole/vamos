package agentchat

import (
	"fmt"
	"strings"
	"unicode"
)

type transcriptActivitySegment struct {
	Grouped  bool
	Messages []TranscriptMessage
}

func groupTranscriptActivity(messages []TranscriptMessage) []transcriptActivitySegment {
	out := make([]transcriptActivitySegment, 0, len(messages))
	var run []TranscriptMessage
	flushRun := func() {
		if len(run) == 0 {
			return
		}
		out = append(out, transcriptActivitySegment{
			Grouped:  len(run) >= 2,
			Messages: run,
		})
		run = nil
	}
	for _, msg := range messages {
		if strings.TrimSpace(msg.Variant) == "detail" {
			run = append(run, msg)
			continue
		}
		flushRun()
		out = append(out, transcriptActivitySegment{
			Messages: []TranscriptMessage{msg},
		})
	}
	flushRun()
	return out
}

func activityGroupID(messages []TranscriptMessage) string {
	if len(messages) == 0 {
		return "activity"
	}
	id := strings.TrimSpace(messages[0].DOMID)
	if id == "" {
		id = "activity"
	}
	return id
}

func activityGroupSummary(messages []TranscriptMessage) string {
	type kindCount struct {
		kind  string
		count int
	}
	order := make([]kindCount, 0, len(messages))
	index := map[string]int{}
	for _, msg := range messages {
		kind := activityGroupKindLabel(msg)
		if i, ok := index[kind]; ok {
			order[i].count++
			continue
		}
		index[kind] = len(order)
		order = append(order, kindCount{kind: kind, count: 1})
	}
	parts := make([]string, 0, len(order))
	for _, item := range order {
		if item.count > 1 {
			parts = append(parts, fmt.Sprintf("%s×%d", item.kind, item.count))
			continue
		}
		parts = append(parts, item.kind)
	}
	return strings.Join(parts, " · ")
}

func activityGroupKindLabel(msg TranscriptMessage) string {
	title := strings.TrimSpace(msg.Title)
	if title == "" {
		kind := chatDensityKind(msg)
		if kind == "" {
			return "DETAIL"
		}
		return strings.ToUpper(kind)
	}
	var b strings.Builder
	for _, r := range title {
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	if b.Len() == 0 {
		return "DETAIL"
	}
	return b.String()
}
