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

type PiArtifactKind string

const (
	PiArtifactLegacyResult     PiArtifactKind = "legacy_pi_result"
	PiArtifactLegacyCheckpoint PiArtifactKind = "legacy_managed_checkpoint"
	PiArtifactOpaqueSettlement PiArtifactKind = "opaque_settlement"
)

type PiArtifactSchema struct {
	Kind    PiArtifactKind
	Version int
}

func PiArtifactSchemaForPath(planDir, artifactPath string) (PiArtifactSchema, error) {
	planDir, err := filepath.Abs(planDir)
	if err != nil {
		return PiArtifactSchema{}, err
	}
	artifactPath, err = filepath.Abs(artifactPath)
	if err != nil {
		return PiArtifactSchema{}, err
	}
	if !pathWithinPlan(artifactPath, planDir) {
		return PiArtifactSchema{}, fmt.Errorf("Pi artifact path escapes plan directory")
	}
	rel, err := filepath.Rel(
		filepath.Join(planDir, ".vamos", "sessions", "pi"),
		artifactPath,
	)
	if err != nil {
		return PiArtifactSchema{}, err
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) == 1 && strings.HasSuffix(parts[0], "_result.yaml") {
		session := strings.TrimSuffix(parts[0], "_result.yaml")
		if err := ValidateSafeComponent(session); err == nil {
			return PiArtifactSchema{Kind: PiArtifactLegacyResult, Version: 1}, nil
		}
	}
	if len(parts) == 3 && parts[1] == "checkpoints" &&
		strings.HasSuffix(parts[2], ".yaml") {
		if err := ValidateSafeComponent(parts[0]); err != nil {
			return PiArtifactSchema{}, err
		}
		if err := ValidateSafeComponent(
			strings.TrimSuffix(parts[2], ".yaml"),
		); err != nil {
			return PiArtifactSchema{}, err
		}
		return PiArtifactSchema{Kind: PiArtifactLegacyCheckpoint, Version: 2}, nil
	}
	if len(parts) == 3 && parts[1] == "settlements" &&
		strings.HasSuffix(parts[2], ".json") {
		if err := ValidateSafeComponent(parts[0]); err != nil {
			return PiArtifactSchema{}, err
		}
		if err := ValidateSafeComponent(
			strings.TrimSuffix(parts[2], ".json"),
		); err != nil {
			return PiArtifactSchema{}, err
		}
		return PiArtifactSchema{Kind: PiArtifactOpaqueSettlement, Version: 1}, nil
	}
	return PiArtifactSchema{}, fmt.Errorf(
		"unrecognized Pi artifact path %q",
		artifactPath,
	)
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
	root := thoughtsRoot(path)
	if root == "" {
		return filepath.ToSlash(filepath.Clean(path))
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(path))
	}
	return filepath.ToSlash(rel)
}

func thoughtsRoot(path string) string {
	clean := filepath.Clean(path)
	for dir := clean; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if filepath.Base(dir) == "thoughts" {
			return dir
		}
	}
	return ""
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

func CheckpointPath(planDir, sessionID, finalEntryID string) (string, error) {
	if err := ValidateSafeComponent(sessionID); err != nil {
		return "", fmt.Errorf("checkpoint session: %w", err)
	}
	if err := ValidateSafeComponent(finalEntryID); err != nil {
		return "", fmt.Errorf("checkpoint final entry: %w", err)
	}
	planDir, err := filepath.Abs(planDir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(
		planDir,
		".vamos",
		"sessions",
		"pi",
		sessionID,
		"checkpoints",
		finalEntryID+".yaml",
	)
	if !pathWithinPlan(path, planDir) {
		return "", fmt.Errorf("checkpoint path escapes plan directory")
	}
	return path, nil
}

func DeliveryAttemptPath(
	planDir, sessionID, finalEntryID string,
	attempt uint64,
) (string, error) {
	checkpoint, err := CheckpointPath(planDir, sessionID, finalEntryID)
	if err != nil {
		return "", err
	}
	return filepath.Join(
		filepath.Dir(filepath.Dir(checkpoint)), "delivery-attempts",
		fmt.Sprintf("%s-%d.yaml", finalEntryID, attempt),
	), nil
}

func pathWithinPlan(path, planDir string) bool {
	rel, err := filepath.Rel(planDir, path)
	return err == nil && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
