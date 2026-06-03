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
	ProjectID    string                      `json:"project_id"`
	Slug         string                      `json:"slug"`
	TransitionID string                      `json:"transition_id"`
	Disposition  WorkspaceCleanupDisposition `json:"disposition"`
	Confirmed    bool                        `json:"confirmed"`
}

type WorkspaceCleanupAction struct {
	ProjectID         string
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
	filter := WorkspacesFilterFromRequest(c.Request())
	views, err := h.listImplWorkspaceViews(c.Request().Context(), filter)
	if err != nil {
		return err
	}
	view, ok := findImplWorkspaceViewForProject(views, filter.ProjectQueryValue(), slug)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown workspace")
	}
	action := workspaceCleanupAction(view)
	if h.isProtectedReleaseWorkspace(slug) {
		return echo.NewHTTPError(http.StatusBadRequest, "protected release lane cannot be cleaned up")
	}
	if action.Disabled {
		if isIdempotentCleanupNoop(action, view) {
			if isDatastarRequest(c.Request()) {
				return h.patchWorkspacesFresh(c)
			}
			return c.Redirect(http.StatusSeeOther, workspacesURL("/workspaces", filter))
		}
		return echo.NewHTTPError(http.StatusConflict, action.DisabledReason)
	}
	confirmed := c.FormValue("confirmed") == "true"
	if action.RequiresConfirm && !confirmed {
		return echo.NewHTTPError(http.StatusBadRequest, "unmerged workspace close requires confirmation")
	}
	input := WorkspaceCleanupWorkflowInput{ProjectID: view.Row.ProjectID, Slug: slug, TransitionID: uuid.NewString(), Disposition: action.Disposition, Confirmed: confirmed}
	if err := h.cleanupStarter.StartCleanup(c.Request().Context(), input); err != nil {
		return err
	}
	h.notifier.Notify("workspace-cleanup")
	if isDatastarRequest(c.Request()) {
		return h.patchWorkspacesFreshWithoutSlug(c, slug)
	}
	return c.Redirect(http.StatusSeeOther, workspacesURL("/workspaces", filter))
}

func workspaceCleanupAction(view ImplWorkspaceView) WorkspaceCleanupAction {
	slug := workspaceViewSlug(view)
	action := WorkspaceCleanupAction{ProjectID: strings.TrimSpace(view.Row.ProjectID), Slug: slug, PlanDirsPreserved: true}
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
	if view.Runtime.Workspace.IsConfigured || IsProtectedCheckoutRole(CheckoutRole(strings.TrimSpace(view.Row.CheckoutRole))) {
		action.Disabled = true
		action.DisabledReason = "configured checkout cannot be cleaned up"
		return action
	}
	readiness := view.Cleanup
	if readiness.Group == "" {
		readiness = workspaceCleanupReadiness(view)
	}
	switch view.Row.Status {
	case string(ImplWorkspaceStatusMerged):
		if !readiness.Safe {
			action.Disabled = true
			action.DisabledReason = workspaceRiskReason(view)
			return action
		}
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

func isIdempotentCleanupNoop(action WorkspaceCleanupAction, view ImplWorkspaceView) bool {
	if view.Row.Status == string(ImplWorkspaceStatusCleanedUp) {
		return true
	}
	return strings.Contains(strings.ToLower(action.DisabledReason), "already cleaned")
}

func findImplWorkspaceView(views []ImplWorkspaceView, slug string) (ImplWorkspaceView, bool) {
	return findImplWorkspaceViewForProject(views, "", slug)
}

func findImplWorkspaceViewForProject(views []ImplWorkspaceView, projectID, slug string) (ImplWorkspaceView, bool) {
	projectID = strings.TrimSpace(projectID)
	for _, view := range views {
		if workspaceViewSlug(view) == slug && (projectID == "" || strings.TrimSpace(view.Row.ProjectID) == projectID) {
			return view, true
		}
		if child, ok := findImplWorkspaceViewForProject(view.Children, projectID, slug); ok {
			return child, true
		}
	}
	return ImplWorkspaceView{}, false
}

func (h *Handler) isProtectedReleaseWorkspace(slug string) bool {
	_, ok := h.protectedReleaseSlugs()[strings.TrimSpace(slug)]
	return ok
}

func (h *Handler) protectedReleaseSlugs() map[string]ReleaseLaneWorkspace {
	if h.releaseProjector == nil || h.releaseProjector.Registry == nil {
		return nil
	}
	return ProtectedReleaseSlugs(h.releaseProjector.Registry)
}
