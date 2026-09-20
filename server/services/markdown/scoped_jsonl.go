package markdown

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func jsonlIsScopedListRow(path string) bool {
	info := inspectScopedJSONL(path)
	return !info.unreadable && info.hasUserMessage
}

func resolveScopedJSONLPath(current, piID string) string {
	piID = strings.TrimSpace(piID)
	if piID != "" {
		return filepath.Join(filepath.Dir(current), "pi", piID+".jsonl")
	}
	if dest, _, err := migrateLegacyCurrentJSONLAtPath(
		current,
	); err == nil &&
		dest != "" {
		return dest
	}
	return current
}

func piSessionIDFromJSONLPath(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}

type scopedJSONLInspect struct {
	hasSessionHeader bool
	sessionID        string
	hasUserMessage   bool
	unreadable       bool
}

func inspectScopedJSONL(path string) scopedJSONLInspect {
	path = strings.TrimSpace(path)
	if path == "" {
		return scopedJSONLInspect{unreadable: true}
	}
	file, err := os.Open(path)
	if err != nil {
		return scopedJSONLInspect{unreadable: true}
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	out := scopedJSONLInspect{}
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var envelope struct {
			Type    string `json:"type"`
			ID      string `json:"id"`
			Message struct {
				Role string `json:"role"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			if first {
				out.unreadable = true
				return out
			}
			first = false
			continue
		}
		if first {
			first = false
			if envelope.Type == "session" && strings.TrimSpace(envelope.ID) != "" {
				out.hasSessionHeader = true
				out.sessionID = strings.TrimSpace(envelope.ID)
			} else {
				return out
			}
			continue
		}
		if envelope.Type == "message" &&
			strings.EqualFold(strings.TrimSpace(envelope.Message.Role), "user") {
			out.hasUserMessage = true
		}
	}
	if err := scanner.Err(); err != nil {
		out.unreadable = true
	}
	return out
}

func migrateLegacyCurrentJSONLAtPath(currentAbs string) (string, bool, error) {
	info := inspectScopedJSONL(currentAbs)
	if info.unreadable || !info.hasSessionHeader || !info.hasUserMessage {
		if dest := soleMigratedPiJSONL(filepath.Dir(currentAbs)); dest != "" {
			return dest, false, nil
		}
		return "", false, nil
	}
	dest := filepath.Join(filepath.Dir(currentAbs), "pi", info.sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", false, err
	}
	if destInfo, err := os.Stat(dest); err == nil && destInfo.Size() > 0 {
		return dest, false, nil
	}
	if err := os.Rename(currentAbs, dest); err != nil {
		return "", false, err
	}
	return dest, true, nil
}

func soleMigratedPiJSONL(sessionsDir string) string {
	matches, err := filepath.Glob(filepath.Join(sessionsDir, "pi", "*.jsonl"))
	if err != nil {
		return ""
	}
	var rows []string
	for _, path := range matches {
		if jsonlIsScopedListRow(path) {
			rows = append(rows, path)
		}
	}
	if len(rows) != 1 {
		return ""
	}
	return rows[0]
}
