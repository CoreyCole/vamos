package story

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func ParseFile(path string, opts ParseOptions) (Feature, error) {
	f, err := os.Open(path)
	if err != nil {
		return Feature{}, err
	}
	defer f.Close()

	feature := Feature{
		SourcePath: path,
		Slug:       slugify(strings.TrimSuffix(filepath.Base(path), ".story.md")),
	}
	var currentSection string
	var currentScenario *Scenario
	var currentProperty *Property
	var currentDimension string
	scanner := bufio.NewScanner(f)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "# Feature:"):
			feature.Title = strings.TrimSpace(strings.TrimPrefix(line, "# Feature:"))
		case strings.HasPrefix(line, "## User story"):
			currentSection = "user-story"
		case strings.HasPrefix(line, "## Business rules"):
			currentSection = "business-rules"
		case strings.HasPrefix(line, "## Properties"):
			if currentScenario != nil {
				feature.Scenarios = append(feature.Scenarios, *currentScenario)
				currentScenario = nil
			}
			currentSection = "properties"
		case strings.HasPrefix(line, "## Scenario:"):
			if currentProperty != nil {
				feature.Properties = append(feature.Properties, *currentProperty)
				currentProperty = nil
			}
			if currentScenario != nil {
				feature.Scenarios = append(feature.Scenarios, *currentScenario)
			}
			title := strings.TrimSpace(strings.TrimPrefix(line, "## Scenario:"))
			currentScenario = &Scenario{Title: title, Slug: slugify(title)}
			currentSection = "scenario"
		case strings.HasPrefix(line, "### ") && (currentSection == "properties" || currentSection == "property" || currentSection == "property-dimension" || currentSection == "property-then"):
			if currentProperty != nil {
				feature.Properties = append(feature.Properties, *currentProperty)
			}
			title := strings.TrimSpace(strings.TrimPrefix(line, "### "))
			currentProperty = &Property{Title: title, Slug: slugify(title)}
			currentDimension = ""
			currentSection = "property"
		case strings.HasPrefix(line, "### Given"):
			currentSection = "given"
		case strings.HasPrefix(line, "### When"):
			currentSection = "when"
		case strings.HasPrefix(line, "### Then"):
			currentSection = "then"
		case currentProperty != nil && line == "Then:":
			currentSection = "property-then"
			currentDimension = ""
		case currentProperty != nil && strings.HasPrefix(line, "For each ") && strings.HasSuffix(line, ":"):
			dimension := strings.TrimSuffix(strings.TrimPrefix(line, "For each "), ":")
			if dimension != "viewport" && dimension != "route" {
				return Feature{}, fmt.Errorf(
					"line %d: unsupported property dimension %q",
					lineNo,
					dimension,
				)
			}
			currentProperty.Dimensions = append(
				currentProperty.Dimensions,
				Dimension{Name: dimension},
			)
			currentDimension = dimension
			currentSection = "property-dimension"
		case strings.HasPrefix(line, "- "):
			text := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			switch {
			case currentProperty != nil && currentSection == "property-dimension":
				idx := len(currentProperty.Dimensions) - 1
				if idx < 0 || currentProperty.Dimensions[idx].Name != currentDimension {
					return Feature{}, fmt.Errorf(
						"line %d: property value without dimension",
						lineNo,
					)
				}
				currentProperty.Dimensions[idx].Values = append(
					currentProperty.Dimensions[idx].Values,
					unquoteValue(text),
				)
			case currentProperty != nil && currentSection == "property-then":
				step, err := parseStep("then", text, lineNo)
				if err != nil {
					return Feature{}, err
				}
				currentProperty.Then = append(currentProperty.Then, step)
			case currentScenario != nil && (currentSection == "given" || currentSection == "when" || currentSection == "then"):
				step, err := parseStep(currentSection, text, lineNo)
				if err != nil {
					return Feature{}, err
				}
				addStep(currentScenario, currentSection, step)
			case currentSection == "business-rules":
				feature.BusinessRules = append(feature.BusinessRules, text)
			}
		default:
			if currentSection == "user-story" && line != "" {
				feature.UserStory += line + "\n"
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Feature{}, err
	}
	if currentProperty != nil {
		feature.Properties = append(feature.Properties, *currentProperty)
	}
	if currentScenario != nil {
		feature.Scenarios = append(feature.Scenarios, *currentScenario)
	}
	if feature.Title == "" {
		return Feature{}, fmt.Errorf("%s: missing # Feature heading", path)
	}
	return feature, nil
}

func ParseDir(dir string, opts ParseOptions) ([]Feature, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.story.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	features := make([]Feature, 0, len(matches))
	for _, path := range matches {
		feature, err := ParseFile(path, opts)
		if err != nil {
			return nil, err
		}
		features = append(features, feature)
	}
	return features, nil
}

func addStep(s *Scenario, section string, step Step) {
	switch section {
	case "given":
		s.Given = append(s.Given, step)
	case "when":
		s.When = append(s.When, step)
	case "then":
		s.Then = append(s.Then, step)
	}
}

var quoted = regexp.MustCompile(`"([^"]+)"`)

func parseStep(section, text string, line int) (Step, error) {
	step := Step{
		Kind:   StepKind(section),
		Args:   map[string]string{},
		Source: SourceRange{Line: line},
	}
	switch {
	case strings.HasPrefix(text, "I am authenticated as "):
		step.Verb = "authenticated_as"
		step.Args["email"] = firstQuote(text)
	case strings.HasPrefix(text, "Fixture ") && strings.HasSuffix(text, " is loaded."):
		step.Verb = "load_fixture"
		step.Args["name"] = firstQuote(text)
	case strings.HasPrefix(text, "I visit "):
		step.Verb = "visit"
		step.Args["path"] = firstQuote(text)
	case strings.HasPrefix(text, "I open plan workspace "):
		step.Verb = "open_plan_workspace"
		step.Args["plan_dir"] = firstQuote(text)
	case strings.HasPrefix(text, "I open workspace chat"):
		step.Verb = "open_workspace_chat"
		step.Args["target"] = "current"
	case strings.HasPrefix(text, "I open freeform chat for fixture "):
		step.Verb = "open_freeform_chat_fixture"
		step.Args["name"] = firstQuote(text)
	case strings.HasPrefix(text, "I open Thoughts root chat"):
		step.Verb = "open_thoughts_root_chat"
		step.Args["target"] = "current"
	case strings.HasPrefix(text, "I send freeform chat prompt "):
		step.Verb = "send_freeform_chat_prompt"
		step.Args["marker"] = firstQuote(text)
	case strings.HasPrefix(text, "I wait for latest freeform chat run completion"):
		step.Verb = "wait_for_latest_freeform_chat_run_completion"
		step.Args["target"] = "current"
	case strings.HasPrefix(text, "I wait for latest freeform chat run"):
		step.Verb = "wait_for_latest_freeform_chat_run"
		step.Args["target"] = "current"
	case strings.HasPrefix(text, "Latest workspace chats ") && strings.HasSuffix(text, " are seeded."):
		step.Verb = "seed_latest_workspace_chats"
		step.Args["marker_a"] = firstQuote(text)
		step.Args["marker_b"] = secondQuote(text)
	case strings.HasPrefix(text, "I open seeded workspace chat "):
		step.Verb = "open_seeded_workspace_chat"
		step.Args["label"] = firstQuote(text)
	case strings.HasPrefix(text, "I reload chat"):
		step.Verb = "reload_chat"
		step.Args["target"] = "current"
	case strings.HasPrefix(text, "I reopen current chat"):
		step.Verb = "reopen_current_chat"
		step.Args["target"] = "current"
	case strings.HasPrefix(text, "I remember file hash "):
		step.Verb = "remember_file_hash"
		step.Args["path"] = firstQuote(text)
	case strings.HasPrefix(text, "I send Pi docs review prompt "):
		step.Verb = "send_pi_docs_review_prompt"
		step.Args["marker"] = firstQuote(text)
		step.Args["artifact"] = secondQuote(text)
	case strings.HasPrefix(text, "I wait for chat marker "):
		step.Verb = "wait_for_chat_marker"
		step.Args["marker"] = firstQuote(text)
	case strings.HasPrefix(text, "I wait for feature "):
		step.Verb = "wait_for_feature_ready"
		step.Args["feature"] = firstQuote(text)
	case strings.HasPrefix(text, "Region ") && strings.HasSuffix(text, " is visible."):
		step.Verb = "expect_region_visible"
		step.Args["key"] = firstQuote(text)
	case strings.HasPrefix(text, "Region ") && strings.HasSuffix(text, " is reachable."):
		step.Verb = "expect_region_reachable"
		step.Args["key"] = firstQuote(text)
	case strings.HasPrefix(text, "Tab ") && strings.HasSuffix(text, " is selected."):
		step.Verb = "expect_tab_selected"
		step.Args["key"] = firstQuote(text)
	case strings.HasPrefix(text, "Text ") && strings.HasSuffix(text, " is absent."):
		step.Verb = "expect_text_absent"
		step.Args["text"] = firstQuote(text)
	case strings.HasPrefix(text, "Transcript contains "):
		step.Verb = "expect_transcript_contains"
		step.Args["text"] = firstQuote(text)
	case strings.HasPrefix(text, "File ") && strings.Contains(text, " changed from remembered hash"):
		step.Verb = "expect_file_hash_changed"
		step.Args["path"] = firstQuote(text)
	case strings.HasPrefix(text, "File ") && strings.Contains(text, " contains required Pi review sections"):
		step.Verb = "expect_pi_review_file_sections"
		step.Args["path"] = firstQuote(text)
	case strings.HasPrefix(text, "Only file ") && strings.Contains(text, " changed"):
		step.Verb = "expect_only_file_changed"
		step.Args["path"] = firstQuote(text)
	default:
		return Step{}, fmt.Errorf("line %d: unsupported story step %q", line, text)
	}
	if len(step.Args) == 0 || hasEmptyArg(step.Args) {
		return Step{}, fmt.Errorf("line %d: missing quoted argument", line)
	}
	return step, nil
}

func firstQuote(text string) string {
	m := quoted.FindStringSubmatch(text)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

func secondQuote(text string) string {
	matches := quoted.FindAllStringSubmatch(text, -1)
	if len(matches) >= 2 && len(matches[1]) == 2 {
		return matches[1][1]
	}
	return ""
}

func hasEmptyArg(args map[string]string) bool {
	for _, v := range args {
		if v == "" {
			return true
		}
	}
	return false
}

func unquoteValue(text string) string {
	return strings.Trim(strings.TrimSpace(text), `"`)
}

func slugify(in string) string {
	in = strings.ToLower(strings.TrimSpace(in))
	var b strings.Builder
	dash := false
	for _, r := range in {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
