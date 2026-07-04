package applets

import (
	"fmt"
	"strings"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/CoreyCole/vamos/server/services/appletruntime"
	"github.com/CoreyCole/vamos/server/services/commentui"
)

type AppletWorkbenchInput struct {
	UserEmail     string
	Context       AppletContext
	Process       appletruntime.AppletProcessState
	CommentUI     commentui.CommentableMarkdownArgs
	Sidebar       workbench.WorkbenchSidebarArgs
	RightRail     workbench.RightRailArgs
	SavedConfig   *workbench.WorkbenchConfig
	ViewportClass workbench.ViewportClass
}

func BuildAppletWorkbenchState(input AppletWorkbenchInput) (workbench.WorkbenchState, error) {
	if strings.TrimSpace(input.Context.IdentityPath) == "" {
		return workbench.WorkbenchState{}, fmt.Errorf("applet identity path is required")
	}
	if strings.TrimSpace(input.Context.RouteHref) == "" {
		return workbench.WorkbenchState{}, fmt.Errorf("applet route href is required")
	}

	rightRail := input.RightRail
	if rightRail.Chat == nil {
		rightRail.Chat = EmptyRegion("Chat will appear here.")
	}
	if rightRail.Comments == nil {
		rightRail.Comments = EmptyRegion("Comments will appear here.")
	}

	return workbench.BuildDocWorkbenchState(workbench.WorkbenchDocContext{
		EntryMode:          workbench.DocEntryModeThoughts,
		UserEmail:          input.UserEmail,
		SelectedPath:       input.Context.IdentityPath,
		RouteHref:          input.Context.RouteHref,
		ViewportClass:      input.ViewportClass,
		SavedConfig:        input.SavedConfig,
		Sidebar:            input.Sidebar,
		InitialSidebarOpen: false,
		InitialRailOpen:    false,
		Center: workbench.CenterDocPaneArgs{
			Title:    input.Context.Manifest.Title,
			Actions:  BuildAppletWorkbenchActions(input.Context, input.Process),
			Document: AppletWorkbenchDocument(input.Context, input.Process, input.CommentUI),
		},
		RightRail: rightRail,
	})
}
