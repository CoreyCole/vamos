package agentchat

import (
	"strings"
	"unicode/utf8"
)

const frontmatterValueMaxRunes = 48

type messageFrontmatterField struct {
	Key   string
	Value string
}

func formatMessageFrontmatter(fields []messageFrontmatterField, body string) string {
	var b strings.Builder
	wrote := 0
	for _, field := range fields {
		key := strings.TrimSpace(field.Key)
		value := strings.TrimSpace(field.Value)
		if key == "" || value == "" {
			continue
		}
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(value)
		b.WriteByte('\n')
		wrote++
	}
	if wrote == 0 {
		return body
	}
	b.WriteString("---\n")
	b.WriteString(body)
	return b.String()
}

func parseMessageFrontmatter(
	content string,
) (fields []messageFrontmatterField, body string, ok bool) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 {
		return nil, content, false
	}
	start := 0
	if strings.TrimSpace(lines[0]) == "---" {
		start = 1
	}
	parsed := make([]messageFrontmatterField, 0, 4)
	closedAt := -1
	for i := start; i < len(lines); i++ {
		trim := strings.TrimSpace(lines[i])
		if trim == "---" {
			closedAt = i
			break
		}
		if trim == "" {
			return nil, content, false
		}
		key, value, parsedOK := parseFrontmatterLine(lines[i])
		if !parsedOK {
			return nil, content, false
		}
		parsed = append(parsed, messageFrontmatterField{Key: key, Value: value})
	}
	if closedAt < 0 || len(parsed) == 0 {
		return nil, content, false
	}
	body = strings.TrimPrefix(strings.Join(lines[closedAt+1:], "\n"), "\n")
	return parsed, body, true
}

func parseFrontmatterLine(line string) (key, value string, ok bool) {
	line = strings.TrimRight(line, " \t")
	colon := strings.Index(line, ":")
	if colon <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:colon])
	if key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false
	}
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '_' ||
			r == '-' {
			continue
		}
		return "", "", false
	}
	value = strings.TrimSpace(line[colon+1:])
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return key, value, true
}

func truncateFrontmatterValue(value string) string {
	if utf8.RuneCountInString(value) <= frontmatterValueMaxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:frontmatterValueMaxRunes-1]) + "…"
}

func applyMessageFrontmatter(args ChatMessageArgs) ChatMessageArgs {
	fields, body, ok := parseMessageFrontmatter(args.Content)
	if !ok {
		return args
	}
	args.Frontmatter = fields
	args.Content = body
	if args.Role == "user" {
		args.HTMLContent = ""
	}
	return args
}
