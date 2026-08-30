package workbench

import "github.com/a-h/templ"

const (
	WorkbenchV2ThreadsRegionID  = "workbench-v2-threads"
	WorkbenchV2ChatRegionID     = "workbench-v2-chat"
	WorkbenchV2ArtifactRegionID = "workbench-v2-artifact"
	WorkbenchV2CommentsRegionID = "workbench-v2-comments"
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
	return BuildWorkbenchState(BuildWorkbenchStateInput{
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
				args.ThreadsOpen,
				args.Threads,
			),
			v2Region(
				WorkbenchV2ChatRegionID,
				WorkbenchSlotContext,
				RegionChat,
				defaultPrimaryRatio,
				args.ChatOpen,
				args.Chat,
			),
			v2Region(
				WorkbenchV2ArtifactRegionID,
				WorkbenchSlotPrimary,
				RegionArtifact,
				defaultPrimaryRatio,
				args.ArtifactOpen,
				args.Artifact,
			),
			v2Region(
				WorkbenchV2CommentsRegionID,
				WorkbenchSlotContext,
				RegionComments,
				defaultSideRatio,
				args.CommentsOpen,
				args.Comments,
			),
		},
	})
}

func v2Region(
	id string,
	slot WorkbenchSlot,
	kind RegionKind,
	ratio float64,
	visible bool,
	component templ.Component,
) WorkbenchRegion {
	return WorkbenchRegion{
		ID: id, TargetID: id, BodyID: id + "-body", Slot: slot, Kind: kind,
		Ratio: ratio, Visible: visible, Component: component,
	}
}

func WorkbenchV2(args WorkbenchV2Args) templ.Component {
	state, err := BuildWorkbenchV2State(args)
	if err != nil {
		return templ.NopComponent
	}
	return Workbench(state)
}
