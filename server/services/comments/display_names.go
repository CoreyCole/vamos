package comments

import (
	"strings"
	"unicode"
)

func normalizeEmailDomains(domains []string) []string {
	out := make([]string, 0, len(domains))
	seen := make(map[string]bool, len(domains))
	for _, domain := range domains {
		d := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), "@")
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

func (s *Service) commentDisplayName(email string) string {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" {
		return "Unknown"
	}
	local, domain, ok := strings.Cut(trimmed, "@")
	if !ok || local == "" {
		return trimmed
	}
	domain = strings.ToLower(strings.TrimSpace(domain))
	for _, allowed := range s.allowedDomains {
		if domain == allowed {
			return titleCaseEmailLocalPart(local)
		}
	}
	return trimmed
}

func titleCaseEmailLocalPart(local string) string {
	parts := strings.FieldsFunc(local, func(r rune) bool {
		switch r {
		case '.', '_', '-', '+':
			return true
		default:
			return unicode.IsSpace(r)
		}
	})
	if len(parts) == 0 {
		return local
	}
	for i, part := range parts {
		parts[i] = titleWord(part)
	}
	return strings.Join(parts, " ")
}

func titleWord(word string) string {
	runes := []rune(strings.ToLower(word))
	if len(runes) == 0 {
		return ""
	}
	runes[0] = unicode.ToTitle(runes[0])
	return string(runes)
}
