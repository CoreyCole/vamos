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
	workbenchV2CommentsMinRem = workbenchV2ChatMinRem
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
	// MobileChatCommentsHeader is set only by ServeThreads and ServeAI470Room.
	// Do not key this off WorkbenchPageThreads; /threads/:id uses that page too.
	MobileChatCommentsHeader bool
	SkipMobileRegionTabs     bool
}

func BuildWorkbenchV2State(args WorkbenchV2Args) (WorkbenchState, error) {
	if args.CommentsOpen {
		args.ChatOpen = false
	}
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
				WorkbenchV2CommentsRegionID,
				WorkbenchSlotContext,
				RegionComments,
				defaultPrimaryRatio,
				workbenchV2CommentsMinRem,
				args.CommentsOpen,
				args.Comments,
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
		},
	})
	if err != nil {
		return state, err
	}
	// First-paint mobile pane follows open columns, not a blanket artifact default.
	// ?artifact= / plan design.md set ArtifactOpen; comments cookie wins over chat.
	if args.ArtifactOpen {
		state.Config.Mobile.ActiveRegionID = WorkbenchV2ArtifactRegionID
	} else if args.CommentsOpen {
		state.Config.Mobile.ActiveRegionID = WorkbenchV2CommentsRegionID
	} else if args.ChatOpen {
		state.Config.Mobile.ActiveRegionID = WorkbenchV2ChatRegionID
	} else {
		state.Config.Mobile.ActiveRegionID = WorkbenchV2ThreadsRegionID
	}
	state.MobileChatCommentsHeader = args.MobileChatCommentsHeader
	state.SkipMobileRegionTabs = args.SkipMobileRegionTabs
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

func threadsRegionVisible(state WorkbenchState) bool {
	for _, region := range state.Regions {
		if region.ID == WorkbenchV2ThreadsRegionID {
			return region.Visible
		}
	}
	return false
}

func WorkbenchV2(args WorkbenchV2Args) templ.Component {
	state, err := BuildWorkbenchV2State(args)
	if err != nil {
		return templ.NopComponent
	}
	return Workbench(state)
}
