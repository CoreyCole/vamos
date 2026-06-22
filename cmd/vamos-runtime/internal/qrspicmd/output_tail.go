package qrspicmd

import "strings"

func FilterChildOutputTail(data []byte, maxLines int) []string {
	if maxLines <= 0 {
		maxLines = 8
	}
	raw := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(raw))
	inUsage := false
	for _, line := range raw {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isUsageBlockLine(trimmed) {
			inUsage = true
			continue
		}
		if inUsage {
			if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, "Flags:") || strings.HasPrefix(trimmed, "Global Flags:") {
				continue
			}
			inUsage = false
		}
		lines = append(lines, trimmed)
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return lines
}

func isUsageBlockLine(line string) bool {
	line = strings.TrimSpace(line)
	return line == "Usage:" || strings.HasPrefix(line, "Usage: ") || strings.HasPrefix(line, "Flags:") || strings.HasPrefix(line, "Global Flags:")
}
