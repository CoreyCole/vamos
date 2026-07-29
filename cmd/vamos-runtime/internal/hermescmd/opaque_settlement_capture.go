package hermescmd

// OpaqueSettlementFence is copied lexical evidence. Its content is never
// decoded or interpreted by Hermes.
type OpaqueSettlementFence struct {
	Language string
	Raw      string
}

// OpaqueSettlementEvidence is the neutral input to the future envelope
// serializer. It has no lifecycle or routing fields.
type OpaqueSettlementEvidence struct {
	RawResponse      string
	FencedYAMLBlocks []OpaqueSettlementFence
}

// CaptureOpaqueSettlementEvidence prepares raw response evidence without
// parsing the fence contents.
func CaptureOpaqueSettlementEvidence(raw string) OpaqueSettlementEvidence {
	return OpaqueSettlementEvidence{
		RawResponse:      raw,
		FencedYAMLBlocks: CaptureOpaqueSettlementFences(raw),
	}
}

// CaptureOpaqueSettlementFences finds YAML/YML fences using the settlement
// byte grammar. It preserves every captured byte, including CRLF terminators.
func CaptureOpaqueSettlementFences(raw string) []OpaqueSettlementFence {
	var fences []OpaqueSettlementFence
	for lineStart := 0; lineStart < len(raw); {
		openerEnd := settlementLineEnd(raw, lineStart)
		runLength, language, ok := settlementFenceOpener(raw, lineStart, openerEnd)
		if !ok {
			lineStart = openerEnd
			continue
		}

		matched := false
		for closerStart := openerEnd; closerStart < len(raw); {
			closerEnd := settlementLineEnd(raw, closerStart)
			if settlementFenceCloser(raw, closerStart, closerEnd, runLength) {
				fences = append(fences, OpaqueSettlementFence{
					Language: language,
					Raw:      raw[lineStart:closerEnd],
				})
				lineStart = closerEnd
				matched = true
				break
			}
			closerStart = closerEnd
		}
		if !matched {
			lineStart = openerEnd
		}
	}
	return fences
}

func settlementLineEnd(raw string, start int) int {
	for i := start; i < len(raw); i++ {
		if raw[i] == '\n' {
			return i + 1
		}
	}
	return len(raw)
}

func settlementFenceOpener(raw string, start, end int) (int, string, bool) {
	cursor := start
	for cursor < len(raw) && raw[cursor] == '`' {
		cursor++
	}
	runLength := cursor - start
	if runLength < 3 {
		return 0, "", false
	}
	for cursor < len(raw) && settlementSpaceTab(raw[cursor]) {
		cursor++
	}
	languageStart := cursor
	for cursor < len(raw) && settlementASCIIAlpha(raw[cursor]) {
		cursor++
	}
	language := raw[languageStart:cursor]
	if !settlementYAMLLanguage(language) {
		return 0, "", false
	}
	for cursor < len(raw) && (settlementSpaceTab(raw[cursor]) || raw[cursor] == '\r') {
		cursor++
	}
	if cursor != end && raw[cursor] != '\n' {
		return 0, "", false
	}
	return runLength, language, true
}

func settlementFenceCloser(raw string, start, end, runLength int) bool {
	cursor := start
	for cursor < len(raw) && raw[cursor] == '`' {
		cursor++
	}
	if cursor-start != runLength {
		return false
	}
	for cursor < len(raw) && (settlementSpaceTab(raw[cursor]) || raw[cursor] == '\r') {
		cursor++
	}
	return cursor == end || raw[cursor] == '\n'
}

func settlementSpaceTab(b byte) bool { return b == ' ' || b == '\t' }

func settlementASCIIAlpha(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func settlementYAMLLanguage(language string) bool {
	if len(language) == 3 {
		return settlementASCIIEqualFold(language, "yml")
	}
	return len(language) == 4 && settlementASCIIEqualFold(language, "yaml")
}

func settlementASCIIEqualFold(value, want string) bool {
	if len(value) != len(want) {
		return false
	}
	for i := range value {
		b := value[i]
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}
		if b != want[i] {
			return false
		}
	}
	return true
}
