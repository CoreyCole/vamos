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
	b.WriteString("3. docs/q-manager.md (provided below or from project root)\n")
	fmt.Fprintf(&b, "4. %s/AGENTS.md\n", strings.TrimRight(planDir, "/"))
	latestArtifact := latestPrimaryArtifact(ctx.LastResult)
	if latestArtifact != "" {
		fmt.Fprintf(&b, "5. %s\n", latestArtifact)
	} else {
		b.WriteString("5. Latest primary artifact: none yet\n")
	}
	b.WriteString("\nStart stage immediately unless the loaded stage skill names a human/safety gate.\n")
	b.WriteString("Do not answer ready-to-proceed.\n")
	b.WriteString("Emit the required fenced YAML qrspi_result on completion.\n\n")
	fmt.Fprintf(&b, "Plan dir: %s\n", planDir)
	fmt.Fprintf(&b, "Current node: %s\n", ctx.Node.ID)
	fmt.Fprintf(&b, "Graph-selected skill: %s\n", skillPath)
	b.WriteString("\nWorkspace routing:\n")
	fmt.Fprintf(&b, "- Source cwd: %s\n", ctx.State.SourceCwd)
	if strings.TrimSpace(ctx.State.ImplementationCwd) != "" {
		fmt.Fprintf(&b, "- Implementation cwd: %s\n", ctx.State.ImplementationCwd)
		b.WriteString("- For implementation/review/verify stages, use implementation cwd when graph semantics require it.\n")
	} else {
		b.WriteString("- Implementation cwd: not set yet; before /q-workspace, use source/planning cwd.\n")
	}
	if strings.TrimSpace(ctx.Manifest) != "" {
		b.WriteString("\nProject manifest excerpt:\n```markdown\n")
		b.WriteString(strings.TrimSpace(ctx.Manifest))
		b.WriteString("\n```\n")
	} else {
		b.WriteString("\nProject manifest excerpt: docs/q-manager.md not found.\n")
	}
	if ctx.LastResult != nil {
		b.WriteString("\nLatest result summary:\n")
		fmt.Fprintf(&b, "- Source node: %s\n", ctx.LastResult.SourceNodeID)
		fmt.Fprintf(&b, "- Status: %s\n", ctx.LastResult.Status)
		if ctx.LastResult.Outcome != "" {
			fmt.Fprintf(&b, "- Outcome: %s\n", ctx.LastResult.Outcome)
		}
		if strings.TrimSpace(ctx.LastResult.Summary) != "" {
			fmt.Fprintf(&b, "- Summary: %s\n", ctx.LastResult.Summary)
		}
		if latestArtifact != "" {
			fmt.Fprintf(&b, "- Latest artifact: %s\n", latestArtifact)
		}
	} else {
		b.WriteString("\nLatest result summary: none yet.\n")
	}
	return b.String(), nil
}

func latestPrimaryArtifact(result *wruntime.WorkflowResultSnapshot) string {
	if result == nil {
		return ""
	}
	return strings.TrimSpace(result.PrimaryArtifact)
}
