package workbench

import "testing"

func TestRegionSurfaceClass_RailAndChatAreDistinctGrokHex(t *testing.T) {
	t.Parallel()
	threads := WorkbenchRegion{
		ID:   WorkbenchV2ThreadsRegionID,
		Slot: WorkbenchSlotNavigation,
	}
	if got := RegionSurfaceClass(threads); got != "bg-[#111111]" {
		t.Fatalf("threads surface = %q, want bg-[#111111]", got)
	}
	chat := WorkbenchRegion{ID: WorkbenchV2ChatRegionID, Slot: WorkbenchSlotContext}
	if got := RegionSurfaceClass(chat); got != "bg-[#070707]" {
		t.Fatalf("chat surface = %q, want bg-[#070707]", got)
	}
	if RegionSurfaceClass(threads) == RegionSurfaceClass(chat) {
		t.Fatal("rail and chat must not share a surface color")
	}
}
