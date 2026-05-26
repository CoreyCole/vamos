package agentchat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAttachablePlanRootMostSpecificNestedQRSPIPlan(t *testing.T) {
	thoughtsRoot := t.TempDir()
	parent := filepath.Join(thoughtsRoot, "agent", "plans", "parent")
	review := filepath.Join(parent, "reviews", "2026-05-26_parent_implementation-review")
	if err := os.MkdirAll(filepath.Join(review, "questions"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(parent, "design.md"),
		filepath.Join(review, "AGENTS.md"),
		filepath.Join(review, "design.md"),
		filepath.Join(review, "outline.md"),
		filepath.Join(review, "plan.md"),
	} {
		if err := os.WriteFile(path, []byte("ok"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	svc := &Service{thoughtsRoot: thoughtsRoot}
	got, ok := svc.ResolveAttachablePlanRoot(filepath.Join(review, "plan.md"))
	if !ok {
		t.Fatal("ResolveAttachablePlanRoot() ok = false")
	}
	wantRel := "agent/plans/parent/reviews/2026-05-26_parent_implementation-review"
	if got.RelPath != wantRel || !got.IsNested || got.ParentRelPath != "agent/plans/parent" {
		t.Fatalf("root = %+v, want rel %q nested with parent", got, wantRel)
	}
}

func TestResolveAttachablePlanRootRejectsLightweightPlanningReview(t *testing.T) {
	thoughtsRoot := t.TempDir()
	review := filepath.Join(thoughtsRoot, "agent", "plans", "parent", "reviews", "2026-05-26_parent_plan-review")
	if err := os.MkdirAll(filepath.Join(review, "questions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(review, "review.md"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := &Service{thoughtsRoot: thoughtsRoot}
	got, ok := svc.ResolveAttachablePlanRoot(filepath.Join(review, "review.md"))
	if !ok {
		t.Fatal("top-level parent plan should remain attachable")
	}
	if got.RelPath != "agent/plans/parent" || got.IsNested {
		t.Fatalf("root = %+v, want parent plan, not planning review", got)
	}
}
