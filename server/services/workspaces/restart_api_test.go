package workspaces

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestInternalRestartRejectsBadToken(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		nil,
		"https://main.cn-agents.test",
		"main",
		WithRestartAPI("secret", "/repo/cn-agents"),
	)
	rec, err := runInternalRestart(handler, "bad", RestartRequest{Slug: "feature"})
	if err == nil {
		t.Fatal("HandleInternalRestart error = nil, want unauthorized")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf(
			"recorder status = %d before Echo error handling, want default OK",
			rec.Code,
		)
	}
}

func TestInternalRestartRestartsChildWorkspace(t *testing.T) {
	t.Parallel()

	manager := &fakeRestartManager{
		workspaces: map[string]Workspace{
			"feature": {
				Slug:         "feature",
				CheckoutPath: "/repo/cn-agents-feature",
				Status:       StatusRunning,
			},
		},
	}
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithRestartAPI("secret", "/repo/cn-agents"),
	)
	rec, err := runInternalRestart(
		handler,
		"secret",
		RestartRequest{
			Slug:         "feature",
			CheckoutPath: "/repo/cn-agents-feature",
			Components:   []BundleComponent{ComponentWeb},
		},
	)
	if err != nil {
		t.Fatalf("HandleInternalRestart: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if manager.componentRestarted != "feature" {
		t.Fatalf("componentRestarted = %q, want feature", manager.componentRestarted)
	}
	if len(manager.restartedComponents) != 1 ||
		manager.restartedComponents[0] != ComponentWeb {
		t.Fatalf("restartedComponents = %#v, want web", manager.restartedComponents)
	}
	if manager.restartOptions.Force {
		t.Fatal("Force = true, want false")
	}
}

func TestInternalRestartResolvesStaleSlugByCheckoutPath(t *testing.T) {
	t.Parallel()

	manager := &fakeRestartManager{
		workspaces: map[string]Workspace{
			"new-slug": {
				Slug:         "new-slug",
				CheckoutPath: "/repo/cn-agents-feature",
				Status:       StatusRunning,
			},
		},
	}
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithRestartAPI("secret", "/repo/cn-agents"),
	)
	rec, err := runInternalRestart(
		handler,
		"secret",
		RestartRequest{
			Slug:         "old-too-long-slug",
			CheckoutPath: "/repo/cn-agents-feature",
			Components:   []BundleComponent{ComponentWeb},
		},
	)
	if err != nil {
		t.Fatalf("HandleInternalRestart: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if manager.componentRestarted != "new-slug" {
		t.Fatalf("componentRestarted = %q, want new-slug", manager.componentRestarted)
	}
}

func TestInternalRestartPassesForceOption(t *testing.T) {
	t.Parallel()

	manager := &fakeRestartManager{
		workspaces: map[string]Workspace{
			"feature": {
				Slug:         "feature",
				CheckoutPath: "/repo/cn-agents-feature",
				Status:       StatusRunning,
			},
		},
	}
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithRestartAPI("secret", "/repo/cn-agents"),
	)
	_, err := runInternalRestart(
		handler,
		"secret",
		RestartRequest{
			Slug:         "feature",
			CheckoutPath: "/repo/cn-agents-feature",
			Components:   []BundleComponent{ComponentWeb},
			Force:        true,
		},
	)
	if err != nil {
		t.Fatalf("HandleInternalRestart: %v", err)
	}
	if !manager.restartOptions.Force {
		t.Fatal("Force = false, want true")
	}
}

func TestInternalRestartForceDoesNotUseHandlerStartFallback(t *testing.T) {
	t.Parallel()

	manager := &fakeRestartManager{
		workspaces: map[string]Workspace{
			"feature": {
				Slug:         "feature",
				CheckoutPath: "/repo/cn-agents-feature",
				Status:       StatusFailed,
			},
		},
	}
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithRestartAPI("secret", "/repo/cn-agents"),
	)
	_, err := runInternalRestart(
		handler,
		"secret",
		RestartRequest{
			Slug:         "feature",
			CheckoutPath: "/repo/cn-agents-feature",
			Components:   []BundleComponent{ComponentWeb},
			Force:        true,
		},
	)
	if err == nil {
		t.Fatal("HandleInternalRestart error = nil, want force restart error")
	}
	if manager.started != "" {
		t.Fatalf("started = %q, want empty", manager.started)
	}
}

func TestInternalRestartStartsFailedWorkspace(t *testing.T) {
	t.Parallel()

	manager := &fakeRestartManager{
		workspaces: map[string]Workspace{
			"feature": {
				Slug:         "feature",
				CheckoutPath: "/repo/cn-agents-feature",
				Status:       StatusFailed,
			},
		},
	}
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithRestartAPI("secret", "/repo/cn-agents"),
	)
	rec, err := runInternalRestart(
		handler,
		"secret",
		RestartRequest{
			Slug:         "feature",
			CheckoutPath: "/repo/cn-agents-feature",
			Components:   []BundleComponent{ComponentWeb},
		},
	)
	if err != nil {
		t.Fatalf("HandleInternalRestart: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if manager.started != "feature" {
		t.Fatalf("started = %q, want feature", manager.started)
	}
}

func TestInternalRestartRejectsMissingCheckout(t *testing.T) {
	t.Parallel()

	manager := &fakeRestartManager{
		workspaces: map[string]Workspace{
			"feature": {
				Slug:         "feature",
				CheckoutPath: "/repo/cn-agents-feature",
				Status:       StatusRunning,
			},
		},
	}
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithRestartAPI("secret", "/repo/cn-agents"),
	)
	_, err := runInternalRestart(
		handler,
		"secret",
		RestartRequest{Slug: "feature", Components: []BundleComponent{ComponentWeb}},
	)
	if err == nil {
		t.Fatal("HandleInternalRestart error = nil, want missing checkout error")
	}
	if manager.componentRestarted != "" {
		t.Fatalf("componentRestarted = %q, want empty", manager.componentRestarted)
	}
}

func TestInternalRestartRejectsMainSlugWithChildCheckout(t *testing.T) {
	t.Parallel()

	manager := &fakeRestartManager{
		workspaces: map[string]Workspace{
			"feature": {
				Slug:         "feature",
				CheckoutPath: "/repo/cn-agents-feature",
				Status:       StatusRunning,
			},
		},
	}
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithRestartAPI("secret", "/repo/cn-agents"),
	)
	_, err := runInternalRestart(
		handler,
		"secret",
		RestartRequest{Slug: "main", CheckoutPath: "/repo/cn-agents-feature"},
	)
	if err == nil {
		t.Fatal("HandleInternalRestart error = nil, want mismatch error")
	}
}

func TestInternalRestartRejectsMismatchedCheckout(t *testing.T) {
	t.Parallel()

	manager := &fakeRestartManager{
		workspaces: map[string]Workspace{
			"feature": {
				Slug:         "feature",
				CheckoutPath: "/repo/cn-agents-feature",
				Status:       StatusRunning,
			},
		},
	}
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithRestartAPI("secret", "/repo/cn-agents"),
	)
	_, err := runInternalRestart(
		handler,
		"secret",
		RestartRequest{
			Slug:         "feature",
			CheckoutPath: "/repo/other",
			Components:   []BundleComponent{ComponentWeb},
		},
	)
	if err == nil {
		t.Fatal("HandleInternalRestart error = nil, want mismatch error")
	}
	if manager.componentRestarted != "" {
		t.Fatalf("componentRestarted = %q, want empty", manager.componentRestarted)
	}
}

func TestInternalRestartReadsSlugFromMetadata(t *testing.T) {
	t.Parallel()

	checkout := t.TempDir()
	if err := WriteMetadata(
		WorkspaceMetadataPath(checkout),
		WorkspaceMetadata{Slug: "feature", CheckoutPath: checkout},
	); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}
	manager := &fakeRestartManager{
		workspaces: map[string]Workspace{
			"feature": {Slug: "feature", CheckoutPath: checkout, Status: StatusRunning},
		},
	}
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithRestartAPI("secret", filepath.Join(t.TempDir(), "cn-agents")),
	)
	rec, err := runInternalRestart(
		handler,
		"secret",
		RestartRequest{CheckoutPath: checkout},
	)
	if err != nil {
		t.Fatalf("HandleInternalRestart: %v", err)
	}
	if rec.Code != http.StatusAccepted || manager.componentRestarted != "feature" {
		t.Fatalf("status=%d componentRestarted=%q", rec.Code, manager.componentRestarted)
	}
}

func TestInternalRestartMainReturnsBeforeExit(t *testing.T) {
	t.Parallel()

	exited := make(chan int, 1)
	handler := NewHandler(
		nil,
		"https://main.cn-agents.test",
		"main",
		WithRestartAPI("secret", "/repo/cn-agents"),
		WithExitFunc(func(code int) {
			exited <- code
		}),
	)
	rec, err := runInternalRestart(
		handler,
		"secret",
		RestartRequest{Slug: "main", CheckoutPath: "/repo/cn-agents"},
	)
	if err != nil {
		t.Fatalf("HandleInternalRestart: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want accepted", rec.Code)
	}
	select {
	case code := <-exited:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	case <-time.After(time.Second):
		t.Fatal("exit func was not called")
	}
}

func runInternalRestart(
	handler *Handler,
	token string,
	body RestartRequest,
) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(
		http.MethodPost,
		"/internal/workspaces/restart",
		strings.NewReader(string(payload)),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Vamos-Workspace-Restart-Token", token)
	rec := httptest.NewRecorder()
	return rec, handler.HandleInternalRestart(e.NewContext(req, rec))
}

type fakeRestartManager struct {
	workspaces          map[string]Workspace
	started             string
	restarted           string
	componentRestarted  string
	restartedComponents []BundleComponent
	restartOptions      RestartComponentsOptions
}

func (m *fakeRestartManager) Refresh(_ context.Context) error { return nil }
func (m *fakeRestartManager) List() []Workspace {
	items := make([]Workspace, 0, len(m.workspaces))
	for _, ws := range m.workspaces {
		items = append(items, ws)
	}
	return items
}

func (m *fakeRestartManager) Lookup(slug string) (Workspace, bool) {
	ws, ok := m.workspaces[slug]
	return ws, ok
}

func (m *fakeRestartManager) LookupHost(
	string,
) (Workspace, bool) {
	return Workspace{}, false
}

func (m *fakeRestartManager) Start(_ context.Context, slug string) (Workspace, error) {
	m.started = slug
	ws := m.workspaces[slug]
	ws.Status = StatusRunning
	m.workspaces[slug] = ws
	return ws, nil
}

func (m *fakeRestartManager) Stop(_ context.Context, slug string) (Workspace, error) {
	return m.workspaces[slug], nil
}

func (m *fakeRestartManager) Restart(_ context.Context, slug string) (Workspace, error) {
	m.restarted = slug
	ws := m.workspaces[slug]
	ws.Status = StatusRunning
	return ws, nil
}

func (m *fakeRestartManager) RestartComponents(
	_ context.Context,
	slug string,
	components []BundleComponent,
	opts RestartComponentsOptions,
) (Workspace, error) {
	m.componentRestarted = slug
	m.restartedComponents = append([]BundleComponent(nil), components...)
	m.restartOptions = opts
	ws := m.workspaces[slug]
	if ws.Status != StatusRunning {
		return ws, fmt.Errorf("workspace %q is %s", slug, ws.Status)
	}
	ws.Status = StatusRunning
	return ws, nil
}
