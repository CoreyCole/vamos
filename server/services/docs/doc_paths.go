package docs

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

type EntryMode string

const (
	EntryModeThoughts  EntryMode = "thoughts"
	EntryModeAgentChat EntryMode = "agent-chat"
)

type DocPath string

func ParseThoughtsDocPath(prefix, raw string) (DocPath, error) {
	value := strings.TrimSpace(raw)
	if decoded, err := url.PathUnescape(value); err == nil {
		value = decoded
	}
	value = strings.TrimPrefix(value, "/")
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix != "" {
		value = strings.TrimPrefix(value, prefix+"/")
	}
	value = strings.TrimPrefix(value, "thoughts/")
	if value == ".." || strings.HasPrefix(value, "../") ||
		strings.Contains(value, "/../") {
		return "", fmt.Errorf("doc path escapes thoughts root: %q", raw)
	}
	value = path.Clean("/" + value)
	value = strings.TrimPrefix(value, "/")
	if value == "." || value == "" {
		return "", fmt.Errorf("doc path is required")
	}
	if strings.HasPrefix(value, "../") || value == ".." {
		return "", fmt.Errorf("doc path escapes thoughts root: %q", raw)
	}
	return DocPath(value), nil
}

func ThoughtsDocHref(docPath DocPath) string {
	return "/thoughts/" + escapeDocPath(docPath)
}

func escapeDocPath(docPath DocPath) string {
	parts := strings.Split(strings.Trim(string(docPath), "/"), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
