package agenthome

import "testing"

func TestRefinePlanRosterSelection_NestedSiblingNotSelected(t *testing.T) {
	t.Parallel()
	parentID := "2026-09-21_19-33-37_equitrust-2026-run-rate-met"
	parentDir := "CoreyCole/plans/" + parentID
	reviewID := "CoreyCole--plans--" + parentID + "--reviews--full-code-review"
	plans := []RosterPlanRow{
		{ID: parentID, DirRel: parentDir, Title: "equitrust 2026 run rate met"},
		{
			ID:     reviewID,
			DirRel: parentDir + "/reviews/2026-09-22_13-33-24_full-code-review",
			Title:  "full code review",
		},
		{
			ID: parentDir + "--reviews--impl",
			DirRel: parentDir +
				"/reviews/2026-09-21_22-10-14_equitrust-2026-run-rate-met_implementation-review",
			Title: "implementation review",
		},
	}
	sel := RefinePlanRosterSelection(RosterSelection{
		Kind:   KindPlan,
		ID:     parentID,
		DirRel: parentDir + "/eric-comparison/comparison-report",
	}, plans)
	if sel.ID != parentID {
		t.Fatalf("nested sibling dir selected %q, want parent %q", sel.ID, parentID)
	}
}

func TestRefinePlanRosterSelection_NestedReviewIsMostSpecific(t *testing.T) {
	t.Parallel()
	parentID := "alpha"
	parentDir := "owner/plans/alpha"
	reviewID := "owner--plans--alpha--reviews--full-code-review"
	reviewDir := parentDir + "/reviews/full-code-review"
	plans := []RosterPlanRow{
		{ID: parentID, DirRel: parentDir},
		{ID: reviewID, DirRel: reviewDir},
	}
	sel := RefinePlanRosterSelection(RosterSelection{
		Kind:   KindPlan,
		ID:     parentID,
		DirRel: reviewDir + "/plan.md",
	}, plans)
	if sel.ID != reviewID {
		t.Fatalf("nested review selected %q, want %q", sel.ID, reviewID)
	}
}

func TestRefinePlanRosterSelection_EmptyDirRelKeepsID(t *testing.T) {
	t.Parallel()
	sel := RefinePlanRosterSelection(RosterSelection{
		Kind: KindPlan,
		ID:   "alpha",
	}, []RosterPlanRow{
		{ID: "alpha", DirRel: "owner/plans/alpha"},
		{ID: "owner--plans--alpha--reviews--r", DirRel: "owner/plans/alpha/reviews/r"},
	})
	if sel.ID != "alpha" {
		t.Fatalf("empty DirRel selected %q", sel.ID)
	}
}
