package workflows

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

func RenderNodePrompt(
	ctx context.Context,
	def wruntime.Definition,
	node wruntime.Node,
	state wruntime.State,
) (string, error) {
	_ = ctx
	if prompt := strings.TrimSpace(node.Prompt.Static); prompt != "" {
		return prompt, nil
	}
	if skillPath := strings.TrimSpace(node.Prompt.SkillPath); skillPath != "" {
		return skillPrompt(def, node, state, skillPath), nil
	}
	if body := strings.TrimSpace(node.Prompt.Template); body != "" {
		return renderTemplate(body, def, node, state)
	}
	return "", fmt.Errorf("node %q has no prompt", node.ID)
}

func skillPrompt(
	def wruntime.Definition,
	node wruntime.Node,
	state wruntime.State,
	skillPath string,
) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(
		&b,
		"Use the skill at `%s` for workflow `%s` node `%s`.\n\n",
		skillPath,
		def.ID,
		node.ID,
	)
	_, _ = fmt.Fprintf(
		&b,
		"Load the current workflow state and prior artifacts from the Agent Chat workspace. Emit the required QRSPI XML footer with `<stage>%s</stage>`.\n",
		node.ID,
	)
	if state.LastResult != nil {
		_, _ = fmt.Fprintf(
			&b,
			"\nPrevious node `%s` finished with status `%s`. Summary: %s\n",
			state.LastResult.SourceNodeID,
			state.LastResult.Status,
			strings.TrimSpace(state.LastResult.Summary),
		)
		if artifact := strings.TrimSpace(
			state.LastResult.PrimaryArtifact,
		); artifact != "" {
			_, _ = fmt.Fprintf(&b, "Primary artifact: `%s`\n", artifact)
		}
		for _, artifact := range state.LastResult.Artifacts {
			path := strings.TrimSpace(artifact.Path)
			if path == "" {
				continue
			}
			role := strings.TrimSpace(artifact.Role)
			if role == "" {
				role = "related"
			}
			_, _ = fmt.Fprintf(&b, "Related artifact (%s): `%s`\n", role, path)
		}
	}
	if helper := helperPromptContext(state, node); helper != "" {
		_, _ = fmt.Fprintf(&b, "\n%s\n", helper)
	}
	if followup := implementationFollowupPromptContext(state); followup != "" {
		_, _ = fmt.Fprintf(&b, "\n%s\n", followup)
	}
	return strings.TrimSpace(b.String())
}

func helperPromptContext(state wruntime.State, node wruntime.Node) string {
	switch node.ID {
	case "research-for-review-design",
		"research-for-review-outline",
		"research-for-review-plan":
		return researchForReviewPrompt(node.ID, state)
	case "address-review-research-design",
		"address-review-research-outline",
		"address-review-research-plan":
		return addressReviewResearchPrompt(node.ID, state)
	default:
		return ""
	}
}

func researchForReviewPrompt(nodeID wruntime.NodeID, state wruntime.State) string {
	questions := artifactByRole(state.LastResult, "followup-questions", "questions")
	review := ""
	if state.LastResult != nil {
		review = strings.TrimSpace(state.LastResult.PrimaryArtifact)
	}
	return fmt.Sprintf(`Planning-review research helper.
Use /skill:q-research-for-review on the review questions artifact.
Review artifact: %s
Questions artifact: %s
Emit <stage>%s</stage> and include the research artifact plus the source review/questions artifacts in <artifacts>.`, review, questions, nodeID)
}

func addressReviewResearchPrompt(nodeID wruntime.NodeID, state wruntime.State) string {
	review := artifactByRole(state.LastResult, "review")
	research := ""
	if state.LastResult != nil {
		research = strings.TrimSpace(state.LastResult.PrimaryArtifact)
	}
	if review == "" {
		review = artifactByRole(state.LastResult, "source-review", "parent-review")
	}
	return fmt.Sprintf(`Planning-review address helper.
Use /skill:q-address-review-research with the review artifact and review-research artifact.
Review artifact: %s
Research artifact: %s
Emit <stage>%s</stage> after editing parent planning docs and include all modified planning artifacts.`, review, research, nodeID)
}

func artifactByRole(result *wruntime.WorkflowResultSnapshot, roles ...string) string {
	if result == nil {
		return ""
	}
	for _, role := range roles {
		for _, artifact := range result.Artifacts {
			if strings.EqualFold(strings.TrimSpace(artifact.Role), role) {
				return strings.TrimSpace(artifact.Path)
			}
		}
	}
	return ""
}

func implementationFollowupPromptContext(state wruntime.State) string {
	if len(state.Followups) == 0 {
		return ""
	}
	top := state.Followups[len(state.Followups)-1]
	return fmt.Sprintf(`Implementation-review follow-up context is active.
Parent plan dir: %s
Follow-up plan dir: %s
Parent implementation review: %s
Load and write artifacts under the follow-up plan dir until the follow-up implementation review completes. Do not route this follow-up review directly to parent done; runtime will return to the parent review-implementation node.`, top.ParentPlanDir, top.FollowupPlanDir, top.ParentReviewPath)
}

func renderTemplate(
	body string,
	def wruntime.Definition,
	node wruntime.Node,
	state wruntime.State,
) (string, error) {
	tpl, err := template.New(string(node.ID)).Parse(body)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := tpl.Execute(&b, map[string]any{
		"Definition": def,
		"Node":       node,
		"State":      state,
	}); err != nil {
		return "", err
	}
	return strings.TrimSpace(b.String()), nil
}
