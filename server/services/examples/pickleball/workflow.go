package pickleball

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/generatedgo"
)

const WorkflowPickleballSelfModify = "pickleball-self-modify"

type AIGenerateInput struct {
	SessionID     string
	Prompt        string
	WorkspacePath string
	History       []BuildSnapshot
}

type AIGenerator interface {
	ApplyPrompt(ctx context.Context, input AIGenerateInput) error
}

func BuildAIPrompt(req PromptRequest, history []BuildSnapshot) string {
	var b strings.Builder
	b.WriteString("You are editing a generated Go bundle for the Vamos pickleball self-modifying app example.\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Edit only the generated bundle workspace.\n")
	b.WriteString("- Preserve one-shot behavior; do not start a server or use the network.\n")
	b.WriteString("- Write app.html, results.csv, and manifest.json to VAMOS_GENERATED_OUTPUT_DIR.\n")
	b.WriteString("- Keep generated HTML iframe-safe and mobile-friendly.\n")
	if len(history) > 0 {
		b.WriteString("\nRecent successful builds:\n")
		for i, snapshot := range history {
			if i >= 5 {
				break
			}
			fmt.Fprintf(&b, "- %s: %s (%s)\n", snapshot.BuildID, snapshot.PromptSummary, snapshot.HTMLThoughtsPath)
		}
	}
	b.WriteString("\nUser prompt:\n")
	b.WriteString(strings.TrimSpace(req.Prompt))
	b.WriteByte('\n')
	return b.String()
}

func SeedOrUpdateGeneratedWorkspace(ctx context.Context, session PickleballSession, seedDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(seedDir) == "" {
		seedDir = defaultSeedDir
	}
	if strings.TrimSpace(session.WorkspacePath) == "" {
		return fmt.Errorf("session workspace path is required")
	}
	return copyDirIfMissing(seedDir, session.WorkspacePath)
}

func BuildOneShotRunnerInput(session PickleballSession, buildID string, outputDir string) generatedgo.RunnerInput {
	return generatedgo.RunnerInput{
		WorkspaceDir:      session.WorkspacePath,
		OutputDir:         outputDir,
		ModulePath:        ".",
		CompileTimeout:    30 * time.Second,
		RunTimeout:        30 * time.Second,
		ArtifactAllowlist: []string{"app.html", "results.csv", "manifest.json"},
		EnvAllowlist: map[string]string{
			"VAMOS_GENERATED_BUILD_ID": buildID,
			"VAMOS_PARENT_BUILD_ID":    session.CurrentBuildID,
		},
	}
}

func BuildSnapshotFromRunner(session PickleballSession, result generatedgo.RunnerResult, snapshotPath string) BuildSnapshot {
	snapshotPath = cleanRelativePath(snapshotPath)
	htmlPath := pathJoinSlash(snapshotPath, result.Manifest.Artifacts.HTML)
	csvPath := pathJoinSlash(snapshotPath, result.Manifest.Artifacts.CSV)
	return BuildSnapshot{
		BuildID:          result.Manifest.BuildID,
		ParentBuildID:    result.Manifest.ParentBuildID,
		PromptSummary:    result.Manifest.PromptSummary,
		Mode:             string(result.Manifest.Mode),
		Status:           string(result.Status),
		SnapshotPath:     snapshotPath,
		ManifestPath:     pathJoinSlash(snapshotPath, "manifest.json"),
		HTMLThoughtsPath: htmlPath,
		CSVThoughtsPath:  csvPath,
		SourceHash:       result.SourceHash,
		HTMLHash:         result.ArtifactHashes["app.html"],
		CSVHash:          result.ArtifactHashes["results.csv"],
		CreatedAt:        time.Now().UTC(),
	}
}

func pathJoinSlash(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(cleanRelativePath(part), "/")
		if part != "" {
			clean = append(clean, part)
		}
	}
	return filepath.ToSlash(filepath.Join(clean...))
}
