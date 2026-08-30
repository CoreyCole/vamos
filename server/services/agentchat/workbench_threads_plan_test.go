package agentchat

import "testing"

func TestThoughtsPlanKey(t *testing.T) {
	t.Parallel()
	got := thoughtsPlanKey(
		"/abs/thoughts/user/plans/plan-a/design.md",
	)
	if got != "thoughts/user/plans/plan-a" {
		t.Fatalf("thoughtsPlanKey() = %q", got)
	}
	if thoughtsPlanKey(
		"thoughts/user/plans/plan-a/outline.md",
	) != "thoughts/user/plans/plan-a" {
		t.Fatalf("relative thoughtsPlanKey mismatch")
	}
}
