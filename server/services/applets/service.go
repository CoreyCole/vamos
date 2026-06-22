package applets

import (
	"context"
	"strings"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/a-h/templ"
)

const (
	filesAppSuffix = "files-app"
	chatSuffix     = "chat"
)

type Service struct{}

func NewService() *Service { return &Service{} }

func BuildWorkbenchState(_ context.Context, state WorkbenchState) workbench.WorkbenchState {
	appID := strings.TrimSpace(state.Config.ID)
	if appID == "" {
		appID = "applet"
	}

	filesTitle := firstNonEmpty(state.Files.Title, state.Config.UserLabels.FilesTitle, "Files")
	chatTitle := firstNonEmpty(state.Chat.Title, state.Config.UserLabels.ChatTitle, "Chat")

	files := state.Files.Component
	if files == nil {
		files = EmptyRegion("Files will appear here.")
	}
	chat := state.Chat.Component
	if chat == nil {
		chat = EmptyRegion("Chat will appear here.")
	}

	filesRegionID := appID + "-" + filesAppSuffix
	chatRegionID := appID + "-" + chatSuffix
	mobileActive := strings.TrimSpace(state.MobileActive)
	if mobileActive == "" {
		mobileActive = filesRegionID
	}

	wb, _ := workbench.BuildWorkbenchState(workbench.BuildWorkbenchStateInput{
		Page:       workbench.WorkbenchPageAgentChat,
		View:       workbench.WorkbenchViewSplit,
		ActivePath: appID,
		RouteHref:  "/examples/" + appID,
		Regions: []workbench.WorkbenchRegion{
			{
				ID:        filesRegionID,
				Slot:      workbench.WorkbenchSlotPrimary,
				Kind:      workbench.RegionDoc,
				Ratio:     0.62,
				MinRem:    24,
				Visible:   true,
				TargetID:  appID + "-files-app-region",
				Title:     filesTitle,
				Component: files,
			},
			{
				ID:        chatRegionID,
				Slot:      workbench.WorkbenchSlotContext,
				Kind:      workbench.RegionChat,
				Ratio:     0.38,
				MinRem:    20,
				Visible:   true,
				TargetID:  appID + "-chat-region",
				Title:     chatTitle,
				Component: chat,
			},
		},
		SavedConfig: &workbench.WorkbenchConfig{
			Version: 1,
			Page:    workbench.WorkbenchPageAgentChat,
			View:    workbench.WorkbenchViewSplit,
			Regions: []workbench.RegionSpec{
				{ID: filesRegionID, Slot: workbench.WorkbenchSlotPrimary, Kind: workbench.RegionDoc, Ratio: 0.62, Visible: true},
				{ID: chatRegionID, Slot: workbench.WorkbenchSlotContext, Kind: workbench.RegionChat, Ratio: 0.38, Visible: true},
			},
			Mobile: workbench.MobileSpec{ActiveRegionID: mobileActive},
		},
		NormalRegions: []workbench.RegionNormalState{
			{SignalKey: filesRegionID, Available: true, Visible: true},
			{SignalKey: chatRegionID, Available: true, Visible: true},
		},
	})
	return wb
}

func EmptyRegion(message string) templ.Component { return emptyRegion(message) }

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
