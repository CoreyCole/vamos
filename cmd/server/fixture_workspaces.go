package main

import (
	"context"
	"errors"
	"strings"

	"github.com/CoreyCole/vamos/server/services/workspaces"
)

var errFixtureWorkspacesReadOnly = errors.New("fixture Workspaces page is read-only")

type fixtureLifecycleRegistry struct {
	snapshot workspaces.WorkspaceLifecycleSnapshot
}

func newFixtureLifecycleRegistry(cfg Config) *fixtureLifecycleRegistry {
	slug := strings.TrimSpace(cfg.WorkspaceSlug)
	if slug == "" {
		slug = "fixture"
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")
	host := hostFromBaseURL(baseURL)

	workspace := workspaces.Workspace{
		Slug:         slug,
		DisplayName:  slug,
		CheckoutPath: strings.TrimSpace(cfg.RepoPath),
		Host:         host,
		URL:          baseURL,
		Status:       workspaces.StatusRunning,
		IsConfigured: true,
	}
	return &fixtureLifecycleRegistry{snapshot: workspaces.WorkspaceLifecycleSnapshot{
		Workspace:     workspace,
		DesiredState:  workspaces.WorkspaceDesiredRunning,
		ObservedState: workspaces.WorkspaceObservedRunning,
	}}
}

func (r *fixtureLifecycleRegistry) Refresh(context.Context) error { return nil }

func (r *fixtureLifecycleRegistry) List() []workspaces.Workspace {
	if strings.TrimSpace(r.snapshot.Workspace.Slug) == "" {
		return nil
	}
	return []workspaces.Workspace{r.snapshot.Workspace}
}

func (r *fixtureLifecycleRegistry) Lookup(slug string) (workspaces.Workspace, bool) {
	if strings.TrimSpace(slug) == strings.TrimSpace(r.snapshot.Workspace.Slug) {
		return r.snapshot.Workspace, true
	}
	return workspaces.Workspace{}, false
}

func (r *fixtureLifecycleRegistry) LookupHost(host string) (workspaces.Workspace, bool) {
	if strings.TrimSpace(host) == "" || strings.TrimSpace(r.snapshot.Workspace.Host) == "" {
		return workspaces.Workspace{}, false
	}
	if strings.EqualFold(strings.TrimSpace(host), strings.TrimSpace(r.snapshot.Workspace.Host)) {
		return r.snapshot.Workspace, true
	}
	return workspaces.Workspace{}, false
}

func (r *fixtureLifecycleRegistry) Start(context.Context, string) (workspaces.Workspace, error) {
	return workspaces.Workspace{}, errFixtureWorkspacesReadOnly
}

func (r *fixtureLifecycleRegistry) Stop(context.Context, string) (workspaces.Workspace, error) {
	return workspaces.Workspace{}, errFixtureWorkspacesReadOnly
}

func (r *fixtureLifecycleRegistry) Restart(context.Context, string) (workspaces.Workspace, error) {
	return workspaces.Workspace{}, errFixtureWorkspacesReadOnly
}

func (r *fixtureLifecycleRegistry) ListLifecycle(context.Context) ([]workspaces.WorkspaceLifecycleSnapshot, error) {
	if strings.TrimSpace(r.snapshot.Workspace.Slug) == "" {
		return nil, nil
	}
	return []workspaces.WorkspaceLifecycleSnapshot{r.snapshot}, nil
}

func (r *fixtureLifecycleRegistry) RequestLifecycle(context.Context, workspaces.WorkspaceLifecycleRequest) (workspaces.WorkspaceLifecycleSnapshot, error) {
	return workspaces.WorkspaceLifecycleSnapshot{}, errFixtureWorkspacesReadOnly
}

func (r *fixtureLifecycleRegistry) CompleteTransition(context.Context, string, string, workspaces.WorkspaceTransitionResult) error {
	return errFixtureWorkspacesReadOnly
}
