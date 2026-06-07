package qrspi

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type CommitFooter struct {
	Plan      string   `yaml:"plan" json:"plan,omitempty"`
	Stage     string   `yaml:"stage" json:"stage,omitempty"`
	Slice     string   `yaml:"slice" json:"slice,omitempty"`
	Summary   string   `yaml:"summary" json:"summary,omitempty"`
	Artifacts []string `yaml:"artifacts" json:"artifacts,omitempty"`
}

type commitEnvelope struct {
	Commit CommitFooter `yaml:"qrspi_commit"`
}

func ParseQRSPICommitFooter(message string) (CommitFooter, error) {
	yamlText, err := extractQRSPICommitYAML(message)
	if err != nil {
		return CommitFooter{}, err
	}
	var envelope commitEnvelope
	decoder := yaml.NewDecoder(strings.NewReader(yamlText))
	decoder.KnownFields(true)
	if err := decoder.Decode(&envelope); err != nil {
		return CommitFooter{}, fmt.Errorf("parse qrspi commit YAML: %w", err)
	}
	footer := trimQRSPICommitFooter(envelope.Commit)
	if footer.Empty() {
		return CommitFooter{}, fmt.Errorf("missing top-level qrspi_commit")
	}
	return footer, nil
}

func FormatQRSPICommitFooter(commit CommitFooter) string {
	data, _ := yaml.Marshal(commitEnvelope{Commit: trimQRSPICommitFooter(commit)})
	return "```yaml\n" + strings.TrimSpace(string(data)) + "\n```"
}

func extractQRSPICommitYAML(message string) (string, error) {
	for _, match := range fencedYAMLPattern.FindAllStringSubmatch(message, -1) {
		candidate := strings.TrimSpace(match[1])
		if candidate == "" {
			continue
		}
		if hasQRSPICommitRoot(candidate) {
			return candidate, nil
		}
	}
	whole := strings.TrimSpace(message)
	if hasOnlyQRSPICommitRoot(whole) {
		return whole, nil
	}
	return "", fmt.Errorf("missing fenced YAML qrspi_commit")
}

func hasQRSPICommitRoot(text string) bool {
	var root map[string]any
	decoder := yaml.NewDecoder(strings.NewReader(text))
	if err := decoder.Decode(&root); err != nil {
		return false
	}
	_, ok := root["qrspi_commit"]
	return ok
}

func hasOnlyQRSPICommitRoot(text string) bool {
	var root map[string]any
	decoder := yaml.NewDecoder(strings.NewReader(text))
	if err := decoder.Decode(&root); err != nil {
		return false
	}
	_, ok := root["qrspi_commit"]
	return ok && len(root) == 1
}

func trimQRSPICommitFooter(commit CommitFooter) CommitFooter {
	commit.Plan = strings.TrimSpace(commit.Plan)
	commit.Stage = strings.TrimSpace(commit.Stage)
	commit.Slice = strings.TrimSpace(commit.Slice)
	commit.Summary = strings.TrimSpace(commit.Summary)
	artifacts := make([]string, 0, len(commit.Artifacts))
	for _, artifact := range commit.Artifacts {
		artifact = strings.TrimSpace(artifact)
		if artifact == "" {
			continue
		}
		artifacts = append(artifacts, artifact)
	}
	commit.Artifacts = artifacts
	return commit
}

func (commit CommitFooter) Empty() bool {
	return commit.Plan == "" &&
		commit.Stage == "" &&
		commit.Slice == "" &&
		commit.Summary == "" &&
		len(commit.Artifacts) == 0
}
