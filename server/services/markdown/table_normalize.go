package markdown

import (
	"bytes"
	"strings"
	"unicode"
)

func normalizeMarkdownTables(md []byte) []byte {
	if !bytes.Contains(md, []byte("|")) {
		return md
	}
	text := strings.ReplaceAll(string(md), "\r\n", "\n")
	const nl = "\n"
	lines := strings.Split(text, "\n")
	var out strings.Builder
	out.Grow(len(text))
	fence := ""
	for i := 0; i < len(lines); i++ {
		if i > 0 {
			out.WriteString(nl)
		}
		line := lines[i]
		if marker, ok := fenceOpen(line); ok {
			if fence == "" {
				fence = marker
			} else if strings.HasPrefix(marker, fence[:1]) && len(marker) >= len(fence) {
				fence = ""
			}
			out.WriteString(line)
			continue
		}
		if fence != "" {
			out.WriteString(line)
			continue
		}
		header, headerOK := splitPipeRow(line)
		if !headerOK || isDelimiterRow(header) || i+1 >= len(lines) {
			out.WriteString(line)
			continue
		}
		delim, delimOK := splitPipeRow(lines[i+1])
		if !delimOK || !isDelimiterRow(delim) {
			out.WriteString(line)
			continue
		}
		body := make([][]string, 0)
		end := i + 2
		for end < len(lines) {
			row, ok := splitPipeRow(lines[end])
			if !ok {
				break
			}
			body = append(body, row)
			end++
		}
		indent := leadingIndent(line)
		width := markdownTableWidth(header, body)
		writeNormalizedTable(
			&out,
			nl,
			indent,
			padCells(header, width),
			padDelimiter(delim, width),
			padRows(body, width),
		)
		i = end - 1
	}
	return []byte(out.String())
}

func writeNormalizedTable(
	out *strings.Builder,
	nl, indent string,
	header, delim []string,
	body [][]string,
) {
	out.WriteString(indent)
	out.WriteString(formatPipeRow(header))
	out.WriteString(nl)
	out.WriteString(indent)
	out.WriteString(formatPipeRow(delim))
	for _, row := range body {
		out.WriteString(nl)
		out.WriteString(indent)
		out.WriteString(formatPipeRow(row))
	}
}

func markdownTableWidth(header []string, rows [][]string) int {
	width := len(header)
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}
	return width
}

func padCells(cells []string, width int) []string {
	if len(cells) >= width {
		return cells[:width]
	}
	out := make([]string, width)
	copy(out, cells)
	return out
}

func padDelimiter(cells []string, width int) []string {
	out := padCells(cells, width)
	for i, cell := range out {
		if cell == "" {
			out[i] = "---"
		}
	}
	return out
}

func padRows(rows [][]string, width int) [][]string {
	out := make([][]string, len(rows))
	for i, row := range rows {
		out[i] = padCells(row, width)
	}
	return out
}

func formatPipeRow(cells []string) string {
	return "| " + strings.Join(cells, " | ") + " |"
}

func leadingIndent(line string) string {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return line[:i]
}

func fenceOpen(line string) (string, bool) {
	trimmed := strings.TrimLeftFunc(line, func(r rune) bool {
		return r == ' ' || r == '\t'
	})
	indent := len(line) - len(trimmed)
	if indent > 3 {
		return "", false
	}
	if len(trimmed) < 3 {
		return "", false
	}
	marker := trimmed[0]
	if marker != '`' && marker != '~' {
		return "", false
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == marker {
		n++
	}
	if n < 3 {
		return "", false
	}
	return trimmed[:n], true
}

func splitPipeRow(line string) ([]string, bool) {
	trimmedRight := strings.TrimRightFunc(line, unicode.IsSpace)
	if !strings.Contains(trimmedRight, "|") {
		return nil, false
	}
	parts := splitPipes(trimmedRight)
	if len(parts) == 0 {
		return nil, false
	}
	left := strings.TrimLeftFunc(trimmedRight, unicode.IsSpace)
	if strings.HasPrefix(left, "|") && parts[0] == "" {
		parts = parts[1:]
	}
	if strings.HasSuffix(trimmedRight, "|") && len(parts) > 0 &&
		parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 {
		return nil, false
	}
	cells := make([]string, len(parts))
	for i, part := range parts {
		cells[i] = strings.TrimSpace(part)
	}
	return cells, true
}

func splitPipes(line string) []string {
	parts := make([]string, 0, 8)
	var b strings.Builder
	escaped := false
	inCode := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if escaped {
			b.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' {
			b.WriteByte(c)
			escaped = true
			continue
		}
		if c == '`' {
			inCode = !inCode
			b.WriteByte(c)
			continue
		}
		if c == '|' && !inCode {
			parts = append(parts, b.String())
			b.Reset()
			continue
		}
		b.WriteByte(c)
	}
	parts = append(parts, b.String())
	return parts
}

func isDelimiterRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if !isDelimiterCell(cell) {
			return false
		}
	}
	return true
}

func isDelimiterCell(cell string) bool {
	if cell == "" {
		return false
	}
	i := 0
	if cell[0] == ':' {
		i++
	}
	dashes := 0
	for i < len(cell) && cell[i] == '-' {
		i++
		dashes++
	}
	if dashes == 0 {
		return false
	}
	if i < len(cell) && cell[i] == ':' {
		i++
	}
	return i == len(cell)
}
