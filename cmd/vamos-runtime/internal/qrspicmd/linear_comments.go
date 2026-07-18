package qrspicmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

const linearQRSPIThreadMarker = "<!-- vamos-qrspi-thread -->"

type LinearCommentUpdate struct {
	IssueID        string
	RootCommentID  string
	ReplyCommentID string
	Error          error
}

// DetectLinearIssue records the durable Linear issue identity captured by
// q-question. It deliberately has no CLI flags: q-manager derives it from the
// plan that it is managing.
func DetectLinearIssue(planDir string) string {
	data, err := os.ReadFile(
		filepath.Join(planDir, "context", "question", "linear", "issue.json"),
	)
	if err != nil {
		return ""
	}
	var issue map[string]any
	if json.Unmarshal(data, &issue) != nil {
		return ""
	}
	for _, key := range []string{"id", "identifier"} {
		if value, ok := issue[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func postLinearStageUpdate(
	ctx context.Context,
	state *ManagerState,
	parsed ParsedDecision,
	runner CommandRunner,
) LinearCommentUpdate {
	update := LinearCommentUpdate{}
	if state == nil {
		return update
	}
	if state.LinearIssueID == "" {
		state.LinearIssueID = DetectLinearIssue(state.CanonicalPlanDir)
	}
	update.IssueID = state.LinearIssueID
	if update.IssueID == "" {
		return update
	}
	if runner == nil {
		runner = ShellCommandRunner{}
	}

	if state.LinearRootCommentID == "" {
		rootID, err := findOrCreateLinearRootComment(ctx, runner, update.IssueID)
		if err != nil {
			update.Error = err
			return update
		}
		state.LinearRootCommentID = rootID
	}
	update.RootCommentID = state.LinearRootCommentID
	body := formatLinearStageUpdate(parsed.Result)
	result, err := runner.Run(ctx, "linear-cli", "comments", "create", update.IssueID,
		"--parent-id", update.RootCommentID, "--body", body, "--id-only")
	if err != nil {
		update.Error = fmt.Errorf(
			"post Linear QRSPI reply: %w: %s",
			err,
			strings.TrimSpace(result.Stderr),
		)
		return update
	}
	update.ReplyCommentID = linearCommentID(result.Stdout)
	return update
}

func findOrCreateLinearRootComment(
	ctx context.Context,
	runner CommandRunner,
	issueID string,
) (string, error) {
	result, err := runner.Run(
		ctx,
		"linear-cli",
		"comments",
		"list",
		issueID,
		"--all",
		"--output",
		"json",
	)
	if err == nil {
		if id := findLinearThreadCommentID(result.Stdout); id != "" {
			return id, nil
		}
	}
	result, err = runner.Run(ctx, "linear-cli", "comments", "create", issueID,
		"--body", "QRSPI progress updates\n\n"+linearQRSPIThreadMarker, "--id-only")
	if err != nil {
		return "", fmt.Errorf(
			"create Linear QRSPI root comment: %w: %s",
			err,
			strings.TrimSpace(result.Stderr),
		)
	}
	if id := linearCommentID(result.Stdout); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("create Linear QRSPI root comment: missing comment ID")
}

func findLinearThreadCommentID(text string) string {
	var value any
	if json.Unmarshal([]byte(text), &value) != nil {
		return ""
	}
	var walk func(any) string
	walk = func(value any) string {
		switch value := value.(type) {
		case []any:
			for _, item := range value {
				if id := walk(item); id != "" {
					return id
				}
			}
		case map[string]any:
			body, _ := value["body"].(string)
			id, _ := value["id"].(string)
			if strings.Contains(body, linearQRSPIThreadMarker) && id != "" {
				return id
			}
			for _, item := range value {
				if id := walk(item); id != "" {
					return id
				}
			}
		}
		return ""
	}
	return walk(value)
}

func linearCommentID(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	var value any
	if json.Unmarshal([]byte(trimmed), &value) == nil {
		if object, ok := value.(map[string]any); ok {
			if id, ok := object["id"].(string); ok {
				return id
			}
		}
	}
	return strings.Fields(trimmed)[0]
}

func formatLinearStageUpdate(result wruntime.WorkflowResult) string {
	lines := []string{
		fmt.Sprintf("QRSPI update: `%s` %s", result.SourceNodeID, result.Status),
	}
	if result.Outcome != "" {
		lines[0] += fmt.Sprintf(" (%s)", result.Outcome)
	}
	if result.Summary != "" {
		lines = append(lines, "", "- "+result.Summary)
	}
	if result.PrimaryArtifact != "" {
		lines = append(lines, "- Artifact: `"+result.PrimaryArtifact+"`")
	}
	return strings.Join(lines, "\n")
}
