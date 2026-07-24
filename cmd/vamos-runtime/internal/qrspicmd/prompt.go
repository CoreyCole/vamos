package qrspicmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type PromptContext struct {
	Node       wruntime.Node
	State      ManagerState
	PlanDir    string
	Manifest   string
	LastResult *wruntime.WorkflowResultSnapshot
	Launch     *ChildLaunchIntent
}

func LoadManifest(projectRoot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(projectRoot, "docs", "q-manager.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func ResolveStageSkill(node wruntime.Node) string {
	return strings.TrimSpace(node.Prompt.SkillPath)
}

func RenderStagePrompt(ctx PromptContext) (string, error) {
	skillPath := ResolveStageSkill(ctx.Node)
	latestArtifact := latestPrimaryArtifact(ctx.LastResult)
	if ctx.Launch != nil && ctx.Launch.Kind == ChildLaunchResumeHandoff {
		skillPath = strings.TrimSpace(ctx.Launch.SkillPath)
		latestArtifact = strings.TrimSpace(ctx.Launch.PrimaryArtifact)
	}
	if skillPath == "" {
		return "", fmt.Errorf("node %q does not define a stage skill path", ctx.Node.ID)
	}
	planDir := strings.TrimSpace(ctx.PlanDir)
	if planDir == "" {
		planDir = ctx.State.CanonicalPlanDir
	}
	var b strings.Builder
	b.WriteString("You are a child QRSPI stage session launched by q-manager.\n\n")
	b.WriteString("Read in order:\n")
	b.WriteString("1. .pi/skills/qrspi-planning/SKILL.md\n")
	fmt.Fprintf(&b, "2. %s\n", skillPath)
	fmt.Fprintf(&b, "3. %s/AGENTS.md\n", strings.TrimRight(planDir, "/"))
	if latestArtifact != "" {
		fmt.Fprintf(&b, "4. %s\n", latestArtifact)
	} else {
		b.WriteString("4. Latest primary artifact: none yet\n")
	}
	b.WriteString(
		"\nStart stage immediately unless the loaded stage skill names a human/safety gate.\n",
	)
	b.WriteString("Do not answer ready-to-proceed.\n")
	b.WriteString(
		"When work reaches a terminal state, run `vamos qrspi result init --state-file \"$Q_MANAGER_STATE_FILE\" --state <state> [--outcome <outcome>] [--artifact thoughts/...]`, edit that generated record's summary and contained artifact references, then stop. Do not emit qrspi_result YAML from memory.\n\n",
	)
	fmt.Fprintf(&b, "Plan dir: %s\n", planDir)
	fmt.Fprintf(&b, "Current node: %s\n", ctx.Node.ID)
	fmt.Fprintf(&b, "Graph-selected skill: %s\n", skillPath)
	if ref := ctx.State.LastResultRecord; ref != nil && strings.TrimSpace(ref.ID) != "" {
		b.WriteString(
			"\nPrevious result record (canonical manager-derived handoff context):\n",
		)
		fmt.Fprintf(&b, "- ID: %s\n", ref.ID)
		fmt.Fprintf(&b, "- Path: %s\n", ref.Path)
	} else {
		b.WriteString("\nWorkspace routing:\n")
		fmt.Fprintf(&b, "- Source cwd: %s\n", ctx.State.SourceCwd)
		if strings.TrimSpace(ctx.State.ImplementationCwd) != "" {
			fmt.Fprintf(&b, "- Implementation cwd: %s\n", ctx.State.ImplementationCwd)
		} else {
			b.WriteString(
				"- Implementation cwd: not set yet; before /q-workspace, use source/planning cwd.\n",
			)
		}
	}
	return b.String(), nil
}

func latestPrimaryArtifact(result *wruntime.WorkflowResultSnapshot) string {
	if result == nil {
		return ""
	}
	return strings.TrimSpace(result.PrimaryArtifact)
}
