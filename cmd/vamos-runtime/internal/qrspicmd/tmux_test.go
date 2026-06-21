package qrspicmd

import (
	"strings"
	"testing"
)

func TestSplitPaneArgsTargetsCurrentPaneWhenAvailable(t *testing.T) {
	args := splitPaneArgs(TmuxSplitRequest{Cwd: "/repo", Direction: "right", Command: []string{"echo", "hi"}}, "%18")
	joined := strings.Join(args, " ")
	for _, want := range []string{"split-window", "-t %18", "-h", "-c /repo", "'echo' 'hi'"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("split args missing %q: %v", want, args)
		}
	}
}

func TestSplitPaneArgsOmitsTargetOutsideTmuxPane(t *testing.T) {
	args := splitPaneArgs(TmuxSplitRequest{Cwd: "/repo", Direction: "down", Command: []string{"echo"}}, "")
	joined := strings.Join(args, " ")
	if strings.Contains(joined, " -t ") {
		t.Fatalf("split args unexpectedly include target: %v", args)
	}
	if !strings.Contains(joined, " -v ") {
		t.Fatalf("split args missing vertical split: %v", args)
	}
}
