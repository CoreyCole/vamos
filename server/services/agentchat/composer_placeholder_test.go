package agentchat

import "testing"

func TestSharedThreadComposerPlaceholder(t *testing.T) {
	t.Parallel()
	if got := composerPlaceholderForTitle("Nova"); got != "Message Nova" {
		t.Fatalf("title placeholder = %q", got)
	}
	if got := composerPlaceholderForTitle(""); got != "Message…" {
		t.Fatalf("empty title = %q", got)
	}
	if got := sharedThreadComposerPlaceholder(EmbeddedFreeformPanelArgs{
		HasThread: true,
	}); got != "Message…" {
		t.Fatalf("default thread placeholder = %q", got)
	}
	if got := sharedThreadComposerPlaceholder(EmbeddedFreeformPanelArgs{
		HasThread:   true,
		Placeholder: "Message Nova",
	}); got != "Message Nova" {
		t.Fatalf("explicit placeholder = %q", got)
	}
}
