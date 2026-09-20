package agentchat

import "testing"

func TestApplyChatQuote(t *testing.T) {
	t.Parallel()
	got := ApplyChatQuote("please explain", "selected line", "thoughts/plan.md")
	want := "plan.md\n> selected line\n\nplease explain"
	if got != want {
		t.Fatalf("ApplyChatQuote() = %q, want %q", got, want)
	}
	if got := ApplyChatQuote("only", "", ""); got != "only" {
		t.Fatalf("empty quote should leave prompt: %q", got)
	}
}
