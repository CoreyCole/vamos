package qrspicmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type linearCommandCall struct {
	name string
	args []string
}

type linearCommandRunner struct {
	calls []linearCommandCall
}

func (r *linearCommandRunner) Run(
	_ context.Context,
	name string,
	args ...string,
) (CommandResult, error) {
	r.calls = append(r.calls, linearCommandCall{name: name, args: args})
	switch len(r.calls) {
	case 1:
		return CommandResult{Stdout: `[]`}, nil // no existing root thread
	case 2:
		return CommandResult{Stdout: "root-comment\n"}, nil
	default:
		return CommandResult{Stdout: "stage-reply\n"}, nil
	}
}

func TestPostLinearStageUpdateCreatesAndReusesOneRootThread(t *testing.T) {
	planDir := t.TempDir()
	linearDir := filepath.Join(planDir, "context", "question", "linear")
	if err := os.MkdirAll(linearDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(linearDir, "issue.json"),
		[]byte(`{"id":"issue-123","identifier":"ENG-123"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	state := ManagerState{CanonicalPlanDir: planDir}
	runner := &linearCommandRunner{}
	parsed := ParsedDecision{Result: wruntime.WorkflowResult{
		SourceNodeID:    "design",
		Status:          "complete",
		Outcome:         "complete",
		Summary:         "design written",
		PrimaryArtifact: "thoughts/example/design.md",
	}}

	first := postLinearStageUpdate(t.Context(), &state, parsed, runner)
	if first.Error != nil || state.LinearIssueID != "issue-123" ||
		state.LinearRootCommentID != "root-comment" {
		t.Fatalf("first update/state = %+v / %+v", first, state)
	}
	if len(runner.calls) != 3 || runner.calls[2].args[4] != "root-comment" {
		t.Fatalf("calls = %+v", runner.calls)
	}

	second := postLinearStageUpdate(t.Context(), &state, parsed, runner)
	if second.Error != nil || len(runner.calls) != 4 {
		t.Fatalf("second update/calls = %+v / %+v", second, runner.calls)
	}
	for _, arg := range runner.calls[3].args {
		if arg == "root-comment" {
			return
		}
	}
	t.Fatalf("reply did not use stored root: %+v", runner.calls[3])
}

func TestWriteLinearCommentWakeSurfacesThreadedReplyCommand(t *testing.T) {
	var payload strings.Builder
	writeLinearCommentWake(&payload, ManagerState{
		LinearIssueID:       "issue-123",
		LinearRootCommentID: "root-456",
	}, "/tmp/state.json")
	for _, want := range []string{"issue_ref: \"issue-123\"", "parent_comment_uuid: \"root-456\"", "linear-cli comments create issue-123 --parent-id root-456", "use parent_comment_uuid exactly"} {
		if !strings.Contains(payload.String(), want) {
			t.Fatalf("wake = %q, missing %q", payload.String(), want)
		}
	}
}

func TestFindLinearThreadCommentID(t *testing.T) {
	got := findLinearThreadCommentID(
		`[{"id":"root","body":"QRSPI\n<!-- vamos-qrspi-thread -->"}]`,
	)
	if got != "root" {
		t.Fatalf("comment ID = %q", got)
	}
}
