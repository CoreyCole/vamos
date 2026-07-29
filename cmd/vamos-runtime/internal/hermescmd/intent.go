package hermescmd

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// PiIntent is the child-supplied, presentation-only portion of a managed
// completion. It deliberately has no graph state or transition behavior.
type PiIntent struct {
	Outcome        Outcome
	Next           NextAction
	Recommendation string
	Summary        string
	Artifacts      []string
	RawResponse    string
	RawYAML        string
	Metadata       map[string]any
	Diagnostics    []string
}

var (
	yamlFence         = regexp.MustCompile("(?s)```(?:yaml|yml)\\s*\\n(.*?)```")
	lifecycleYAMLLine = regexp.MustCompile(`^(?:state|outcome|status|result|decision):`)
	mappingYAMLLine   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*:`)
)

// ExtractPiIntent tolerantly decodes compact YAML embedded in a child's final
// response. Exactly one valid lifecycle declaration is required; all other
// cases deliberately become a blocked, graph-agnostic checkpoint intent.
func ExtractPiIntent(response string) PiIntent {
	intent := PiIntent{Outcome: OutcomeBlocked, Next: NextNone, RawResponse: response}
	mappings, rawYAML, malformed := intentMappings(response)
	intent.RawYAML = rawYAML

	var lifecycle []string
	var diagnostics []string
	if malformed {
		diagnostics = append(diagnostics, "malformed lifecycle YAML")
	}
	for _, mapping := range mappings {
		values, bad := lifecycleValues(mapping)
		if bad {
			diagnostics = append(diagnostics, "malformed lifecycle value")
		}
		lifecycle = append(lifecycle, values...)
		if intent.Recommendation == "" {
			intent.Recommendation = recommendation(mapping)
		}
		if intent.Summary == "" {
			intent.Summary = stringValue(mapping, "summary", "explanation")
		}
		if len(intent.Artifacts) == 0 {
			intent.Artifacts = artifactValues(mapping)
		}
		if intent.Metadata == nil {
			intent.Metadata = unknownMetadata(mapping)
		}
	}

	if len(lifecycle) != 1 || len(diagnostics) != 0 {
		if len(lifecycle) == 0 && len(diagnostics) == 0 {
			diagnostics = append(diagnostics, "missing lifecycle value")
		} else if len(lifecycle) > 1 {
			diagnostics = append(diagnostics, "duplicate or conflicting lifecycle values")
		}
		intent.Diagnostics = diagnostics
		return intent
	}

	outcome, err := ParseOutcome(lifecycle[0])
	if err != nil {
		intent.Diagnostics = []string{
			fmt.Sprintf("unknown lifecycle value %q", lifecycle[0]),
		}
		return intent
	}
	intent.Outcome = outcome
	return intent
}

// ApplyPiIntent copies child-owned presentation data into a checkpoint while
// keeping the managed successor permanently graph-agnostic.
func ApplyPiIntent(checkpoint *PiCheckpoint, intent PiIntent) {
	checkpoint.Outcome = intent.Outcome
	checkpoint.Next = NextNone
	checkpoint.Recommendation = intent.Recommendation
	checkpoint.Summary = intent.Summary
	checkpoint.Artifacts = intent.Artifacts
	checkpoint.RawResponse = intent.RawResponse
	checkpoint.RawYAML = intent.RawYAML
	checkpoint.IntentMetadata = intent.Metadata
	checkpoint.Diagnostics = intent.Diagnostics
}

func intentMappings(response string) ([]map[string]any, string, bool) {
	matches := yamlFence.FindAllStringSubmatch(response, -1)
	var sources []string
	if len(matches) > 0 {
		for _, match := range matches {
			sources = append(sources, strings.TrimSpace(match[1]))
		}
	} else {
		// Accept a compact mapping embedded in prose without treating prose as a
		// lifecycle declaration. A lifecycle key must begin its own YAML line.
		sources = unfencedIntentMappings(response)
		if len(sources) == 0 {
			sources = []string{strings.TrimSpace(response)}
		}
	}

	var mappings []map[string]any
	var raw []string
	malformed := false
	for _, source := range sources {
		if source == "" {
			continue
		}
		// Keep the supplied compact YAML even when it is malformed: diagnostics
		// explain why it could not become a lifecycle intent.
		if len(matches) > 0 || hasLifecycleToken(source) {
			raw = append(raw, source)
		}
		var decoded any
		if err := yaml.Unmarshal([]byte(source), &decoded); err != nil {
			if hasLifecycleToken(source) {
				malformed = true
			}
			continue
		}
		mapping, ok := normalizeMap(decoded)
		if !ok {
			continue
		}
		if hasLifecycleMapping(mapping) {
			mappings = append(mappings, mapping)
		}
	}
	return mappings, strings.Join(raw, "\n---\n"), malformed
}

func unfencedIntentMappings(response string) []string {
	lines := strings.Split(response, "\n")
	var sources []string
	for start := 0; start < len(lines); start++ {
		line := strings.TrimSpace(lines[start])
		if !lifecycleYAMLLine.MatchString(line) {
			continue
		}
		end := start + 1
		for end < len(lines) {
			next := lines[end]
			trimmed := strings.TrimSpace(next)
			if trimmed == "" || strings.HasPrefix(next, " ") || strings.HasPrefix(next, "\t") ||
				strings.HasPrefix(trimmed, "- ") ||
				mappingYAMLLine.MatchString(trimmed) {
				end++
				continue
			}
			break
		}
		sources = append(sources, strings.Join(lines[start:end], "\n"))
		start = end - 1
	}
	return sources
}

func hasLifecycleToken(value string) bool {
	value = strings.ToLower(value)
	for _, key := range []string{"state:", "outcome:", "status:", "result:", "decision:"} {
		if strings.Contains(value, key) {
			return true
		}
	}
	return false
}

func hasLifecycleMapping(mapping map[string]any) bool {
	for _, key := range []string{"state", "outcome", "status", "result", "decision"} {
		value, ok := mapping[key]
		if !ok {
			continue
		}
		if key == "result" {
			if nested, ok := normalizeMap(value); ok {
				return hasLifecycleMapping(nested)
			}
		}
		return true
	}
	return false
}

func lifecycleValues(mapping map[string]any) ([]string, bool) {
	var values []string
	bad := false
	for _, key := range []string{"state", "outcome", "status", "result", "decision"} {
		value, ok := mapping[key]
		if !ok {
			continue
		}
		if key == "result" {
			if nested, ok := normalizeMap(value); ok {
				nestedValues, nestedBad := lifecycleValues(nested)
				values, bad = append(values, nestedValues...), bad || nestedBad
				continue
			}
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			bad = true
			continue
		}
		values = append(values, strings.TrimSpace(text))
	}
	return values, bad
}

func lifecycleValue(mapping map[string]any) (string, bool, bool) {
	values, bad := lifecycleValues(mapping)
	if len(values) != 1 {
		return "", bad, false
	}
	return values[0], bad, true
}

func recommendation(mapping map[string]any) string {
	for _, key := range []string{"recommendation", "next"} {
		value, ok := mapping[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		if next, err := ParseNextAction(strings.TrimSpace(text)); err == nil {
			return string(next)
		}
	}
	return ""
}

func stringValue(mapping map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := mapping[key].(string); ok {
			return value
		}
	}
	return ""
}

func artifactValues(mapping map[string]any) []string {
	for _, key := range []string{"artifacts", "artifact"} {
		value, ok := mapping[key]
		if !ok {
			continue
		}
		switch value := value.(type) {
		case string:
			return []string{value}
		case []any:
			artifacts := make([]string, 0, len(value))
			for _, item := range value {
				if text, ok := item.(string); ok {
					artifacts = append(artifacts, text)
				}
			}
			return artifacts
		}
	}
	return nil
}

func unknownMetadata(mapping map[string]any) map[string]any {
	metadata := make(map[string]any)
	for key, value := range mapping {
		switch key {
		case "state",
			"outcome",
			"status",
			"result",
			"decision",
			"recommendation",
			"next",
			"summary",
			"explanation",
			"artifact",
			"artifacts":
			continue
		default:
			metadata[key] = value
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func normalizeMap(value any) (map[string]any, bool) {
	mapping, ok := value.(map[string]any)
	return mapping, ok
}
