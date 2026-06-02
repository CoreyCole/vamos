package workspaces

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/layouts"
	"github.com/CoreyCole/vamos/server/services/auth"
)

type SessionCreator interface {
	ValidateEmail(email string) error
	CreateSession(ctx context.Context, email string) (*auth.Session, error)
	LogAuthAttempt(
		ctx context.Context,
		email string,
		success bool,
		errorMessage string,
	) error
}

type PlanWorkspaceLister interface {
	ListCurrentPlanWorkspaces(ctx context.Context, projectID string) ([]db.PlanWorkspace, error)
	ListPlanWorkspaceProjects(ctx context.Context, planDirRel string) ([]db.PlanWorkspaceProject, error)
	ListPlanWorkspaceImplBindings(ctx context.Context, planDirRel string) ([]db.PlanWorkspaceImplBinding, error)
}

type ImplWorkspaceLister interface {
	ListImplWorkspaces(ctx context.Context, projectID string) ([]db.ImplWorkspace, error)
}

const (
	workspaceSyncRefreshTimeout = 5 * time.Minute
	workspaceErrorScanTimeout   = 5 * time.Minute
)

type WorkspaceSyncRefreshFunc func(ctx context.Context) (WorkspaceSyncRefreshResult, error)

type WorkspaceSyncRefreshResult struct {
	PlanUpserted           int
	PlanArchived           int
	ImplUpserted           int
	ImplRepairedEnv        int
	ImportedPiSessions     int
	AdoptedQRSPIWorkspaces int
	ImplCleanedUp          int
	ImplMerged             int
	Changed                bool
}

type WorkspaceRefreshState struct {
	InFlight    bool
	LastResult  WorkspaceSyncRefreshResult
	LastError   string
	CompletedAt time.Time
}

type WorkspaceSyncCompletionFunc func(ctx context.Context, result WorkspaceSyncRefreshResult, err error) WorkspaceSyncRefreshResult

type TerminalSessionAdopter interface {
	ImportAdoptablePiSessions(ctx context.Context) (TerminalSessionAdoptionResult, error)
}

type TerminalSessionAdoptionResult struct {
	ImportedSessions       int
	AdoptedQRSPIWorkspaces int
	Changed                bool
}

func NewWorkspaceSyncCompletion(
	manager Registry,
	workspaceNotifier WorkspaceLifecycleNotifier,
	agentChatNotifier func(),
	adopter TerminalSessionAdopter,
) WorkspaceSyncCompletionFunc {
	return func(ctx context.Context, result WorkspaceSyncRefreshResult, err error) WorkspaceSyncRefreshResult {
		if err == nil {
			if manager != nil {
				if refreshErr := manager.Refresh(ctx); refreshErr != nil {
					log.Printf("workspace_manager_refresh_after_sync_failed: %v", refreshErr)
				}
			}
			if adopter != nil {
				adoption, adoptionErr := adopter.ImportAdoptablePiSessions(ctx)
				if adoptionErr != nil {
					log.Printf("terminal_session_adoption_after_sync_failed: %v", adoptionErr)
				} else {
					result.ImportedPiSessions += adoption.ImportedSessions
					result.AdoptedQRSPIWorkspaces += adoption.AdoptedQRSPIWorkspaces
					result.Changed = result.Changed || adoption.Changed
				}
			}
			if agentChatNotifier != nil {
				agentChatNotifier()
			}
		}
		if workspaceNotifier != nil {
			workspaceNotifier.Notify("workspaces-refresh")
		}
		return result
	}
}

type Handler struct {
	manager                    Manager
	lifecycle                  LifecycleManager
	planWorkspaces             PlanWorkspaceLister
	implWorkspaces             ImplWorkspaceLister
	workspaceSyncRefresh       WorkspaceSyncRefreshFunc
	workspaceSyncComplete      WorkspaceSyncCompletionFunc
	workflowSummaries          WorkspaceWorkflowSummaryResolver
	refreshStateMu             sync.Mutex
	refreshStateValue          WorkspaceRefreshState
	notifier                   WorkspaceLifecycleNotifier
	managerURL                 string
	currentSlug                string
	authService                SessionCreator
	signer                     *HandoffSigner
	restartToken               string
	mainCheckoutPath           string
	verifier                   *Verifier
	provisionStarter           WorkspaceProvisionStarter
	releaseProjector           *ReleaseProjector
	releaseStore               ReleaseQueueStore
	releaseStarter             ReleaseWorkflowStarter
	cleanupStarter             WorkspaceCleanupStarter
	workspaceErrorRecorder     *WorkspaceErrorRecorder
	workspaceErrorScanner      *WorkspaceErrorScanner
	workspaceErrorScanMu       sync.Mutex
	workspaceErrorScanInFlight map[string]bool
	exitFunc                   func(int)
}
type RestartRequest struct {
	Slug         string            `json:"slug"`
	CheckoutPath string            `json:"checkout_path"`
	Components   []BundleComponent `json:"components"`
	Force        bool              `json:"force,omitempty"`
}

type RestartComponentsOptions struct {
	Force bool
}

type componentRestarter interface {
	RestartComponents(
		ctx context.Context,
		slug string,
		components []BundleComponent,
		opts RestartComponentsOptions,
	) (Workspace, error)
}

type HandlerOption func(*Handler)

func WithDevAuth(authService SessionCreator, signer *HandoffSigner) HandlerOption {
	return func(h *Handler) {
		h.authService = authService
		h.signer = signer
	}
}

func WithRestartAPI(token, mainCheckoutPath string) HandlerOption {
	return func(h *Handler) {
		h.restartToken = strings.TrimSpace(token)
		h.mainCheckoutPath = strings.TrimSpace(mainCheckoutPath)
	}
}

func WithExitFunc(exitFunc func(int)) HandlerOption {
	return func(h *Handler) {
		if exitFunc != nil {
			h.exitFunc = exitFunc
		}
	}
}

func WithLifecycleNotifier(notifier WorkspaceLifecycleNotifier) HandlerOption {
	return func(h *Handler) {
		h.notifier = notifier
	}
}

func WithPlanWorkspaces(source PlanWorkspaceLister) HandlerOption {
	return func(h *Handler) {
		h.planWorkspaces = source
	}
}

func WithImplWorkspaces(source ImplWorkspaceLister) HandlerOption {
	return func(h *Handler) {
		h.implWorkspaces = source
	}
}

func WithWorkspaceSyncRefresh(refresh WorkspaceSyncRefreshFunc) HandlerOption {
	return func(h *Handler) {
		h.workspaceSyncRefresh = refresh
	}
}

func WithWorkspaceWorkflowSummaryResolver(resolver WorkspaceWorkflowSummaryResolver) HandlerOption {
	return func(h *Handler) {
		h.workflowSummaries = resolver
	}
}

func WithWorkspaceSyncCompletion(complete WorkspaceSyncCompletionFunc) HandlerOption {
	return func(h *Handler) {
		h.workspaceSyncComplete = complete
	}
}
func WithWorkspaceProvisionStarter(starter WorkspaceProvisionStarter) HandlerOption {
	return func(h *Handler) {
		h.provisionStarter = starter
	}
}

func WithReleaseProjector(projector *ReleaseProjector) HandlerOption {
	return func(h *Handler) {
		h.releaseProjector = projector
	}
}

func WithReleaseQueue(projector *ReleaseProjector, store ReleaseQueueStore, starter ReleaseWorkflowStarter) HandlerOption {
	return func(h *Handler) {
		h.releaseProjector = projector
		h.releaseStore = store
		h.releaseStarter = starter
	}
}

func WithWorkspaceCleanupStarter(starter WorkspaceCleanupStarter) HandlerOption {
	return func(h *Handler) {
		h.cleanupStarter = starter
	}
}

func WithWorkspaceErrorStore(store WorkspaceErrorEventStore) HandlerOption {
	return func(h *Handler) {
		h.workspaceErrorRecorder = &WorkspaceErrorRecorder{Store: store}
	}
}

func WithWorkspaceErrorScanner(scanner *WorkspaceErrorScanner) HandlerOption {
	return func(h *Handler) {
		h.workspaceErrorScanner = scanner
	}
}

func (h *Handler) triggerWorkspaceErrorScan(selected string) {
	if h.workspaceErrorScanner == nil {
		return
	}
	key := workspaceErrorScanKey(selected)
	h.workspaceErrorScanMu.Lock()
	if h.workspaceErrorScanInFlight == nil {
		h.workspaceErrorScanInFlight = map[string]bool{}
	}
	if h.workspaceErrorScanInFlight[key] {
		h.workspaceErrorScanMu.Unlock()
		return
	}
	h.workspaceErrorScanInFlight[key] = true
	h.workspaceErrorScanMu.Unlock()
	h.notifier.Notify(key)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), workspaceErrorScanTimeout)
		defer cancel()
		if err := h.workspaceErrorScanner.ScanSelectedThenAll(ctx, selected); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			log.Printf("workspace_error_scan_failed workspace=%q error=%v", selected, err)
		}
		h.workspaceErrorScanMu.Lock()
		delete(h.workspaceErrorScanInFlight, key)
		h.workspaceErrorScanMu.Unlock()
		h.notifier.Notify(key)
	}()
}

func (h *Handler) isWorkspaceErrorScanInFlight(selected string) bool {
	key := workspaceErrorScanKey(selected)
	h.workspaceErrorScanMu.Lock()
	defer h.workspaceErrorScanMu.Unlock()
	return h.workspaceErrorScanInFlight[key]
}

func workspaceErrorScanKey(selected string) string {
	key := strings.TrimSpace(selected)
	if key == "" {
		return "*"
	}
	return key
}

// Deprecated: use WithWorkspaceSyncRefresh.
func WithPlanWorkspaceRefresh(refresh WorkspaceSyncRefreshFunc) HandlerOption {
	return WithWorkspaceSyncRefresh(refresh)
}

func NewHandler(
	manager Manager,
	managerURL, currentSlug string,
	opts ...HandlerOption,
) *Handler {
	h := &Handler{
		manager:     manager,
		managerURL:  strings.TrimRight(strings.TrimSpace(managerURL), "/"),
		currentSlug: strings.TrimSpace(currentSlug),
		exitFunc:    os.Exit,
	}
	if lifecycle, ok := manager.(LifecycleManager); ok {
		h.lifecycle = lifecycle
	}
	for _, opt := range opts {
		opt(h)
	}
	if h.notifier == nil {
		h.notifier = NewLifecycleNotifier()
	}
	if h.workspaceErrorRecorder != nil && h.workspaceErrorRecorder.Notifier == nil {
		h.workspaceErrorRecorder.Notifier = h.notifier
	}
	if h.workspaceErrorScanner != nil {
		if h.workspaceErrorScanner.Notifier == nil {
			h.workspaceErrorScanner.Notifier = h.notifier
		}
		if h.workspaceErrorScanner.Manager == nil {
			h.workspaceErrorScanner.Manager = h.manager
		}
		if h.workspaceErrorRecorder != nil && h.workspaceErrorScanner.Store == nil {
			h.workspaceErrorScanner.Store = h.workspaceErrorRecorder.Store
		}
	}
	if manager, ok := h.manager.(*ManagerService); ok && manager != nil {
		manager.SetLifecycleNotifier(h.notifier)
		if h.workspaceErrorRecorder != nil {
			manager.SetWorkspaceErrorRecorder(h.workspaceErrorRecorder)
		}
	}
	return h
}

func (h *Handler) RegisterRoutes(e *echo.Echo, authMiddleware echo.MiddlewareFunc) {
	g := e.Group("/workspaces")
	g.Use(authMiddleware)
	g.GET("", h.HandleWorkspacesPage)
	g.GET("/stream", h.HandleWorkspacesStream)
	g.GET("/errors", h.HandleWorkspaceErrorsPage)
	g.GET("/errors/stream", h.HandleWorkspaceErrorsStream)
	g.POST("/refresh", h.HandleRefreshWorkspaces)
	g.GET("/switch/:slug", h.HandleSwitchWorkspace)
	g.GET("/:slug/verify-html", h.HandleVerifyHTML)
	g.POST("/:slug/start", h.HandleStart)
	g.POST("/:slug/stop", h.HandleStop)
	g.POST("/:slug/restart", h.HandleRestart)
	g.POST("/host-action", h.HandleWorkspaceHostAction)
	g.POST("/release/enqueue", h.HandleEnqueueRelease)
	g.POST("/cleanup", h.HandleCleanupWorkspace)
}

func (h *Handler) RegisterFixtureReadOnlyRoutes(e *echo.Echo, authMiddleware echo.MiddlewareFunc) {
	g := e.Group("/workspaces")
	g.Use(authMiddleware)
	g.GET("", h.HandleWorkspacesPage)
	g.GET("/stream", h.HandleWorkspacesStream)
}

func (h *Handler) RegisterDevAuthRoute(e *echo.Echo) {
	e.GET("/internal/dev-auth/handoff", h.HandleDevAuthHandoff)
}

func (h *Handler) RegisterInternalRestartRoute(e *echo.Echo) {
	e.POST("/internal/workspaces/restart", h.HandleInternalRestart)
}

func (h *Handler) RegisterInternalProvisionRoute(e *echo.Echo) {
	e.POST("/internal/workspaces/provision", h.HandleInternalProvisionWorkspace)
}

func (h *Handler) HandleWorkspacesPage(c echo.Context) error {
	filter := ProjectFilterFromRequest(c.Request())
	model, err := h.buildWorkspacesPageModel(c.Request().Context(), filter)
	if err != nil {
		return err
	}
	showCleanedHistory := showCleanedHistoryFromRequest(c.Request())
	renderedViews := h.renderedImplWorkspaceViews(model.Views, showCleanedHistory)
	groups := h.workspaceGroups(model.Views, showCleanedHistory)
	args := layouts.RootArgs{
		Title:       "Workspaces",
		CurrentPath: "/workspaces",
		PageType:    layouts.PageTypeWorkspaces,
		ShowHeader:  true,
		UserEmail:   userEmailFromContext(c),
		Workspaces: BuildNavItems(
			ImplViewsToNavWorkspaces(renderedViews),
			h.currentSlug,
			h.managerURL,
			"/workspaces",
			h.protectedReleaseSlugList()...,
		),
		CurrentWorkspaceSlug: h.currentSlug,
		WorkspaceManagerURL:  h.managerURL,
	}
	return render(
		c,
		http.StatusOK,
		WorkspacesDocument(args, groups, model.ReleasePanel, h.refreshState(), showCleanedHistory, filter, model.ProjectOptions),
	)
}

func (h *Handler) HandleVerifyHTML(c echo.Context) error {
	slug := strings.TrimSpace(c.Param("slug"))
	if slug == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "workspace slug is required")
	}
	views, err := h.listImplWorkspaceViews(c.Request().Context(), ProjectFilter{})
	if err != nil {
		return err
	}
	view, ok := findImplWorkspaceViewBySlug(views, slug)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "workspace not found")
	}
	path, ok := latestWorkspaceVerifyHTMLPath(view)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "workspace verify HTML not found")
	}
	return c.File(path)
}

func findImplWorkspaceViewBySlug(views []ImplWorkspaceView, slug string) (ImplWorkspaceView, bool) {
	for _, view := range views {
		if workspaceViewSlug(view) == slug {
			return view, true
		}
		if child, ok := findImplWorkspaceViewBySlug(view.Children, slug); ok {
			return child, true
		}
	}
	return ImplWorkspaceView{}, false
}

func (h *Handler) HandleRefreshWorkspaces(c echo.Context) error {
	showCleanedHistory := showCleanedHistoryFromRequest(c.Request())
	filter := ProjectFilterFromRequest(c.Request())
	if h.workspaceSyncRefresh == nil {
		return echo.NewHTTPError(
			http.StatusNotImplemented,
			"workspace sync refresh is not configured",
		)
	}
	started := h.tryStartWorkspaceSyncRefresh()
	if started {
		h.notifier.Notify("workspaces-refresh")
		go h.runWorkspaceSyncRefresh()
	}
	if isDatastarRequest(c.Request()) {
		sse := datastar.NewSSE(c.Response().Writer, c.Request())
		c.Response().WriteHeader(http.StatusAccepted)
		model, err := h.buildWorkspacesPageModel(c.Request().Context(), filter)
		if err != nil {
			return err
		}
		return sse.PatchElementTempl(
			WorkspacesHeader(h.refreshState(), showCleanedHistory, filter, model.ProjectOptions),
			datastar.WithSelectorID("workspaces-header"),
			datastar.WithModeOuter(),
		)
	}
	return c.Redirect(http.StatusSeeOther, workspacesURL("/workspaces", showCleanedHistory, filter))
}

func (h *Handler) tryStartWorkspaceSyncRefresh() bool {
	h.refreshStateMu.Lock()
	defer h.refreshStateMu.Unlock()
	if h.refreshStateValue.InFlight {
		return false
	}
	h.refreshStateValue.InFlight = true
	h.refreshStateValue.LastError = ""
	return true
}

func (h *Handler) refreshState() WorkspaceRefreshState {
	h.refreshStateMu.Lock()
	defer h.refreshStateMu.Unlock()
	return h.refreshStateValue
}

func (h *Handler) recordWorkspaceSyncRefresh(
	result WorkspaceSyncRefreshResult,
	err error,
	completedAt time.Time,
) {
	h.refreshStateMu.Lock()
	defer h.refreshStateMu.Unlock()
	h.refreshStateValue.InFlight = false
	h.refreshStateValue.LastResult = result
	h.refreshStateValue.CompletedAt = completedAt
	if err != nil {
		h.refreshStateValue.LastError = err.Error()
	} else {
		h.refreshStateValue.LastError = ""
	}
}

func (h *Handler) runWorkspaceSyncRefresh() {
	ctx, cancel := context.WithTimeout(context.Background(), workspaceSyncRefreshTimeout)
	defer cancel()
	result, err := h.workspaceSyncRefresh(ctx)
	if h.workspaceSyncComplete != nil {
		result = h.workspaceSyncComplete(ctx, result, err)
	} else if err == nil && h.manager != nil {
		if refreshErr := h.manager.Refresh(ctx); refreshErr != nil {
			log.Printf("workspace_manager_refresh_after_sync_failed: %v", refreshErr)
		}
	}
	if err != nil {
		log.Printf("workspace_sync_refresh_failed: %v", err)
	}
	h.recordWorkspaceSyncRefresh(result, err, time.Now())
	h.notifier.Notify("workspaces-refresh")
}

func (h *Handler) HandleStart(c echo.Context) error {
	return h.lifecycleAction(
		c,
		c.Param("slug"),
		WorkspaceTransitionStart,
		WorkspaceDesiredRunning,
	)
}

func (h *Handler) HandleStop(c echo.Context) error {
	return h.lifecycleAction(
		c,
		c.Param("slug"),
		WorkspaceTransitionStop,
		WorkspaceDesiredStopped,
	)
}

func (h *Handler) HandleRestart(c echo.Context) error {
	return h.lifecycleAction(
		c,
		c.Param("slug"),
		WorkspaceTransitionRestart,
		WorkspaceDesiredRunning,
	)
}

func (h *Handler) lifecycleAction(
	c echo.Context,
	slug string,
	kind WorkspaceTransitionKind,
	desired WorkspaceDesiredState,
) error {
	if h.lifecycle == nil {
		return echo.NewHTTPError(
			http.StatusNotImplemented,
			"workspace lifecycle manager is not configured",
		)
	}
	snap, err := h.lifecycle.RequestLifecycle(
		c.Request().Context(),
		WorkspaceLifecycleRequest{Slug: slug, Kind: kind, DesiredState: desired},
	)
	if err != nil {
		return err
	}
	if isDatastarRequest(c.Request()) {
		filter := ProjectFilterFromRequest(c.Request())
		views, err := h.listImplWorkspaceViews(c.Request().Context(), filter)
		if err != nil {
			views = lifecycleSnapshotsToImplViews(filterLifecycleSnapshotsByProject([]WorkspaceLifecycleSnapshot{snap}, filter))
		}
		return h.patchWorkspaces(c, views)
	}
	return c.JSON(http.StatusAccepted, snap)
}

func (h *Handler) HandleWorkspacesStream(c echo.Context) error {
	if h.lifecycle == nil {
		return echo.NewHTTPError(
			http.StatusNotImplemented,
			"workspace lifecycle manager is not configured",
		)
	}
	showCleanedHistory := showCleanedHistoryFromRequest(c.Request())
	filter := ProjectFilterFromRequest(c.Request())
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	send := func() error {
		model, err := h.buildWorkspacesPageModel(c.Request().Context(), filter)
		if err != nil {
			return err
		}
		groups := h.workspaceGroups(model.Views, showCleanedHistory)
		if err := sse.PatchElementTempl(
			WorkspacesHeader(h.refreshState(), showCleanedHistory, filter, model.ProjectOptions),
			datastar.WithSelectorID("workspaces-header"),
			datastar.WithModeOuter(),
		); err != nil {
			return err
		}
		if err := sse.PatchElementTempl(
			ReleasePanel(model.ReleasePanel),
			datastar.WithSelectorID("release-queue-panel"),
			datastar.WithModeOuter(),
		); err != nil {
			return err
		}
		return sse.PatchElementTempl(
			WorkspacesList(groups, h.managerURL, showCleanedHistory, filter),
			datastar.WithSelectorID("workspaces-list"),
			datastar.WithModeOuter(),
		)
	}
	if err := send(); err != nil {
		return err
	}
	ch, unsubscribe := h.notifier.Subscribe()
	defer unsubscribe()
	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case <-ch:
			if err := send(); err != nil {
				return err
			}
		}
	}
}

func (h *Handler) HandleWorkspaceHostAction(c echo.Context) error {
	if h.manager == nil {
		return echo.NewHTTPError(
			http.StatusNotFound,
			"workspace manager is not configured",
		)
	}
	slug := strings.TrimSpace(c.FormValue("slug"))
	if slug == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing workspace slug")
	}
	returnTo := strings.TrimSpace(c.FormValue("return_to"))
	if returnTo == "" {
		returnTo = "/workspaces"
	}
	if _, err := ValidateLocalRedirectPath(returnTo); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	switch strings.TrimSpace(c.FormValue("action")) {
	case "start", "retry":
		if _, err := h.manager.Start(c.Request().Context(), slug); err != nil {
			return err
		}
	case "stop":
		if _, err := h.manager.Stop(c.Request().Context(), slug); err != nil {
			return err
		}
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "unsupported workspace action")
	}
	return c.Redirect(http.StatusSeeOther, returnTo)
}

func (h *Handler) HandleInternalProvisionWorkspace(c echo.Context) error {
	if err := h.requireRestartToken(c); err != nil {
		return err
	}
	if h.provisionStarter == nil {
		return echo.NewHTTPError(
			http.StatusNotFound,
			"workspace provision starter is not configured",
		)
	}
	var input WorkspaceProvisionInput
	if err := c.Bind(&input); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	result, err := h.provisionStarter.StartProvision(c.Request().Context(), input)
	if err != nil {
		return err
	}
	status := http.StatusAccepted
	if result.Status == WorkspaceProvisionStatusBlocked {
		status = http.StatusConflict
	}
	return c.JSON(status, result)
}

func (h *Handler) HandleInternalRestart(c echo.Context) error {
	if err := h.requireRestartToken(c); err != nil {
		return err
	}
	var req RestartRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.CheckoutPath) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing checkout_path")
	}
	if strings.TrimSpace(req.Slug) == "" {
		meta, err := ReadMetadata(WorkspaceMetadataPath(req.CheckoutPath))
		if err == nil {
			req.Slug = meta.Slug
		}
	}
	if samePath(req.CheckoutPath, h.mainCheckoutPath) {
		if strings.TrimSpace(req.Slug) != "" && req.Slug != "main" {
			return echo.NewHTTPError(
				http.StatusBadRequest,
				"workspace restart slug/checkout mismatch",
			)
		}
		go func() {
			time.Sleep(250 * time.Millisecond)
			h.exitFunc(0)
		}()
		return c.JSON(http.StatusAccepted, map[string]string{"status": "main-restarting"})
	}
	if req.Slug == "main" {
		return echo.NewHTTPError(
			http.StatusBadRequest,
			"workspace restart slug/checkout mismatch",
		)
	}
	if h.manager == nil {
		return echo.NewHTTPError(
			http.StatusNotFound,
			"workspace manager is not configured",
		)
	}
	restarter, ok := h.manager.(componentRestarter)
	if !ok {
		return echo.NewHTTPError(
			http.StatusNotFound,
			"workspace component restart is not configured",
		)
	}
	ws, ok := h.resolveRestartWorkspace(c.Request().Context(), req)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown workspace")
	}
	if strings.TrimSpace(req.CheckoutPath) != "" &&
		!samePath(ws.CheckoutPath, req.CheckoutPath) {
		return echo.NewHTTPError(
			http.StatusBadRequest,
			"workspace restart slug/checkout mismatch",
		)
	}
	restartSlug := ws.Slug
	ws, err := restarter.RestartComponents(
		c.Request().Context(),
		restartSlug,
		req.Components,
		RestartComponentsOptions{Force: req.Force},
	)
	if err != nil {
		if !req.Force && (ws.Status == StatusFailed || ws.Status == StatusCrashed ||
			ws.Status == StatusStopped) {
			started, startErr := h.manager.Start(c.Request().Context(), req.Slug)
			if startErr != nil {
				return err
			}
			return c.JSON(http.StatusAccepted, started)
		}
		return err
	}
	return c.JSON(http.StatusAccepted, ws)
}

func (h *Handler) resolveRestartWorkspace(
	ctx context.Context,
	req RestartRequest,
) (Workspace, bool) {
	slug := strings.TrimSpace(req.Slug)
	if slug != "" {
		if ws, ok := h.manager.Lookup(slug); ok {
			if strings.TrimSpace(req.CheckoutPath) == "" ||
				samePath(ws.CheckoutPath, req.CheckoutPath) {
				return ws, true
			}
		}
	}
	checkoutPath := strings.TrimSpace(req.CheckoutPath)
	if checkoutPath == "" {
		return Workspace{}, false
	}
	_ = h.manager.Refresh(ctx)
	for _, ws := range h.manager.List() {
		if samePath(ws.CheckoutPath, checkoutPath) {
			return ws, true
		}
	}
	return Workspace{}, false
}

func (h *Handler) HandleSwitchWorkspace(c echo.Context) error {
	if h.manager == nil || h.signer == nil {
		return echo.NewHTTPError(
			http.StatusNotFound,
			"workspace switching is not configured",
		)
	}
	slug := c.Param("slug")
	redirectPath, err := switchRedirectPathForTarget(c.QueryParam("redirect"), h.currentSlug, slug)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	email := userEmailFromContext(c)
	if email == "" {
		return echo.NewHTTPError(http.StatusUnauthorized)
	}
	ws, ok := h.manager.Lookup(slug)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown workspace")
	}
	if ws.Status != StatusRunning && h.isProtectedReleaseWorkspace(slug) && strings.TrimSpace(ws.URL) != "" {
		started, err := h.ensureWorkspaceRunningForSwitch(c.Request().Context(), ws)
		if err != nil {
			log.Printf("workspace_switch_start_failed slug=%q error=%v", slug, err)
			if latest, ok := h.manager.Lookup(slug); ok {
				ws = latest
				if strings.TrimSpace(ws.URL) == "" {
					ws.URL = started.URL
				}
			}
		}
		if ws.Status != StatusRunning && started.Status != "" {
			ws = started
		}
	}
	if ws.Status != StatusRunning || strings.TrimSpace(ws.URL) == "" {
		log.Printf(
			"workspace_switch_unavailable slug=%q status=%q url=%q redirect=%q",
			slug,
			ws.Status,
			ws.URL,
			redirectPath,
		)
		if h.workspaceErrorRecorder != nil {
			if err := h.workspaceErrorRecorder.RecordSwitchUnavailable(c.Request().Context(), ws, redirectPath); err != nil {
				log.Printf("workspace_switch_error_record_failed slug=%q error=%v", slug, err)
			}
		}
		return c.Redirect(http.StatusSeeOther, "/workspaces/errors?workspace="+url.QueryEscape(slug))
	}
	token, err := h.signer.Sign(HandoffClaims{
		Email:        email,
		TargetSlug:   slug,
		RedirectPath: redirectPath,
	})
	if err != nil {
		return err
	}
	target := strings.TrimRight(
		ws.URL,
		"/",
	) + "/internal/dev-auth/handoff?token=" + url.QueryEscape(
		token,
	)
	log.Printf(
		"workspace_switch_redirect slug=%q status=%q pid=%d port=%d url=%q redirect=%q user=%q",
		slug,
		ws.Status,
		ws.PID,
		ws.Port,
		ws.URL,
		redirectPath,
		email,
	)
	return c.Redirect(http.StatusFound, target)
}

func (h *Handler) ensureWorkspaceRunningForSwitch(ctx context.Context, ws Workspace) (Workspace, error) {
	started, err := h.manager.Start(ctx, ws.Slug)
	if err != nil {
		return started, err
	}
	if started.Status == StatusRunning && strings.TrimSpace(started.URL) != "" {
		return started, nil
	}
	deadline, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	latest := started
	for {
		if current, ok := h.manager.Lookup(ws.Slug); ok {
			latest = current
			if latest.Status == StatusRunning && strings.TrimSpace(latest.URL) != "" {
				return latest, nil
			}
			switch latest.Status {
			case StatusFailed, StatusCrashed, StatusInvalid, StatusStopped:
				return latest, nil
			}
		}
		select {
		case <-deadline.Done():
			return latest, deadline.Err()
		case <-ticker.C:
		}
	}
}

func switchRedirectPath(raw string) (string, error) {
	return switchRedirectPathForTarget(raw, "", "")
}

func switchRedirectPathForTarget(raw, currentSlug, targetSlug string) (string, error) {
	redirectPath, err := ValidateLocalRedirectPath(raw)
	if err != nil {
		return "", err
	}
	if redirectPath == "/workspaces" || strings.HasPrefix(redirectPath, "/workspaces/") {
		return "/", nil
	}
	if strings.TrimSpace(currentSlug) == "" || strings.TrimSpace(targetSlug) == "" || strings.TrimSpace(currentSlug) == strings.TrimSpace(targetSlug) {
		return redirectPath, nil
	}
	if strings.HasPrefix(redirectPath, "/thoughts/") {
		return redirectPath, nil
	}
	return stripNonAuthSwitchRedirectQuery(redirectPath), nil
}

func stripNonAuthSwitchRedirectQuery(redirectPath string) string {
	u, err := url.Parse(redirectPath)
	if err != nil || u.RawQuery == "" {
		return redirectPath
	}
	query := u.Query()
	filtered := url.Values{}
	for key, values := range query {
		if !isSwitchAuthQueryParam(key) {
			continue
		}
		for _, value := range values {
			filtered.Add(key, value)
		}
	}
	u.RawQuery = filtered.Encode()
	return u.RequestURI()
}

func isSwitchAuthQueryParam(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "auth", "auth_token", "code", "redirect", "session", "session_id", "state", "token":
		return true
	default:
		return false
	}
}

func (h *Handler) HandleDevAuthHandoff(c echo.Context) error {
	if h.signer == nil || h.authService == nil {
		return echo.NewHTTPError(
			http.StatusNotFound,
			"dev auth handoff is not configured",
		)
	}
	ctx := c.Request().Context()
	claims, err := h.signer.Verify(c.QueryParam("token"), h.currentSlug)
	if err != nil {
		h.logDevAuthAttempt(ctx, "unknown", false, err)
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}
	if err := h.authService.ValidateEmail(claims.Email); err != nil {
		h.logDevAuthAttempt(ctx, claims.Email, false, err)
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}
	session, err := h.authService.CreateSession(ctx, claims.Email)
	if err != nil {
		h.logDevAuthAttempt(ctx, claims.Email, false, err)
		return err
	}
	h.logDevAuthAttempt(ctx, claims.Email, true, nil)
	c.SetCookie(&http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  session.ExpiresAt,
	})
	return c.Redirect(http.StatusFound, claims.RedirectPath)
}

func (h *Handler) logDevAuthAttempt(
	ctx context.Context,
	email string,
	success bool,
	err error,
) {
	if h.authService == nil {
		return
	}
	if strings.TrimSpace(email) == "" {
		email = "unknown"
	}
	errorMessage := ""
	if err != nil {
		errorMessage = err.Error()
	}
	_ = h.authService.LogAuthAttempt(ctx, email, success, errorMessage)
}

func (h *Handler) listLifecycle(
	ctx context.Context,
) ([]WorkspaceLifecycleSnapshot, error) {
	if h.lifecycle != nil {
		return h.lifecycle.ListLifecycle(ctx)
	}
	if h.manager == nil {
		return nil, echo.NewHTTPError(
			http.StatusNotFound,
			"workspace manager is not configured",
		)
	}
	if err := h.manager.Refresh(ctx); err != nil {
		return nil, err
	}
	items := h.manager.List()
	snapshots := make([]WorkspaceLifecycleSnapshot, 0, len(items))
	for _, ws := range items {
		snapshots = append(
			snapshots,
			snapshotFromState(ws, WorkspaceLifecycleState{}),
		)
	}
	return snapshots, nil
}

type ProjectFilter struct {
	ProjectID string
}

type ProjectOption struct {
	ID       string
	Label    string
	Selected bool
}

type workspacesPageModel struct {
	Views          []ImplWorkspaceView
	ReleasePanel   ReleasePanelModel
	ProjectFilter  ProjectFilter
	ProjectOptions []ProjectOption
}

func ProjectFilterFromRequest(r *http.Request) ProjectFilter {
	if r == nil {
		return ProjectFilter{}
	}
	return ProjectFilter{ProjectID: strings.TrimSpace(r.FormValue("project"))}
}

func (f ProjectFilter) QueryValue() string {
	return strings.TrimSpace(f.ProjectID)
}

func (f ProjectFilter) AppendTo(values url.Values) {
	if projectID := f.QueryValue(); projectID != "" {
		values.Set("project", projectID)
	}
}

func workspacesURL(path string, showCleanedHistory bool, filter ProjectFilter) string {
	values := url.Values{}
	if showCleanedHistory {
		values.Set("show_cleaned_history", "true")
	}
	filter.AppendTo(values)
	if encoded := values.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func (h *Handler) buildWorkspacesPageModel(ctx context.Context, filter ProjectFilter) (workspacesPageModel, error) {
	views, err := h.listImplWorkspaceViews(ctx, filter)
	if err != nil {
		return workspacesPageModel{}, err
	}
	views, err = h.attachPlanWorkspaceMetadata(ctx, views, filter)
	if err != nil {
		return workspacesPageModel{}, err
	}
	views, err = h.attachWorkflowSummaries(ctx, views)
	if err != nil {
		return workspacesPageModel{}, err
	}
	panel, rowActions, err := h.releaseProjectionForViews(ctx, views)
	if err != nil {
		return workspacesPageModel{}, err
	}
	if len(rowActions) > 0 {
		views = applyOptionsToImplWorkspaceViews(views, WithWorkspaceReleaseActions(rowActions))
	}
	options, err := h.projectOptions(ctx, views, filter.QueryValue())
	if err != nil {
		return workspacesPageModel{}, err
	}
	return workspacesPageModel{Views: views, ReleasePanel: panel, ProjectFilter: filter, ProjectOptions: options}, nil
}

func (h *Handler) listImplWorkspaceViews(
	ctx context.Context,
	filter ProjectFilter,
) ([]ImplWorkspaceView, error) {
	runtime, err := h.listLifecycle(ctx)
	if err != nil {
		return nil, err
	}
	runtime = filterLifecycleSnapshotsByProject(runtime, filter)
	if h.implWorkspaces == nil {
		return lifecycleSnapshotsToImplViews(runtime), nil
	}
	rows, err := h.implWorkspaces.ListImplWorkspaces(ctx, filter.QueryValue())
	if err != nil {
		return nil, err
	}
	main, nonMain := splitMainSnapshot(runtime)
	return BuildImplWorkspaceViews(rows, nonMain, main), nil
}

func filterLifecycleSnapshotsByProject(items []WorkspaceLifecycleSnapshot, filter ProjectFilter) []WorkspaceLifecycleSnapshot {
	projectID := filter.QueryValue()
	if projectID == "" {
		return items
	}
	out := make([]WorkspaceLifecycleSnapshot, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Workspace.ProjectID) == projectID {
			out = append(out, item)
		}
	}
	return out
}

func (h *Handler) attachPlanWorkspaceMetadata(ctx context.Context, views []ImplWorkspaceView, filter ProjectFilter) ([]ImplWorkspaceView, error) {
	if h.planWorkspaces == nil {
		return views, nil
	}
	planRels := collectImplViewPlanRels(views)
	if len(planRels) == 0 {
		return views, nil
	}
	models := make(map[string]PlanWorkspaceView, len(planRels))
	for _, planRel := range planRels {
		roles, err := h.planWorkspaces.ListPlanWorkspaceProjects(ctx, planRel)
		if err != nil {
			return nil, err
		}
		bindings, err := h.planWorkspaces.ListPlanWorkspaceImplBindings(ctx, planRel)
		if err != nil {
			return nil, err
		}
		models[planRel] = BuildPlanWorkspaceView(planRel, roles, bindings, nil, filter.QueryValue())
	}
	return applyPlanWorkspaceViews(views, models), nil
}

func collectImplViewPlanRels(views []ImplWorkspaceView) []string {
	seen := map[string]struct{}{}
	var ids []string
	var walk func([]ImplWorkspaceView)
	walk = func(items []ImplWorkspaceView) {
		for _, view := range items {
			if planRel := implViewPlanRel(view); planRel != "" {
				if _, ok := seen[planRel]; !ok {
					seen[planRel] = struct{}{}
					ids = append(ids, planRel)
				}
			}
			walk(view.Children)
		}
	}
	walk(views)
	slices.Sort(ids)
	return ids
}

func implViewPlanRel(view ImplWorkspaceView) string {
	return strings.TrimSpace(firstNonEmpty(nullStringValue(view.Row.PlanDirRel), nullStringValue(view.Row.PlanDir)))
}

func applyPlanWorkspaceViews(views []ImplWorkspaceView, models map[string]PlanWorkspaceView) []ImplWorkspaceView {
	out := append([]ImplWorkspaceView(nil), views...)
	for i := range out {
		if model, ok := models[implViewPlanRel(out[i])]; ok {
			out[i].Plan = model
		}
		out[i].Children = applyPlanWorkspaceViews(out[i].Children, models)
	}
	return out
}

func (h *Handler) projectOptions(ctx context.Context, views []ImplWorkspaceView, selected string) ([]ProjectOption, error) {
	seen := map[string]struct{}{}
	ids := projectIDsFromViews(views, seen)
	if h.planWorkspaces != nil {
		plans, err := h.planWorkspaces.ListCurrentPlanWorkspaces(ctx, "")
		if err != nil {
			return nil, err
		}
		for _, plan := range plans {
			addProjectOptionID(&ids, seen, plan.ProjectID)
			roles, err := h.planWorkspaces.ListPlanWorkspaceProjects(ctx, plan.PlanDirRel)
			if err != nil {
				return nil, err
			}
			for _, role := range roles {
				addProjectOptionID(&ids, seen, role.ProjectID)
			}
		}
	}
	slices.Sort(ids)
	options := make([]ProjectOption, 0, len(ids))
	for _, id := range ids {
		options = append(options, ProjectOption{ID: id, Label: id, Selected: id == selected})
	}
	return options, nil
}

func projectOptionsFromViews(views []ImplWorkspaceView, selected string) []ProjectOption {
	seen := map[string]struct{}{}
	ids := projectIDsFromViews(views, seen)
	slices.Sort(ids)
	options := make([]ProjectOption, 0, len(ids))
	for _, id := range ids {
		options = append(options, ProjectOption{ID: id, Label: id, Selected: id == selected})
	}
	return options
}

func projectIDsFromViews(views []ImplWorkspaceView, seen map[string]struct{}) []string {
	var ids []string
	var walk func([]ImplWorkspaceView)
	walk = func(items []ImplWorkspaceView) {
		for _, view := range items {
			addProjectOptionID(&ids, seen, firstNonEmpty(view.Row.ProjectID, view.Runtime.Workspace.ProjectID))
			for _, project := range view.Plan.Projects {
				addProjectOptionID(&ids, seen, project.ProjectID)
			}
			walk(view.Children)
		}
	}
	walk(views)
	return ids
}

func addProjectOptionID(ids *[]string, seen map[string]struct{}, projectID string) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return
	}
	if _, ok := seen[projectID]; ok {
		return
	}
	seen[projectID] = struct{}{}
	*ids = append(*ids, projectID)
}

func (h *Handler) releaseProjectionForViews(ctx context.Context, views []ImplWorkspaceView) (ReleasePanelModel, map[string][]ReleaseActionView, error) {
	if h.releaseProjector == nil {
		return ReleasePanelModel{Enabled: false}, nil, nil
	}
	return h.releaseProjector.BuildWorkspaceProjection(ctx, views)
}

func (h *Handler) attachWorkflowSummaries(ctx context.Context, views []ImplWorkspaceView) ([]ImplWorkspaceView, error) {
	if h.workflowSummaries == nil {
		return views, nil
	}
	rows := flattenImplWorkspaceRows(views)
	summaries, err := h.workflowSummariesForRows(ctx, rows)
	if err != nil {
		return nil, err
	}
	if len(summaries) == 0 {
		return views, nil
	}
	return applyOptionsToImplWorkspaceViews(views, WithWorkspaceWorkflowSummaries(summaries)), nil
}

func (h *Handler) workflowSummariesForRows(ctx context.Context, rows []db.ImplWorkspace) (map[string]WorkspaceWorkflowSummary, error) {
	out := make(map[string]WorkspaceWorkflowSummary)
	if h.workflowSummaries == nil {
		return out, nil
	}
	for _, row := range rows {
		planDir := firstNonEmpty(nullStringValue(row.PlanDirRel), nullStringValue(row.PlanDir))
		if planDir == "" || strings.TrimSpace(row.WorkspaceSlug) == "" {
			continue
		}
		summary, ok, err := h.workflowSummaries.SummaryForPlanDir(ctx, planDir)
		if err != nil {
			return nil, err
		}
		if ok {
			out[row.WorkspaceSlug] = summary
		}
	}
	return out, nil
}

func flattenImplWorkspaceRows(views []ImplWorkspaceView) []db.ImplWorkspace {
	rows := make([]db.ImplWorkspace, 0, len(views))
	var walk func([]ImplWorkspaceView)
	walk = func(items []ImplWorkspaceView) {
		for _, view := range items {
			rows = append(rows, view.Row)
			walk(view.Children)
		}
	}
	walk(views)
	return rows
}

func (h *Handler) renderedImplWorkspaceViews(views []ImplWorkspaceView, showHistorical bool) []ImplWorkspaceView {
	return orderReleaseLaneViewsFirst(
		filterHistoricalImplWorkspaceViews(views, showHistorical, h.protectedReleaseSlugs()),
		h.releaseLaneWorkspaces(),
	)
}

func (h *Handler) workspaceGroups(views []ImplWorkspaceView, showCleanedHistory bool) WorkspaceGroups {
	ordered := orderReleaseLaneViewsFirst(views, h.releaseLaneWorkspaces())
	return groupImplWorkspaceViews(ordered, h.protectedReleaseSlugs(), showCleanedHistory)
}

func (h *Handler) releaseLaneWorkspaces() []ReleaseLaneWorkspace {
	if h.releaseProjector == nil || h.releaseProjector.Registry == nil {
		return nil
	}
	return ReleaseLaneWorkspaces(h.releaseProjector.Registry)
}

func (h *Handler) protectedReleaseSlugList() []string {
	protected := h.protectedReleaseSlugs()
	if len(protected) == 0 {
		return nil
	}
	out := make([]string, 0, len(protected))
	for slug := range protected {
		out = append(out, slug)
	}
	return out
}

func snapshotsToWorkspaces(items []WorkspaceLifecycleSnapshot) []Workspace {
	out := make([]Workspace, 0, len(items))
	for _, item := range items {
		out = append(out, item.Workspace)
	}
	return out
}

func isDatastarRequest(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Datastar-Request"), "true") ||
		strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

func showCleanedHistoryFromRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	value := strings.TrimSpace(r.FormValue("show_cleaned_history"))
	if value == "" {
		value = strings.TrimSpace(r.FormValue("show_historical"))
	}
	return value == "true" || value == "1" || value == "on" || value == "yes"
}

func showHistoricalFromRequest(r *http.Request) bool {
	return showCleanedHistoryFromRequest(r)
}

func BuildNavItems(
	items []Workspace,
	currentSlug, managerURL string,
	redirectPath string,
	switchableSlugs ...string,
) []layouts.WorkspaceNavItem {
	managerURL = strings.TrimRight(strings.TrimSpace(managerURL), "/")
	redirect := "/"
	if validated, err := ValidateLocalRedirectPath(redirectPath); err == nil {
		redirect = validated
	}
	forcedRunning := make(map[string]struct{}, 2)
	forcedRunning[mainWorkspaceSlug] = struct{}{}
	forcedRunning[currentSlug] = struct{}{}
	switchable := make(map[string]struct{}, len(switchableSlugs)+2)
	switchable[mainWorkspaceSlug] = struct{}{}
	switchable[currentSlug] = struct{}{}
	for _, slug := range switchableSlugs {
		if slug = strings.TrimSpace(slug); slug != "" {
			switchable[slug] = struct{}{}
		}
	}
	nav := make([]layouts.WorkspaceNavItem, 0, len(items))
	for _, ws := range items {
		if _, ok := forcedRunning[ws.Slug]; ok {
			ws.Status = StatusRunning
		}
		itemURL := managerURL + "/workspaces"
		_, switchableOK := switchable[ws.Slug]
		if switchableOK && strings.TrimSpace(ws.URL) != "" {
			itemRedirect := redirect
			if ws.Slug != currentSlug {
				itemRedirect = stripNonAuthSwitchRedirectQuery(itemRedirect)
			}
			itemURL = managerURL + "/workspaces/switch/" + ws.Slug + "?redirect=" + url.QueryEscape(
				itemRedirect,
			)
		}
		nav = append(nav, layouts.WorkspaceNavItem{
			Slug:       ws.Slug,
			Label:      workspaceNavLabel(ws),
			URL:        itemURL,
			Status:     string(ws.Status),
			Current:    ws.Slug == currentSlug,
			ManagerURL: managerURL,
		})
	}
	return nav
}

func workspaceNavLabel(ws Workspace) string {
	if strings.TrimSpace(ws.DisplayName) != "" {
		return ws.DisplayName
	}
	return ws.Slug
}

func userEmailFromContext(c echo.Context) string {
	if email, ok := c.Get("user_email").(string); ok {
		return email
	}
	return ""
}

func render(c echo.Context, status int, component templ.Component) error {
	c.Response().Writer.WriteHeader(status)
	return component.Render(c.Request().Context(), c.Response().Writer)
}
