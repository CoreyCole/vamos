package markdown

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/db"
)

type botHomePreview struct {
	Text string
	Time time.Time
}

func lastBotHomePreview(
	ctx context.Context,
	q db.Querier,
	thoughtsRoot, slug string,
) botHomePreview {
	path := filepath.Join(thoughtsRoot, "agents", slug, "sessions", "current.jsonl")
	if q != nil {
		rows, err := q.ListAgentThreadsByAgentSlug(ctx, sql.NullString{
			String: strings.TrimSpace(slug),
			Valid:  strings.TrimSpace(slug) != "",
		})
		if err == nil && len(rows) > 0 {
			row := rows[0]
			path = resolveScopedJSONLPath(path, row.PiSessionID)
			if strings.TrimSpace(row.PiSessionID) == "" &&
				strings.Contains(filepath.ToSlash(path), "/pi/") {
				_ = q.SetAgentThreadPiSessionID(ctx, db.SetAgentThreadPiSessionIDParams{
					PiSessionID: piSessionIDFromJSONLPath(path),
					ID:          row.ID,
				})
			}
		}
	}
	return lastJSONLPreview(path)
}

func lastJSONLPreview(path string) botHomePreview {
	info, err := os.Stat(path)
	if err != nil {
		return botHomePreview{}
	}
	out := botHomePreview{Time: info.ModTime()}
	file, err := os.Open(path)
	if err != nil {
		return out
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var envelope struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
			Message   struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		if envelope.Type != "message" {
			continue
		}
		switch strings.TrimSpace(envelope.Message.Role) {
		case "user", "assistant":
		default:
			continue
		}
		text, ok := envelope.Message.Content.(string)
		if !ok {
			continue
		}
		text = strings.Join(strings.Fields(text), " ")
		if text == "" {
			continue
		}
		out.Text = text
		if ts := parseRosterJSONLTime(envelope.Timestamp); !ts.IsZero() {
			out.Time = ts
		}
	}
	return out
}

func parseRosterJSONLTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts
		}
	}
	return time.Time{}
}
