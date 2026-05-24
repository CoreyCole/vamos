package workspaces

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/CoreyCole/vamos/server/layouts"
)

func (h *Handler) HandleWorkspaceErrorsPage(c echo.Context) error {
	model, err := h.workspaceErrorPageModel(c.Request().Context(), c.QueryParam("workspace"))
	if err != nil {
		return err
	}
	h.triggerWorkspaceErrorScan(model.SelectedWorkspace)
	model.ScanInFlight = h.isWorkspaceErrorScanInFlight(model.SelectedWorkspace)
	args := layouts.RootArgs{
		Title:       "Workspace errors",
		CurrentPath: "/workspaces/errors",
		PageType:    layouts.PageTypeWorkspaces,
		ShowHeader:  true,
		UserEmail:   userEmailFromContext(c),
		Workspaces: BuildNavItems(
			ImplViewsToNavWorkspaces(model.Workspaces),
			h.currentSlug,
			h.managerURL,
			"/workspaces/errors",
			h.protectedReleaseSlugList()...,
		),
		CurrentWorkspaceSlug: h.currentSlug,
		WorkspaceManagerURL:  h.managerURL,
	}
	return render(c, http.StatusOK, WorkspaceErrorsDocument(args, model))
}

func (h *Handler) HandleWorkspaceErrorsStream(c echo.Context) error {
	selected := strings.TrimSpace(c.QueryParam("workspace"))
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	send := func() error {
		model, err := h.workspaceErrorPageModel(c.Request().Context(), selected)
		if err != nil {
			return err
		}
		return sse.PatchElementTempl(
			WorkspaceErrorQueue(model),
			datastar.WithSelectorID("workspace-error-queue"),
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

func (h *Handler) workspaceErrorPageModel(ctx context.Context, selected string) (WorkspaceErrorPageModel, error) {
	selected = strings.TrimSpace(selected)
	views, err := h.listImplWorkspaceViews(ctx)
	if err != nil {
		return WorkspaceErrorPageModel{}, err
	}
	views = h.renderedImplWorkspaceViews(views, true)
	var events []WorkspaceErrorEvent
	if h.workspaceErrorRecorder != nil && h.workspaceErrorRecorder.Store != nil {
		if selected != "" {
			events, err = h.workspaceErrorRecorder.Store.ListRecentWorkspaceErrorEventsForWorkspace(ctx, ListRecentWorkspaceErrorEventsForWorkspaceParams{WorkspaceSlug: selected, Limit: workspaceErrorEventLimit})
		} else {
			events, err = h.workspaceErrorRecorder.Store.ListRecentWorkspaceErrorEvents(ctx, workspaceErrorEventLimit)
		}
		if err != nil {
			return WorkspaceErrorPageModel{}, err
		}
	}
	model := WorkspaceErrorPageModel{
		SelectedWorkspace: selected,
		Workspaces:        views,
		ScanInFlight:      h.isWorkspaceErrorScanInFlight(selected),
		ManagerURL:        h.managerURL,
	}
	for _, event := range events {
		model.Events = append(model.Events, mapWorkspaceErrorEventView(event))
	}
	return model, nil
}
