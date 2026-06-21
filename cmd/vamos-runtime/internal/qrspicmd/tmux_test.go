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

func TestPasteBufferArgsTargetExactPane(t *testing.T) {
	set := strings.Join(setBufferArgs("q-manager-wake", "hello"), " ")
	if !strings.Contains(set, "set-buffer -b q-manager-wake hello") {
		t.Fatalf("set buffer args = %v", set)
	}

	paste := strings.Join(pasteBufferArgs("q-manager-wake", "%18"), " ")
	if !strings.Contains(paste, "paste-buffer -b q-manager-wake -t %18") {
		t.Fatalf("paste args = %v", paste)
	}
}
