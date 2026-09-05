package workbench

import "github.com/a-h/templ"

const (
	WorkbenchV2ThreadsRegionID  = "workbench-v2-threads"
	WorkbenchV2ChatRegionID     = "workbench-v2-chat"
	WorkbenchV2ArtifactRegionID = "workbench-v2-artifact"
	WorkbenchV2CommentsRegionID = "workbench-v2-comments"

	// Keep chat wide enough that the composer prompt stays usable after grip drags.
	workbenchV2ThreadsMinRem  = 12
	workbenchV2ChatMinRem     = 18
	workbenchV2ArtifactMinRem = 20
	workbenchV2CommentsMinRem = 12
)

type WorkbenchV2Args struct {
	UserEmail     string
	ViewportClass ViewportClass
	SavedConfig   *WorkbenchConfig
	Threads       templ.Component
	Chat          templ.Component
	Artifact      templ.Component
	Comments      templ.Component
	ThreadsOpen   bool
	ChatOpen      bool
	ArtifactOpen  bool
	CommentsOpen  bool
}

func BuildWorkbenchV2State(args WorkbenchV2Args) (WorkbenchState, error) {
	state, err := BuildWorkbenchState(BuildWorkbenchStateInput{
		UserEmail:     args.UserEmail,
		Page:          WorkbenchPageThreads,
		View:          WorkbenchViewSplit,
		ViewportClass: args.ViewportClass,
		SavedConfig:   args.SavedConfig,
		Regions: []WorkbenchRegion{
			v2Region(
				WorkbenchV2ThreadsRegionID,
				WorkbenchSlotNavigation,
				RegionPlanSidebar,
				defaultSideRatio,
				workbenchV2ThreadsMinRem,
				args.ThreadsOpen,
				args.Threads,
			),
			v2Region(
				WorkbenchV2ChatRegionID,
				WorkbenchSlotContext,
				RegionChat,
				defaultPrimaryRatio,
				workbenchV2ChatMinRem,
				args.ChatOpen,
				args.Chat,
			),
			v2Region(
				WorkbenchV2ArtifactRegionID,
				WorkbenchSlotPrimary,
				RegionArtifact,
				defaultPrimaryRatio,
				workbenchV2ArtifactMinRem,
				args.ArtifactOpen,
				args.Artifact,
			),
			v2Region(
				WorkbenchV2CommentsRegionID,
				WorkbenchSlotContext,
				RegionComments,
				defaultSideRatio,
				workbenchV2CommentsMinRem,
				args.CommentsOpen,
				args.Comments,
			),
		},
	})
	if err != nil {
		return state, err
	}
	// When chat is open (thread selected), mobile deep links are chat-first.
	// ?artifact= still loads Docs content but does not steal the visible tab.
	// Index / thoughts shells with chat closed keep the previous default.
	if args.ChatOpen {
		state.Config.Mobile.ActiveRegionID = WorkbenchV2ChatRegionID
	}
	return state, nil
}

func v2Region(
	id string,
	slot WorkbenchSlot,
	kind RegionKind,
	ratio float64,
	minRem float64,
	visible bool,
	component templ.Component,
) WorkbenchRegion {
	return WorkbenchRegion{
		ID: id, TargetID: id, BodyID: id + "-body", Slot: slot, Kind: kind,
		Ratio: ratio, MinRem: minRem, Visible: visible, Component: component,
	}
}

func WorkbenchV2(args WorkbenchV2Args) templ.Component {
	state, err := BuildWorkbenchV2State(args)
	if err != nil {
		return templ.NopComponent
	}
	return Workbench(state)
}
