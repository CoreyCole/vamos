package applets

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/CoreyCole/vamos/server/services/appletruntime"
	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"
)

const (
	filesAppSuffix = "files-app"
	chatSuffix     = "chat"
)

type Service struct {
	resolver Resolver
	manager  appletruntime.Manager
}

type ServiceOptions struct {
	Resolver Resolver
	Manager  appletruntime.Manager
}

func NewService() *Service { return &Service{} }

func NewHTTPService(opts ServiceOptions) *Service {
	return &Service{resolver: opts.Resolver, manager: opts.Manager}
}

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

func (s *Service) HandleAppletPage(c echo.Context) error {
	applet, err := s.resolveAppletFromRequest(c)
	if err != nil {
		return err
	}
	process := s.currentProcess(c.Request().Context(), applet)
	if process.Status != appletruntime.ProcessStatusHealthy {
		s.EnsureAppletAsync(c.Request().Context(), applet)
	}
	return s.RenderAppletPage(c, applet, process)
}

func (s *Service) RenderAppletPage(c echo.Context, applet AppletContext, process appletruntime.AppletProcessState) error {
	state, err := BuildAppletWorkbenchState(AppletWorkbenchInput{
		UserEmail: appletUserEmail(c),
		Context:   applet,
		Process:   process,
		Sidebar: workbench.WorkbenchSidebarArgs{
			Tabs:  workbench.DefaultSidebarTabs(),
			Files: workbench.FilesPanelModel{CurrentPath: applet.IdentityPath},
			Workspaces: workbench.WorkspacesPanelModel{
				EmptyLabel: "Workspaces will appear here.",
			},
		},
		RightRail: workbench.RightRailArgs{
			Chat:     EmptyRegion("Chat will appear here."),
			Comments: EmptyRegion("Comments will appear here."),
		},
	})
	if err != nil {
		return err
	}
	return workbench.Workbench(state).Render(c.Request().Context(), c.Response().Writer)
}

func (s *Service) HandleAppletStatus(c echo.Context) error {
	applet, err := s.resolveAppletFromRequest(c)
	if err != nil {
		return err
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	var lastStatus appletruntime.ProcessStatus
	for {
		process := s.currentProcess(c.Request().Context(), applet)
		if process.Status != lastStatus {
			if err := sse.PatchElementTempl(
				AppletStatusFragment(applet, process),
				datastar.WithSelectorID("applet-frame-"+applet.Manifest.ID),
				datastar.WithModeOuter(),
			); err != nil {
				return err
			}
			lastStatus = process.Status
		}
		if process.Status == appletruntime.ProcessStatusHealthy {
			return sse.ExecuteScript("window.location.reload();")
		}
		select {
		case <-c.Request().Context().Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *Service) EnsureAppletAsync(ctx context.Context, applet AppletContext) {
	if s.manager == nil {
		return
	}
	go func() {
		_, _ = s.manager.EnsureStarted(context.WithoutCancel(ctx), RuntimeConfigFromManifest(applet))
	}()
}

func (s *Service) resolveAppletFromRequest(c echo.Context) (AppletContext, error) {
	if s.resolver.ThoughtsRoot != "" {
		if docPath := strings.TrimSpace(firstNonEmpty(c.QueryParam("doc_path"), c.QueryParam("path"))); docPath != "" {
			return s.resolver.ResolveThoughtsApplet(c.Request().Context(), docPath)
		}
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return AppletContext{}, echo.NewHTTPError(http.StatusBadRequest, "applet id is required")
	}
	return s.resolver.ResolveExampleApplet(c.Request().Context(), id)
}

func appletUserEmail(c echo.Context) string {
	if value, ok := c.Get("user_email").(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func (s *Service) currentProcess(ctx context.Context, applet AppletContext) appletruntime.AppletProcessState {
	stopped := appletruntime.AppletProcessState{AppID: applet.Manifest.ID, Status: appletruntime.ProcessStatusStopped}
	if s.manager == nil {
		return stopped
	}
	state, err := s.manager.Health(ctx, applet.Manifest.ID)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return stopped
		}
		state.AppID = applet.Manifest.ID
		if state.Status == "" {
			state.Status = appletruntime.ProcessStatusStopped
		}
		return state
	}
	if state.Status == "" {
		if state.Healthy {
			state.Status = appletruntime.ProcessStatusHealthy
		} else {
			state.Status = appletruntime.ProcessStatusStopped
		}
	}
	return state
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
