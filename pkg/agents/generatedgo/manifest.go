package generatedgo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RunnerMode string

const (
	RunnerModeOneShot RunnerMode = "one_shot"
	RunnerModeSSE     RunnerMode = "sse_process"
)

type BuildStatus string

const (
	BuildStatusSucceeded BuildStatus = "succeeded"
	BuildStatusFailed    BuildStatus = "failed"
)

type GeneratedManifest struct {
	SchemaVersion int                `json:"schema_version"`
	BuildID       string             `json:"build_id"`
	ParentBuildID string             `json:"parent_build_id,omitempty"`
	Mode          RunnerMode         `json:"mode"`
	PromptSummary string             `json:"prompt_summary,omitempty"`
	Status        BuildStatus        `json:"status,omitempty"`
	Artifacts     GeneratedArtifacts `json:"artifacts"`
}

type GeneratedArtifacts struct {
	HTML string `json:"html"`
	CSV  string `json:"csv"`
}

func ReadManifest(path string) (GeneratedManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return GeneratedManifest{}, fmt.Errorf("read manifest: %w", err)
	}
	var manifest GeneratedManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return GeneratedManifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	return manifest, nil
}

func ValidateManifest(path string, outputDir string) (GeneratedManifest, error) {
	outputRoot, err := filepath.Abs(outputDir)
	if err != nil {
		return GeneratedManifest{}, fmt.Errorf("resolve output dir: %w", err)
	}
	manifest, err := ReadManifest(path)
	if err != nil {
		return GeneratedManifest{}, err
	}
	if manifest.SchemaVersion != 1 {
		return GeneratedManifest{}, fmt.Errorf("unsupported manifest schema_version %d", manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.BuildID) == "" {
		return GeneratedManifest{}, fmt.Errorf("manifest build_id is required")
	}
	if manifest.Mode != RunnerModeOneShot {
		return GeneratedManifest{}, fmt.Errorf("unsupported runner mode %q", manifest.Mode)
	}
	if err := validateArtifactName(manifest.Artifacts.HTML, "app.html"); err != nil {
		return GeneratedManifest{}, fmt.Errorf("html artifact: %w", err)
	}
	if err := validateArtifactName(manifest.Artifacts.CSV, "results.csv"); err != nil {
		return GeneratedManifest{}, fmt.Errorf("csv artifact: %w", err)
	}
	for _, name := range []string{manifest.Artifacts.HTML, manifest.Artifacts.CSV, "manifest.json"} {
		if err := validateArtifactPath(outputRoot, name); err != nil {
			return GeneratedManifest{}, err
		}
	}
	return manifest, nil
}

func validateArtifactName(got, want string) error {
	got = strings.TrimSpace(got)
	if got == "" {
		return fmt.Errorf("required")
	}
	if got != want {
		return fmt.Errorf("got %q, want %q", got, want)
	}
	return nil
}

func validateArtifactPath(outputRoot, name string) error {
	name = filepath.FromSlash(strings.TrimSpace(name))
	abs := filepath.Join(outputRoot, name)
	if !pathWithinRoot(abs, outputRoot) {
		return fmt.Errorf("artifact %q escapes output dir", name)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("artifact %q missing: %w", name, err)
	}
	if info.IsDir() {
		return fmt.Errorf("artifact %q is a directory", name)
	}
	return nil
}
