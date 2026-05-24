package artifacts

import (
	"os"
	"path/filepath"
	"time"
)

type RunManifest struct {
	ID             string    `json:"id"`
	StartedAt      time.Time `json:"startedAt"`
	CompletedAt    time.Time `json:"completedAt,omitempty"`
	RepoCommit     string    `json:"repoCommit,omitempty"`
	Stories        []string  `json:"stories,omitempty"`
	GeneratedFiles []string  `json:"generatedFiles,omitempty"`
	Screenshots    []string  `json:"screenshots,omitempty"`
	HTMLSnapshots  []string  `json:"htmlSnapshots,omitempty"`
	Traces         []string  `json:"traces,omitempty"`
	FailuresPath   string    `json:"failuresPath,omitempty"`
	PlanBundlePath string    `json:"planBundlePath,omitempty"`
}

type Failure struct {
	Story         string   `json:"story,omitempty"`
	Scenario      string   `json:"scenario,omitempty"`
	Viewport      string   `json:"viewport,omitempty"`
	Step          string   `json:"step,omitempty"`
	Error         string   `json:"error"`
	ArtifactPaths []string `json:"artifactPaths,omitempty"`
}

func NewRun(root string) (RunManifest, error) {
	id := time.Now().UTC().Format("20060102T150405Z")
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return RunManifest{}, err
	}
	return RunManifest{ID: id, StartedAt: time.Now().UTC()}, nil
}

func RunDir(root string, manifest RunManifest) string {
	return filepath.Join(root, manifest.ID)
}
