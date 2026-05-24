package workspaces

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type WorkspaceCleanupStarter interface {
	StartCleanup(ctx context.Context, input WorkspaceCleanupWorkflowInput) error
}

type WorkspaceCleanupDisposition string

const (
	WorkspaceCleanupDispositionMerged   WorkspaceCleanupDisposition = "merged"
	WorkspaceCleanupDispositionUnmerged WorkspaceCleanupDisposition = "unmerged"
)

type WorkspaceCleanupWorkflowInput struct {
	Slug         string                      `json:"slug"`
	TransitionID string                      `json:"transition_id"`
	Disposition  WorkspaceCleanupDisposition `json:"disposition"`
	Confirmed    bool                        `json:"confirmed"`
}

type WorkspaceCleanupAction struct {
	Slug              string
	Label             string
	Disposition       WorkspaceCleanupDisposition
	RequiresConfirm   bool
	Warning           string
	Disabled          bool
	DisabledReason    string
	PlanDirsPreserved bool
}

func (h *Handler) HandleCleanupWorkspace(c echo.Context) error {
	if h.cleanupStarter == nil {
		return echo.NewHTTPError(http.StatusNotImplemented, "workspace cleanup is not configured")
	}
	slug := strings.TrimSpace(c.FormValue("slug"))
	if slug == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing workspace slug")
	}
	views, err := h.listImplWorkspaceViews(c.Request().Context())
	if err != nil {
		return err
	}
	view, ok := findImplWorkspaceView(views, slug)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown workspace")
	}
	action := workspaceCleanupAction(view)
	if action.Disabled {
		return echo.NewHTTPError(http.StatusConflict, action.DisabledReason)
	}
	if h.isProtectedReleaseWorkspace(slug) {
		return echo.NewHTTPError(http.StatusBadRequest, "protected release lane cannot be cleaned up")
	}
	confirmed := c.FormValue("confirmed") == "true"
	if action.RequiresConfirm && !confirmed {
		return echo.NewHTTPError(http.StatusBadRequest, "unmerged workspace close requires confirmation")
	}
	input := WorkspaceCleanupWorkflowInput{Slug: slug, TransitionID: uuid.NewString(), Disposition: action.Disposition, Confirmed: confirmed}
	if err := h.cleanupStarter.StartCleanup(c.Request().Context(), input); err != nil {
		return err
	}
	h.notifier.Notify("workspace-cleanup")
	if isDatastarRequest(c.Request()) {
		return h.patchWorkspaces(c, views)
	}
	return c.Redirect(http.StatusSeeOther, "/workspaces")
}

func workspaceCleanupAction(view ImplWorkspaceView) WorkspaceCleanupAction {
	slug := workspaceViewSlug(view)
	action := WorkspaceCleanupAction{Slug: slug, PlanDirsPreserved: true}
	if slug == "" {
		action.Disabled = true
		action.DisabledReason = "workspace slug is unknown"
		return action
	}
	if view.IsMain || view.Runtime.Workspace.IsMain || slug == mainWorkspaceSlug {
		action.Disabled = true
		action.DisabledReason = "main workspace cannot be cleaned up"
		return action
	}
	switch view.Row.Status {
	case string(ImplWorkspaceStatusMerged):
		action.Label = "Clean up"
		action.Disposition = WorkspaceCleanupDispositionMerged
	case string(ImplWorkspaceStatusCleanedUp):
		action.Disabled = true
		action.DisabledReason = "workspace already cleaned up"
	case string(ImplWorkspaceStatusActive), "":
		action.Label = "Close"
		action.Disposition = WorkspaceCleanupDispositionUnmerged
		action.RequiresConfirm = true
		action.Warning = "Deletes checkout/runtime files; thoughts and plan docs remain."
	default:
		action.Disabled = true
		action.DisabledReason = "workspace cleanup unavailable"
	}
	return action
}

func findImplWorkspaceView(views []ImplWorkspaceView, slug string) (ImplWorkspaceView, bool) {
	for _, view := range views {
		if workspaceViewSlug(view) == slug {
			return view, true
		}
		if child, ok := findImplWorkspaceView(view.Children, slug); ok {
			return child, true
		}
	}
	return ImplWorkspaceView{}, false
}

func (h *Handler) isProtectedReleaseWorkspace(slug string) bool {
	if h.releaseProjector == nil || h.releaseProjector.Registry == nil {
		return false
	}
	for _, def := range h.releaseProjector.Registry.Definitions() {
		for _, lane := range def.Lanes {
			if strings.TrimSpace(lane.CheckoutSlug) == slug && lane.Protected {
				return true
			}
		}
	}
	return false
}
