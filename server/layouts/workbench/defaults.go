package workbench

const (
	defaultSideRatio            = 0.22
	defaultPrimaryRatio         = 0.39
	legacyFreeformPrimaryRatio  = 0.56
	legacyWorkspacePrimaryRatio = 0.52
	legacyWorkspaceContextRatio = 0.26
	minSavedRatio               = 0.12
	maxSavedRatio               = 0.76
	defaultMinRem               = 12
)

func DefaultWorkbenchLayout(page WorkbenchPage) WorkbenchLayoutMode {
	return WorkbenchWorkspaceDocChat
}

func DefaultWorkbenchConfig(
	page WorkbenchPage,
	view WorkbenchView,
	contextMode string,
) WorkbenchConfig {
	cfg := WorkbenchConfig{Version: 1, Page: page, View: view}
	switch page {
	case WorkbenchPageAgentChat:
		cfg.Regions = []RegionSpec{
			{
				ID:      "agent-chat-navigation",
				Slot:    WorkbenchSlotNavigation,
				Kind:    RegionWorkspaceTopology,
				Ratio:   defaultSideRatio,
				Visible: true,
			},
			{
				ID:      "agent-chat-primary",
				Slot:    WorkbenchSlotPrimary,
				Kind:    RegionDoc,
				Ratio:   defaultPrimaryRatio,
				Visible: true,
			},
			{
				ID:      "agent-chat-context",
				Slot:    WorkbenchSlotContext,
				Kind:    RegionChat,
				Ratio:   defaultPrimaryRatio,
				Visible: true,
			},
		}
		cfg.Mobile.ActiveRegionID = "agent-chat-primary"
	case WorkbenchPageThoughts:
		cfg.Regions = []RegionSpec{
			{
				ID:      "thoughts-sections",
				Slot:    WorkbenchSlotNavigation,
				Kind:    RegionSections,
				Ratio:   defaultSideRatio,
				Visible: true,
			},
			{
				ID:      "thoughts-document",
				Slot:    WorkbenchSlotPrimary,
				Kind:    RegionDocument,
				Ratio:   defaultPrimaryRatio,
				Visible: true,
			},
			{
				ID:      "thoughts-context",
				Slot:    WorkbenchSlotContext,
				Kind:    RegionComments,
				Ratio:   defaultSideRatio,
				Visible: view == WorkbenchViewSplit || contextMode != "",
			},
		}
		cfg.Mobile.ActiveRegionID = "thoughts-document"
	default:
		return DefaultWorkbenchConfig(WorkbenchPageAgentChat, view, contextMode)
	}
	return cfg
}

func MergeWorkbenchConfig(
	defaults WorkbenchConfig,
	saved *WorkbenchConfig,
) WorkbenchConfig {
	if saved == nil || ValidateWorkbenchConfig(*saved) != nil ||
		saved.Page != defaults.Page ||
		saved.View != defaults.View {
		return defaults
	}
	byID := map[string]RegionSpec{}
	for _, region := range saved.Regions {
		byID[region.ID] = region
	}
	out := cloneWorkbenchConfig(defaults)
	for i, region := range out.Regions {
		if savedRegion, ok := byID[region.ID]; ok {
			out.Regions[i].Ratio = clamp(savedRegion.Ratio, minSavedRatio, maxSavedRatio)
		}
	}
	migrateLegacyAgentChatSplitRatios(&out)
	return out
}

func StripDurableInteractionState(
	config WorkbenchConfig,
	defaults WorkbenchConfig,
) WorkbenchConfig {
	stripped := cloneWorkbenchConfig(config)
	defaultByID := map[string]RegionSpec{}
	for _, region := range defaults.Regions {
		defaultByID[region.ID] = region
	}
	for i, region := range stripped.Regions {
		if defaultRegion, ok := defaultByID[region.ID]; ok {
			stripped.Regions[i].Visible = defaultRegion.Visible
		} else {
			stripped.Regions[i].Visible = false
		}
	}
	stripped.Mobile.ActiveRegionID = defaults.Mobile.ActiveRegionID
	return stripped
}

func migrateLegacyAgentChatSplitRatios(config *WorkbenchConfig) {
	if config.Page != WorkbenchPageAgentChat {
		return
	}
	primaryIndex, contextIndex := -1, -1
	for i, region := range config.Regions {
		switch region.ID {
		case "agent-chat-primary":
			primaryIndex = i
		case "agent-chat-context":
			contextIndex = i
		}
	}
	if primaryIndex < 0 || contextIndex < 0 {
		return
	}
	primary := config.Regions[primaryIndex].Ratio
	context := config.Regions[contextIndex].Ratio
	if approxEqual(primary, legacyFreeformPrimaryRatio) &&
		approxEqual(context, defaultSideRatio) ||
		approxEqual(primary, legacyWorkspacePrimaryRatio) &&
			approxEqual(context, legacyWorkspaceContextRatio) {
		config.Regions[primaryIndex].Ratio = defaultPrimaryRatio
		config.Regions[contextIndex].Ratio = defaultPrimaryRatio
	}
}

func approxEqual(a, b float64) bool {
	const epsilon = 0.0001
	return a > b-epsilon && a < b+epsilon
}

func BuildWorkbenchState(input BuildWorkbenchStateInput) (WorkbenchState, error) {
	defaults := configFromRegions(
		input.Page,
		input.View,
		input.Regions,
		defaultMobileActiveRegion(input.Regions),
	)
	config := MergeWorkbenchConfig(defaults, input.SavedConfig)
	regions := applyConfigToRegions(input.Regions, config)
	if input.FocusDefault {
		for i := range regions {
			regions[i].Visible = regions[i].Slot == WorkbenchSlotPrimary
		}
		config = configFromRegions(
			input.Page,
			input.View,
			regions,
			defaultMobileActiveRegion(regions),
		)
	}
	state := WorkbenchState{
		UserEmail:     input.UserEmail,
		Page:          input.Page,
		View:          input.View,
		ActivePath:    input.ActivePath,
		ContextMode:   input.ContextMode,
		RouteHref:     input.RouteHref,
		Config:        config,
		Regions:       regions,
		FocusDefault:  input.FocusDefault,
		NormalRegions: normalizeNormalRegions(input.NormalRegions, input.Regions),
	}
	return state, ValidateWorkbenchConfig(config)
}

func configFromRegions(
	page WorkbenchPage,
	view WorkbenchView,
	regions []WorkbenchRegion,
	mobileActive string,
) WorkbenchConfig {
	pageDefaults := DefaultWorkbenchConfig(page, view, "")
	defaultByID := map[string]RegionSpec{}
	for _, region := range pageDefaults.Regions {
		defaultByID[region.ID] = region
	}
	cfg := WorkbenchConfig{
		Version: 1,
		Page:    page,
		View:    view,
		Mobile:  MobileSpec{ActiveRegionID: mobileActive},
	}
	for _, region := range regions {
		ratio := region.Ratio
		if ratio <= 0 {
			if defaultRegion, ok := defaultByID[region.ID]; ok {
				ratio = defaultRegion.Ratio
			}
		}
		cfg.Regions = append(cfg.Regions, RegionSpec{
			ID:      region.ID,
			Slot:    region.Slot,
			Kind:    region.Kind,
			Ratio:   ratio,
			Visible: region.Visible,
		})
	}
	return cfg
}

func cloneWorkbenchConfig(config WorkbenchConfig) WorkbenchConfig {
	clone := config
	clone.Regions = append([]RegionSpec(nil), config.Regions...)
	return clone
}

func defaultMobileActiveRegion(regions []WorkbenchRegion) string {
	for _, region := range regions {
		if region.Visible && region.Slot == WorkbenchSlotPrimary {
			return region.ID
		}
	}
	for _, region := range regions {
		if region.Visible {
			return region.ID
		}
	}
	if len(regions) > 0 {
		return regions[0].ID
	}
	return ""
}

func normalizeNormalRegions(
	explicit []RegionNormalState,
	regions []WorkbenchRegion,
) []RegionNormalState {
	byKey := map[string]RegionNormalState{}
	for _, region := range regions {
		key := SignalKey(region)
		byKey[key] = RegionNormalState{
			SignalKey: key,
			Available: true,
			Visible:   region.Visible,
		}
	}
	for _, region := range explicit {
		key := SignalKeyForID(region.SignalKey)
		byKey[key] = RegionNormalState{
			SignalKey: key,
			Available: region.Available,
			Visible:   region.Visible,
		}
	}
	out := make([]RegionNormalState, 0, len(byKey))
	for _, region := range regions {
		key := SignalKey(region)
		out = append(out, byKey[key])
		delete(byKey, key)
	}
	for _, region := range byKey {
		out = append(out, region)
	}
	return out
}

func applyConfigToRegions(
	regions []WorkbenchRegion,
	config WorkbenchConfig,
) []WorkbenchRegion {
	byID := map[string]RegionSpec{}
	for _, spec := range config.Regions {
		byID[spec.ID] = spec
	}
	out := make([]WorkbenchRegion, 0, len(regions))
	for _, region := range regions {
		if spec, ok := byID[region.ID]; ok {
			region.Slot = spec.Slot
			region.Kind = spec.Kind
			region.Ratio = spec.Ratio
			region.Visible = spec.Visible
		}
		if region.SignalKey == "" {
			region.SignalKey = SignalKeyForID(region.ID)
		}
		if region.TargetID == "" {
			region.TargetID = region.ID
		}
		if region.MinRem <= 0 {
			region.MinRem = defaultMinRem
		}
		out = append(out, region)
	}
	return out
}

func clamp(value, lower, upper float64) float64 {
	if value < lower {
		return lower
	}
	if value > upper {
		return upper
	}
	return value
}
