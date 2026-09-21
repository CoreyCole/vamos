package agenthome

import (
	"context"
	"strings"
	"testing"
)

func TestProfilePaneDetailsChrome(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	if err := ProfilePane(ProfilePaneArgs{
		Kind: KindDM,
		Slug: "nova",
	}).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, ">Details<") {
		t.Fatalf("title must be Details: %s", out)
	}
	if strings.Contains(out, ">Profile<") {
		t.Fatalf("must not title Profile: %s", out)
	}
	primary := strings.Index(out, `data-testid="artifact-details-primary"`)
	kebab := strings.Index(out, `data-testid="workbench-overflow-actions"`)
	if primary < 0 || kebab < 0 || primary > kebab {
		t.Fatalf("primary >> left of kebab: p=%d k=%d %s", primary, kebab, out)
	}
}
