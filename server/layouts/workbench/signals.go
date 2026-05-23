package workbench

import (
	"encoding/json"
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

func RegionInitialClass(region WorkbenchRegion) string {
	if !region.Visible {
		return "hidden"
	}
	switch region.Slot {
	case WorkbenchSlotPrimary:
		return "flex min-w-0 flex-1 flex-col"
	case WorkbenchSlotNavigation, WorkbenchSlotContext:
		return "hidden min-w-0 flex-col md:flex"
	default:
		return "hidden min-w-0 flex-col md:flex"
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
