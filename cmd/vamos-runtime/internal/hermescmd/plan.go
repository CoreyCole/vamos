package hermescmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type PlanContext struct {
	PlanDir                 string
	PlanRel                 string
	PlanGoal                string
	ImplementationWorkspace string
}

type planFrontmatter struct {
	PlanGoal                string `yaml:"plan_goal"`
	ImplementationWorkspace string `yaml:"implementation_workspace"`
}

func LoadPlanContext(planDir string) (PlanContext, error) {
	absolute, err := filepath.Abs(planDir)
	if err != nil {
		return PlanContext{}, err
	}
	data, err := os.ReadFile(filepath.Join(absolute, "AGENTS.md"))
	if err != nil {
		return PlanContext{}, fmt.Errorf("read plan AGENTS.md: %w", err)
	}
	parts := strings.SplitN(string(data), "---", 3)
	if len(parts) < 3 || strings.TrimSpace(parts[0]) != "" {
		return PlanContext{}, fmt.Errorf(
			"plan AGENTS.md must begin with YAML frontmatter",
		)
	}
	var front planFrontmatter
	if err := yaml.Unmarshal([]byte(parts[1]), &front); err != nil {
		return PlanContext{}, fmt.Errorf("parse plan frontmatter: %w", err)
	}
	return PlanContext{
		PlanDir:                 absolute,
		PlanRel:                 thoughtsRelative(absolute),
		PlanGoal:                front.PlanGoal,
		ImplementationWorkspace: front.ImplementationWorkspace,
	}, nil
}

func thoughtsRelative(path string) string {
	clean := filepath.Clean(path)
	for dir := clean; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if filepath.Base(dir) == "thoughts" {
			return filepath.ToSlash(
				strings.TrimPrefix(clean, dir+string(filepath.Separator)),
			)
		}
	}
	return filepath.ToSlash(clean)
}

// ValidateSafeComponent accepts opaque IDs used as one filesystem path component.
// Components may contain ASCII letters, digits, hyphens, and underscores only.
func ValidateSafeComponent(value string) error {
	if value == "" || value == "." || value == ".." {
		return fmt.Errorf("unsafe empty or dot path component %q", value)
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' {
			continue
		}
		return fmt.Errorf("unsafe path component %q", value)
	}
	return nil
}

func ResultPath(planDir, sessionID string) string {
	return filepath.Join(planDir, ".vamos", "sessions", "pi", sessionID+"_result.yaml")
}
