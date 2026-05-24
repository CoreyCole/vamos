package workspaces

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/CoreyCole/vamos/pkg/release"
)

func (h *Handler) HandleEnqueueRelease(c echo.Context) error {
	if h.releaseProjector == nil || h.releaseProjector.Registry == nil || h.releaseStore == nil || h.releaseStarter == nil {
		return echo.NewHTTPError(http.StatusNotImplemented, "release queue is not configured")
	}
	req, err := parseReleaseEnqueueForm(c)
	if err != nil {
		return err
	}
	model, err := h.buildWorkspacesPageModel(c.Request().Context())
	if err != nil {
		return err
	}
	def, action, ok := resolveReleaseAction(model.ReleasePanel, model.Views, h.releaseProjector.Registry, req)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "release action is unavailable")
	}
	if action.Disabled {
		return echo.NewHTTPError(http.StatusConflict, releaseDisabledReason(action))
	}
	flow := def.Flows[action.FlowID]
	if action.ExpectedSourceCommit != req.ExpectedSourceCommit || action.ExpectedTargetCommit != req.ExpectedTargetCommit {
		return echo.NewHTTPError(http.StatusConflict, "release action has stale expected commits")
	}
	itemID, err := newReleaseQueueItemID()
	if err != nil {
		return err
	}
	item, err := h.releaseStore.CreateReleaseQueueItem(c.Request().Context(), CreateReleaseQueueItemParams{
		ID:                   itemID,
		DefinitionID:         def.ID,
		DefinitionVersion:    def.Version,
		WorkflowID:           flow.WorkflowID,
		WorkflowVersion:      flow.WorkflowVersion,
		FlowID:               flow.ID,
		SourceSlug:           action.SourceSlug,
		TargetLane:           string(action.TargetLane),
		ExpectedSourceCommit: req.ExpectedSourceCommit,
		ExpectedTargetCommit: req.ExpectedTargetCommit,
		ActorEmail:           userEmailFromContext(c),
	})
	if err != nil {
		return err
	}
	if err := h.releaseStarter.EnqueueRelease(c.Request().Context(), item.ID); err != nil {
		return err
	}
	h.notifier.Notify("release-queue")
	if isDatastarRequest(c.Request()) {
		return h.patchWorkspaces(c, model.Views)
	}
	return c.Redirect(http.StatusSeeOther, "/workspaces")
}

type releaseEnqueueRequest struct {
	DefinitionID         release.DefinitionID
	DefinitionVersion    string
	FlowID               release.FlowID
	SourceSlug           string
	ExpectedSourceCommit string
	ExpectedTargetCommit string
}

func parseReleaseEnqueueForm(c echo.Context) (releaseEnqueueRequest, error) {
	req := releaseEnqueueRequest{
		DefinitionID:         release.DefinitionID(strings.TrimSpace(c.FormValue("definition_id"))),
		DefinitionVersion:    strings.TrimSpace(c.FormValue("definition_version")),
		FlowID:               release.FlowID(strings.TrimSpace(c.FormValue("flow_id"))),
		SourceSlug:           strings.TrimSpace(c.FormValue("source_slug")),
		ExpectedSourceCommit: strings.TrimSpace(c.FormValue("expected_source_commit")),
		ExpectedTargetCommit: strings.TrimSpace(c.FormValue("expected_target_commit")),
	}
	if req.DefinitionID == "" || req.DefinitionVersion == "" || req.FlowID == "" || req.SourceSlug == "" {
		return req, echo.NewHTTPError(http.StatusBadRequest, "missing release action fields")
	}
	if req.ExpectedSourceCommit == "" || req.ExpectedTargetCommit == "" {
		return req, echo.NewHTTPError(http.StatusBadRequest, "missing expected release commits")
	}
	return req, nil
}

func resolveReleaseAction(panel ReleasePanelModel, views []ImplWorkspaceView, reg *release.Registry, req releaseEnqueueRequest) (release.Definition, ReleaseActionView, bool) {
	def, ok := reg.Definition(req.DefinitionID, req.DefinitionVersion)
	if !ok {
		return release.Definition{}, ReleaseActionView{}, false
	}
	for _, action := range releaseProjectedActions(panel, views) {
		if action.DefinitionID == req.DefinitionID && action.DefinitionVersion == req.DefinitionVersion && action.FlowID == req.FlowID && action.SourceSlug == req.SourceSlug {
			return def, action, true
		}
	}
	return release.Definition{}, ReleaseActionView{}, false
}

func releaseProjectedActions(panel ReleasePanelModel, views []ImplWorkspaceView) []ReleaseActionView {
	actions := make([]ReleaseActionView, 0)
	for _, lane := range panel.Lanes {
		actions = append(actions, lane.Actions...)
	}
	var walk func([]ImplWorkspaceView)
	walk = func(items []ImplWorkspaceView) {
		for _, view := range items {
			actions = append(actions, view.ReleaseActions...)
			walk(view.Children)
		}
	}
	walk(views)
	return actions
}

func newReleaseQueueItemID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "rel_" + hex.EncodeToString(buf[:]), nil
}

func (h *Handler) patchWorkspaces(c echo.Context, views []ImplWorkspaceView) error {
	showHistorical := showHistoricalFromRequest(c.Request())
	panel, rowActions, err := h.releaseProjectionForViews(c.Request().Context(), views)
	if err != nil {
		return err
	}
	if len(rowActions) > 0 {
		views = applyOptionsToImplWorkspaceViews(views, WithWorkspaceReleaseActions(rowActions))
	}
	renderedViews := filterHistoricalImplWorkspaceViews(views, showHistorical)
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	c.Response().WriteHeader(http.StatusAccepted)
	if err := sse.PatchElementTempl(WorkspacesHeader(h.isRefreshInFlight(), showHistorical), datastar.WithSelectorID("workspaces-header"), datastar.WithModeOuter()); err != nil {
		return err
	}
	if err := sse.PatchElementTempl(ReleasePanel(panel), datastar.WithSelectorID("release-queue-panel"), datastar.WithModeOuter()); err != nil {
		return err
	}
	return sse.PatchElementTempl(WorkspacesList(renderedViews, h.managerURL, showHistorical), datastar.WithSelectorID("workspaces-list"), datastar.WithModeOuter())
}
