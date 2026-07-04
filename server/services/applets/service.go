package applets

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CoreyCole/vamos/server/layouts"
	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/CoreyCole/vamos/server/services/appletruntime"
	"github.com/CoreyCole/vamos/server/services/commentui"
	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"
)

const (
	filesAppSuffix = "files-app"
	chatSuffix     = "chat"
)

var reservedAliasPrefixes = []string{"/api", "/forms", "/thoughts", "/agent-chat", "/static"}

type Service struct {
	resolver Resolver
	manager  appletruntime.Manager
	aliases  *appletruntime.AliasRegistry
	comments AppletCommentReader

	aliasMu     sync.Mutex
	aliasRoutes map[string]struct{}
}

type ServiceOptions struct {
	Resolver Resolver
	Manager  appletruntime.Manager
	Aliases  *appletruntime.AliasRegistry
	Comments AppletCommentReader
}

func NewService() *Service { return NewHTTPService(ServiceOptions{}) }

func NewHTTPService(opts ServiceOptions) *Service {
	aliases := opts.Aliases
	if aliases == nil {
		aliases = appletruntime.NewAliasRegistry(reservedAliasPrefixes)
	}
	return &Service{resolver: opts.Resolver, manager: opts.Manager, aliases: aliases, comments: opts.Comments, aliasRoutes: map[string]struct{}{}}
}

func (s *Service) RegisterThoughtsRoutes(g *echo.Group) {
	g.GET("/_render/app/:token", s.HandleAppletPage)
	g.GET("/_render/app/:token/status", s.HandleAppletStatus)
	g.OPTIONS("/_render/app/:token/app", s.HandleAppletOptions)
	g.OPTIONS("/_render/app/:token/app/*", s.HandleAppletOptions)
	g.Any("/_render/app/:token/app", s.HandleAppletProxy)
	g.Any("/_render/app/:token/app/*", s.HandleAppletProxy)
}

func (s *Service) RegisterExampleRoutes(e *echo.Echo, auth echo.MiddlewareFunc) {
	// Sandboxed applets have origin "null", so Datastar performs CORS
	// preflights without auth cookies. Answer app-route OPTIONS outside the
	// authenticated group; authenticated GET/POST still go through the group.
	e.OPTIONS("/examples/:id/app", s.HandleAppletOptions)
	e.OPTIONS("/examples/:id/app/*", s.HandleAppletOptions)
	e.Any("/examples/:id/app", s.HandleAppletProxy)
	e.Any("/examples/:id/app/*", s.HandleAppletProxy)

	group := e.Group("/examples")
	if auth != nil {
		group.Use(auth)
	}
	group.GET("/:id", s.HandleAppletPage)
	group.GET("/:id/status", s.HandleAppletStatus)
}

func (s *Service) RegisterFormRoutes(g *echo.Group) {
	g.POST("/applets/:token/stop", s.HandleAppletStop)
	g.POST("/applets/:token/restart", s.HandleAppletRestart)
}

func (s *Service) RegisterStartupAliases(e *echo.Echo) error {
	if err := s.registerExampleStartupAliases(e); err != nil {
		return err
	}
	return s.registerThoughtsStartupAliases(e)
}

func (s *Service) registerExampleStartupAliases(e *echo.Echo) error {
	root := strings.TrimSpace(s.resolver.ExamplesRoot)
	if root == "" {
		return nil
	}
	manifests, err := filepath.Glob(filepath.Join(root, "*", "AGENTS.md"))
	if err != nil {
		return err
	}
	for _, manifestPath := range manifests {
		id := filepath.Base(filepath.Dir(manifestPath))
		ctx, err := s.resolver.ResolveExampleApplet(nil, id)
		if err != nil || len(ctx.Manifest.RootAliases) == 0 {
			continue
		}
		if err := s.registerAppletAliasesDuringStartup(e, ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) registerThoughtsStartupAliases(e *echo.Echo) error {
	root := strings.TrimSpace(s.resolver.ThoughtsRoot)
	if root == "" {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() || d.Name() != "AGENTS.md" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		ctx, err := s.resolver.ResolveThoughtsApplet(nil, "thoughts/"+filepath.ToSlash(rel))
		if err != nil || len(ctx.Manifest.RootAliases) == 0 {
			return nil
		}
		return s.registerAppletAliasesDuringStartup(e, ctx)
	})
}

func (s *Service) registerAppletAliasesDuringStartup(e *echo.Echo, ctx AppletContext) error {
	reg := aliasRegistration(ctx)
	if len(reg.Aliases) == 0 {
		return nil
	}
	s.aliasMu.Lock()
	defer s.aliasMu.Unlock()
	if err := s.aliases.Register(reg); err != nil {
		return err
	}
	for _, alias := range reg.Aliases {
		s.registerAliasRouteLocked(e, alias)
	}
	return nil
}

func (s *Service) registerAliasRouteLocked(e *echo.Echo, alias appletruntime.RouteAlias) {
	route := echoRoute(alias.Pattern)
	if route == "" {
		return
	}
	if _, ok := s.aliasRoutes[route]; ok {
		return
	}
	s.aliasRoutes[route] = struct{}{}
	e.OPTIONS(route, s.HandleAppletOptions)
	e.Any(route, s.HandleAlias)
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
		s.EnsureAppletAsync(c.Request().Context(), applet, process)
	}
	return s.RenderAppletPage(c, applet, process)
}

func (s *Service) RenderAppletPage(c echo.Context, applet AppletContext, process appletruntime.AppletProcessState) error {
	userEmail := appletUserEmail(c)
	commentsPanel := EmptyRegion("Comments will appear here.")
	var commentUI commentui.CommentableMarkdownArgs
	if ui, ok, err := AppletCommentUI(c.Request().Context(), applet, userEmail, s.comments); err != nil {
		c.Logger().Warnf("Failed to load applet comments for %s: %v", applet.IdentityPath, err)
	} else if ok {
		commentUI = ui
		commentsPanel = commentui.CommentsContextPanel(commentui.BuildCommentsPanelArgs(ui, ""))
	}
	state, err := BuildAppletWorkbenchState(AppletWorkbenchInput{
		UserEmail: userEmail,
		Context:   applet,
		Process:   process,
		CommentUI: commentUI,
		Sidebar: workbench.WorkbenchSidebarArgs{
			Tabs:  workbench.DefaultSidebarTabs(),
			Files: workbench.FilesPanelModel{CurrentPath: applet.IdentityPath},
			Workspaces: workbench.WorkspacesPanelModel{
				EmptyLabel: "Workspaces will appear here.",
			},
		},
		RightRail: workbench.RightRailArgs{
			Chat:     EmptyRegion("Chat will appear here."),
			Comments: commentsPanel,
		},
	})
	if err != nil {
		return err
	}
	rootArgs := layouts.RootArgs{
		Title:       applet.Manifest.Title + " Applet",
		CurrentPath: applet.IdentityPath,
		PageType:    layouts.PageTypeMarkdown,
		ShowHeader:  true,
		UserEmail:   userEmail,
	}
	return layouts.Root(rootArgs).Render(
		templ.WithChildren(c.Request().Context(), workbench.Workbench(state)),
		c.Response().Writer,
	)
}

func (s *Service) HandleAppletStatus(c echo.Context) error {
	applet, err := s.resolveAppletFromRequest(c)
	if err != nil {
		return err
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	triggeredStart := false
	var lastStatus appletruntime.ProcessStatus
	for {
		process := s.currentProcess(c.Request().Context(), applet)
		if !triggeredStart && (shouldDemandStartFromStatus(process) || shouldRestartFromStatus(process)) {
			triggeredStart = true
			s.EnsureAppletAsync(c.Request().Context(), applet, process)
		}
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

func (s *Service) HandleAppletOptions(c echo.Context) error {
	setAppletCORS(c.Response().Header())
	c.Response().Before(func() { setAppletCORS(c.Response().Header()) })
	return c.NoContent(http.StatusNoContent)
}

func (s *Service) HandleAppletProxy(c echo.Context) error {
	setAppletCORS(c.Response().Header())
	c.Response().Before(func() { setAppletCORS(c.Response().Header()) })
	if c.Request().Method == http.MethodOptions {
		return c.NoContent(http.StatusNoContent)
	}
	applet, err := s.resolveAppletFromRequest(c)
	if err != nil {
		return err
	}
	if err := s.ensure(c, applet); err != nil {
		return err
	}
	match := appletruntime.AppletProxyMatch{
		AppID:            applet.RuntimeKey,
		StripPrefix:      strings.TrimRight(applet.IFrameSrc, "/"),
		AliasCookiePaths: rootAliasCookiePaths(applet),
	}
	proxy := appletruntime.NewAppletProxy(s.manager, match, proxyOptions())
	proxy.ServeHTTP(c.Response(), c.Request())
	return nil
}

func (s *Service) HandleAlias(c echo.Context) error {
	setAppletCORS(c.Response().Header())
	c.Response().Before(func() { setAppletCORS(c.Response().Header()) })
	if c.Request().Method == http.MethodOptions {
		return c.NoContent(http.StatusNoContent)
	}
	match, ok := s.aliases.Match(c.Request())
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown applet alias")
	}
	applet, err := s.resolveAppletFromRuntimeKey(c.Request().Context(), match.AppID)
	if err != nil {
		return err
	}
	if err := s.ensure(c, applet); err != nil {
		return err
	}
	proxy := appletruntime.NewAppletProxy(s.manager, match, proxyOptions())
	proxy.ServeHTTP(c.Response(), c.Request())
	return nil
}

func (s *Service) HandleAppletStop(c echo.Context) error {
	if s.manager == nil {
		return echo.NewHTTPError(http.StatusBadGateway, "applet runtime is unavailable")
	}
	applet, err := s.resolveAppletFromRequest(c)
	if err != nil {
		return err
	}
	if err := s.manager.Stop(c.Request().Context(), applet.RuntimeKey); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return c.Redirect(http.StatusSeeOther, applet.RouteHref)
}

func (s *Service) HandleAppletRestart(c echo.Context) error {
	if s.manager == nil {
		return echo.NewHTTPError(http.StatusBadGateway, "applet runtime is unavailable")
	}
	applet, err := s.resolveAppletFromRequest(c)
	if err != nil {
		return err
	}
	_ = s.manager.Stop(c.Request().Context(), applet.RuntimeKey)
	if _, err := s.manager.EnsureStarted(c.Request().Context(), RuntimeConfigFromManifest(applet)); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return c.Redirect(http.StatusSeeOther, applet.RouteHref)
}

func shouldDemandStartFromStatus(process appletruntime.AppletProcessState) bool {
	return process.Status == "" || process.Status == appletruntime.ProcessStatusStopped
}

func shouldRestartFromStatus(process appletruntime.AppletProcessState) bool {
	return process.Status == appletruntime.ProcessStatusUnhealthy
}

func (s *Service) EnsureAppletAsync(ctx context.Context, applet AppletContext, process appletruntime.AppletProcessState) {
	if s.manager == nil {
		return
	}
	go func() {
		runCtx := context.WithoutCancel(ctx)
		if shouldRestartFromStatus(process) {
			_ = s.manager.Stop(runCtx, applet.RuntimeKey)
		}
		_, _ = s.manager.EnsureStarted(runCtx, RuntimeConfigFromManifest(applet))
	}()
}

func (s *Service) resolveAppletFromRequest(c echo.Context) (AppletContext, error) {
	if token := strings.TrimSpace(c.Param("token")); token != "" {
		if s.resolver.ThoughtsRoot != "" {
			identity, err := DecodeAppletIdentity(token)
			if err == nil && strings.HasPrefix(identity, "thoughts/") {
				return s.resolver.ResolveThoughtsApplet(c.Request().Context(), identity)
			}
		}
		return s.resolver.ResolveExampleApplet(c.Request().Context(), token)
	}
	if s.resolver.ThoughtsRoot != "" {
		if docPath := strings.TrimSpace(firstNonEmpty(c.QueryParam("doc_path"), c.QueryParam("path"))); docPath != "" {
			return s.resolver.ResolveThoughtsApplet(c.Request().Context(), docPath)
		}
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return AppletContext{}, echo.NewHTTPError(http.StatusBadRequest, "applet identity is required")
	}
	return s.resolver.ResolveExampleApplet(c.Request().Context(), id)
}

func (s *Service) resolveAppletFromRuntimeKey(ctx context.Context, runtimeKey string) (AppletContext, error) {
	if identity, err := DecodeAppletIdentity(runtimeKey); err == nil && strings.HasPrefix(identity, "thoughts/") && s.resolver.ThoughtsRoot != "" {
		return s.resolver.ResolveThoughtsApplet(ctx, identity)
	}
	return s.resolver.ResolveExampleApplet(ctx, runtimeKey)
}

func appletUserEmail(c echo.Context) string {
	if value, ok := c.Get("user_email").(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func (s *Service) currentProcess(ctx context.Context, applet AppletContext) appletruntime.AppletProcessState {
	stopped := appletruntime.AppletProcessState{AppID: applet.RuntimeKey, Status: appletruntime.ProcessStatusStopped}
	if s.manager == nil {
		return stopped
	}
	state, err := s.manager.Health(ctx, applet.RuntimeKey)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return stopped
		}
		state.AppID = applet.RuntimeKey
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

func (s *Service) ensure(c echo.Context, applet AppletContext) error {
	if s.manager == nil {
		return echo.NewHTTPError(http.StatusBadGateway, "applet runtime is unavailable")
	}
	ctx := c.Request().Context()
	process := s.currentProcess(ctx, applet)
	if shouldRestartFromStatus(process) {
		_ = s.manager.Stop(ctx, applet.RuntimeKey)
	}
	if _, err := s.manager.EnsureStarted(ctx, RuntimeConfigFromManifest(applet)); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return nil
}

func proxyOptions() appletruntime.ProxyOptions {
	return appletruntime.ProxyOptions{FlushSSE: true, RewriteCookiePath: true, AllowNullOriginCORS: true}
}

func setAppletCORS(header http.Header) {
	header.Set("Access-Control-Allow-Origin", "null")
	header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	header.Set("Access-Control-Allow-Headers", "Content-Type, Accept, Datastar-Request")
	header.Set("Access-Control-Max-Age", "600")
}

func aliasRegistration(ctx AppletContext) appletruntime.AliasRegistration {
	aliases := make([]appletruntime.RouteAlias, 0, len(ctx.Manifest.RootAliases))
	for _, alias := range ctx.Manifest.RootAliases {
		aliases = append(aliases, appletruntime.RouteAlias{Pattern: alias.Pattern, Methods: alias.Methods})
	}
	return appletruntime.AliasRegistration{AppID: ctx.RuntimeKey, Aliases: aliases}
}

func rootAliasCookiePaths(ctx AppletContext) []string {
	return appletruntime.RootAliasCookiePaths(aliasRegistration(ctx).Aliases)
}

func echoRoute(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "/" || pattern == "/*" {
		return ""
	}
	if strings.HasSuffix(pattern, "/*") {
		return strings.TrimSuffix(pattern, "/*") + "/*"
	}
	return pattern
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
