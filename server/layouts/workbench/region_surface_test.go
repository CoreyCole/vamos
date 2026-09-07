package workbench

import "testing"

func TestRegionSurfaceClass_LeftRailMuted(t *testing.T) {
	t.Parallel()
	threads := WorkbenchRegion{ID: WorkbenchV2ThreadsRegionID, Slot: WorkbenchSlotNavigation}
	if got := RegionSurfaceClass(threads); got != "bg-muted" {
		t.Fatalf("threads surface = %q, want bg-muted", got)
	}
	chat := WorkbenchRegion{ID: WorkbenchV2ChatRegionID, Slot: WorkbenchSlotContext}
	if got := RegionSurfaceClass(chat); got != "bg-card" {
		t.Fatalf("chat surface = %q, want bg-card", got)
	}
}
