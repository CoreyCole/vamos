package agenthome

import "strings"

func rosterDirRel(raw string) string {
	rel := strings.TrimSpace(raw)
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.Trim(rel, "/")
	rel = strings.TrimPrefix(rel, "thoughts/")
	return strings.Trim(rel, "/")
}

// RefinePlanRosterSelection picks the most specific Plan-band row for the
// current thoughts path so nested reviews/milestones do not all light up
// when a sibling or parent dir is open.
func RefinePlanRosterSelection(
	sel RosterSelection,
	plans []RosterPlanRow,
) RosterSelection {
	if sel.Kind != KindPlan {
		return sel
	}
	current := rosterDirRel(sel.DirRel)
	if current == "" {
		return sel
	}
	bestID := ""
	bestLen := -1
	for _, plan := range plans {
		dir := rosterDirRel(plan.DirRel)
		if dir == "" {
			continue
		}
		if current == dir || strings.HasPrefix(current, dir+"/") {
			if len(dir) > bestLen {
				bestID = plan.ID
				bestLen = len(dir)
			}
		}
	}
	if bestID == "" {
		return sel
	}
	sel.ID = bestID
	return sel
}
