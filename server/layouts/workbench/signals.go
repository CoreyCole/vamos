package workbench

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func EncodeWorkbenchSignals(state WorkbenchState) string {
	regions := map[string]any{}
	normal := map[string]any{}
	for _, region := range state.Regions {
		key := SignalKey(region)
		regions[key] = map[string]any{
			"visible": region.Visible,
			"ratio":   region.Ratio,
		}
		normal[key] = map[string]any{
			"available": true,
			"visible":   routeNormalVisible(state, region),
		}
	}
	for _, region := range state.NormalRegions {
		key := SignalKeyForID(region.SignalKey)
		normal[key] = map[string]any{
			"available": region.Available,
			"visible":   region.Visible,
		}
	}
	signals := map[string]any{
		"workbench": map[string]any{
			"activeRegionID": SignalKeyForID(state.Config.Mobile.ActiveRegionID),
			"contextMode":    state.ContextMode,
			"view":           string(state.View),
			"focused":        state.FocusDefault,
			"regions":        regions,
			"normalRegions":  normal,
		},
	}
	payload, err := json.Marshal(signals)
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func routeNormalVisible(state WorkbenchState, region WorkbenchRegion) bool {
	key := SignalKey(region)
	for _, normal := range state.NormalRegions {
		if SignalKeyForID(normal.SignalKey) == key {
			return normal.Visible
		}
	}
	return region.Visible
}

func FocusEnterAction(state WorkbenchState) string {
	primary := firstRegionSignalForSlot(state, WorkbenchSlotPrimary)
	parts := []string{
		"$workbench.focused = true",
		"$workbench.activeRegionID = '" + primary + "'",
	}
	for _, region := range state.Regions {
		key := SignalKey(region)
		parts = append(
			parts,
			"$workbench.regions."+key+".visible = "+strconv.FormatBool(
				region.Slot == WorkbenchSlotPrimary,
			),
		)
	}
	return strings.Join(parts, "; ")
}

func FocusExitAction(state WorkbenchState) string {
	primary := firstRegionSignalForSlot(state, WorkbenchSlotPrimary)
	parts := []string{
		"$workbench.focused = false",
		"$workbench.activeRegionID = '" + primary + "'",
	}
	for _, region := range state.Regions {
		key := SignalKey(region)
		parts = append(
			parts,
			"$workbench.regions."+key+".visible = Boolean($workbench.normalRegions."+key+"?.visible)",
		)
	}
	return strings.Join(parts, "; ")
}

func firstRegionSignalForSlot(state WorkbenchState, slot WorkbenchSlot) string {
	for _, region := range state.Regions {
		if region.Slot == slot {
			return SignalKey(region)
		}
	}
	if len(state.Regions) > 0 {
		return SignalKey(state.Regions[0])
	}
	return ""
}


// RegionSSRFlexStyle paints proportional flex before workbench-resize.js runs,
// so room/thread GETs match SavedConfig ratios on first paint (no default→restore snap).

// RegionSurfaceClass — Grok Bot left rail is a lighter muted panel vs chat/artifact.
func RegionSurfaceClass(region WorkbenchRegion) string {
	if region.Slot == WorkbenchSlotNavigation || region.ID == WorkbenchV2ThreadsRegionID {
		return "bg-muted"
	}
	return "bg-card"
}

func RegionSSRFlexStyle(state WorkbenchState, region WorkbenchRegion) string {
	if !region.Visible || state.ViewportClass == ViewportMobile {
		return ""
	}
	var total float64
	for _, r := range state.Regions {
		if r.Visible {
			total += r.Ratio
		}
	}
	if total <= 0 || region.Ratio <= 0 {
		return ""
	}
	return fmt.Sprintf("flex: %.4f 1 0%%", region.Ratio/total)
}

func RegionInitialClass(state WorkbenchState, region WorkbenchRegion) string {
	// Match RegionDataClass !important display locks so first paint survives
	// Datastar hydrate (no one-frame open/closed flip on room/thread GET).
	if !region.Visible {
		return "hidden md:!hidden"
	}
	// On mobile viewport SSR, show the active region immediately so deep links
	// into /threads/:id land on the default Docs/artifact pane before Datastar
	// hydrates data-class. Chat remains available via the Chat tab.
	if state.ViewportClass == ViewportMobile {
		active := SignalKeyForID(state.Config.Mobile.ActiveRegionID)
		if SignalKey(region) == active {
			return "flex min-w-0 flex-1 flex-col"
		}
		return "hidden min-w-0 flex-col md:!flex"
	}
	switch region.Slot {
	case WorkbenchSlotPrimary:
		return "flex min-w-0 flex-1 flex-col"
	case WorkbenchSlotNavigation, WorkbenchSlotContext:
		return "hidden min-w-0 flex-col md:!flex"
	default:
		return "hidden min-w-0 flex-col md:!flex"
	}
}

func RegionDataClass(region WorkbenchRegion) string {
	key := SignalKey(region)
	visible := "$workbench.regions." + key + ".visible"
	active := "$workbench.activeRegionID === '" + key + "'"
	return "{'!hidden': !" + visible + ", 'md:!hidden': !" + visible + ", 'md:!flex': " + visible + ", 'max-md:!hidden': " + visible + " && !(" + active + "), 'max-md:!flex': " + visible + " && (" + active + ")}"
}

func RegionAriaHidden(region WorkbenchRegion) string {
	return "!$workbench.regions." + SignalKey(region) + ".visible ? 'true' : 'false'"
}

func FloatAttr(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func CanResizeAfter(state WorkbenchState, index int) bool {
	return index >= 0 && index < len(state.Regions)-1
}

func NextVisibleSignalKey(state WorkbenchState, index int) string {
	if index < 0 || index >= len(state.Regions)-1 {
		return ""
	}
	return SignalKey(state.Regions[index+1])
}

func mobileRegionTabClick(region WorkbenchRegion) string {
	key := SignalKey(region)
	click := "$workbench.activeRegionID = '" + key + "'; $workbench.regions." + key + ".visible = true; el.closest('#workbench-root').dataset.workbenchMobileActive = '" + key + "'; queueMicrotask(() => el.dispatchEvent(new CustomEvent('workbench-layout-save', {bubbles: true})))"
	if region.Kind == RegionChat {
		click += "; queueMicrotask(() => document.getElementById('chat-latest')?.focus())"
	}
	return click
}
