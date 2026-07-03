package applets

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/CoreyCole/vamos/server/services/appletruntime"
	"github.com/labstack/echo/v4"
)

func TestBuildWorkbenchStateUsesOnlyFilesAppAndChatRegions(t *testing.T) {
	state := BuildWorkbenchState(t.Context(), WorkbenchState{
		Config: AppletConfig{ID: "pickleball"},
		Files:  FilesViewModel{Title: "Files", Component: EmptyRegion("files")},
		Chat:   ChatViewModel{Title: "Chat", Component: EmptyRegion("chat")},
	})

	if len(state.Regions) != 2 {
		t.Fatalf("regions = %d, want 2", len(state.Regions))
	}

	files := state.Regions[0]
	if files.ID != "pickleball-files-app" || files.Slot != workbench.WorkbenchSlotPrimary || files.Title != "Files" {
		t.Fatalf("files region = %#v", files)
	}
	if files.TargetID != "pickleball-files-app-region" {
		t.Fatalf("files target = %q", files.TargetID)
	}

	chat := state.Regions[1]
	if chat.ID != "pickleball-chat" || chat.Slot != workbench.WorkbenchSlotContext || chat.Title != "Chat" {
		t.Fatalf("chat region = %#v", chat)
	}
	if chat.TargetID != "pickleball-chat-region" {
		t.Fatalf("chat target = %q", chat.TargetID)
	}

	for _, region := range state.Regions {
		if region.Slot == workbench.WorkbenchSlotNavigation || region.Kind == workbench.RegionWorkspaceTopology {
			t.Fatalf("technical/navigation region leaked: %#v", region)
		}
	}
}

func TestBuildWorkbenchStateDefaultsMobileToFilesApp(t *testing.T) {
	state := BuildWorkbenchState(t.Context(), WorkbenchState{Config: AppletConfig{ID: "pickleball"}})

	if got := state.Config.Mobile.ActiveRegionID; got != "pickleball-files-app" {
		t.Fatalf("mobile active region = %q, want files/app", got)
	}
}

func TestEmptyRegionRendersFriendlyPlaceholder(t *testing.T) {
	var body bytes.Buffer
	if err := EmptyRegion("Files will appear here.").Render(t.Context(), &body); err != nil {
		t.Fatalf("render empty region: %v", err)
	}
	if !strings.Contains(body.String(), "Files will appear here.") {
		t.Fatalf("placeholder body = %q", body.String())
	}
}

func TestBuildAppletWorkbenchStateUsesDocumentWorkbenchRegions(t *testing.T) {
	state, err := BuildAppletWorkbenchState(AppletWorkbenchInput{
		Context: appletTestContext(),
		Process: appletruntime.AppletProcessState{
			AppID:  "wordle",
			Status: appletruntime.ProcessStatusHealthy,
		},
		Sidebar: workbench.WorkbenchSidebarArgs{Tabs: workbench.DefaultSidebarTabs()},
		RightRail: workbench.RightRailArgs{
			Chat:     EmptyRegion("chat"),
			Comments: EmptyRegion("comments"),
		},
	})
	if err != nil {
		t.Fatalf("BuildAppletWorkbenchState() error = %v", err)
	}
	if state.ActivePath != "thoughts/apps/wordle.md" {
		t.Fatalf("active path = %q", state.ActivePath)
	}
	if len(state.Regions) != 3 {
		t.Fatalf("regions = %d, want document workbench shell", len(state.Regions))
	}
	if state.Regions[0].Slot != workbench.WorkbenchSlotNavigation || state.Regions[1].Slot != workbench.WorkbenchSlotPrimary || state.Regions[2].Slot != workbench.WorkbenchSlotContext {
		t.Fatalf("regions = %#v", state.Regions)
	}
}

func TestAppletFrameRendersIframeWhenHealthy(t *testing.T) {
	var body bytes.Buffer
	err := AppletFrame(appletTestContext(), appletruntime.AppletProcessState{Status: appletruntime.ProcessStatusHealthy}).Render(t.Context(), &body)
	if err != nil {
		t.Fatalf("AppletFrame.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{"<iframe", `src="/thoughts/_render/app/wordle/app/"`, `sandbox="allow-same-origin allow-forms allow-downloads allow-scripts"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("AppletFrame html missing %q: %s", want, html)
		}
	}
}

func TestAppletFrameRendersStartingPanelWithStatusStream(t *testing.T) {
	var body bytes.Buffer
	err := AppletFrame(appletTestContext(), appletruntime.AppletProcessState{Status: appletruntime.ProcessStatusStarting, LogPath: "/tmp/app.log"}).Render(t.Context(), &body)
	if err != nil {
		t.Fatalf("AppletFrame.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{"Starting Wordle", "@get(&#39;/thoughts/_render/app/wordle/status&#39;)", "/tmp/app.log"} {
		if !strings.Contains(html, want) {
			t.Fatalf("AppletFrame html missing %q: %s", want, html)
		}
	}
}

func TestHandleAppletStatusExecutesReloadWhenHealthy(t *testing.T) {
	examplesRoot := t.TempDir()
	writeAppletManifest(t, examplesRoot, "wordle")
	service := NewHTTPService(ServiceOptions{
		Resolver: Resolver{ExamplesRoot: examplesRoot},
		Manager:  &sequenceManager{states: []appletruntime.AppletProcessState{{Status: appletruntime.ProcessStatusStarting}, {Status: appletruntime.ProcessStatusHealthy}}},
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/examples/wordle/status", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("wordle")

	if err := service.HandleAppletStatus(c); err != nil {
		t.Fatalf("HandleAppletStatus() error = %v", err)
	}
	if body := rec.Body.String(); !strings.Contains(body, "window.location.reload") {
		t.Fatalf("status stream body = %s", body)
	}
}

func appletTestContext() AppletContext {
	return AppletContext{
		Manifest:     AppletManifest{ID: "wordle", Kind: AppletKindDatastar, Title: "Wordle"},
		IdentityPath: "thoughts/apps/wordle.md",
		RouteHref:    "/thoughts/_render/app/wordle",
		IFrameSrc:    "/thoughts/_render/app/wordle/app/",
		StatusURL:    "/thoughts/_render/app/wordle/status",
	}
}

func writeAppletManifest(t *testing.T, root, id string) {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nvamos_artifact: applet\napplet:\n  id: " + id + "\n  kind: datastar\n  title: Wordle\n  app_dir: .\n  start_command: [just, build]\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

type sequenceManager struct {
	states []appletruntime.AppletProcessState
	calls  int
}

func (m *sequenceManager) EnsureStarted(context.Context, appletruntime.RuntimeConfig) (appletruntime.AppletProcessState, error) {
	return appletruntime.AppletProcessState{Status: appletruntime.ProcessStatusHealthy}, nil
}
func (m *sequenceManager) Start(context.Context, appletruntime.RuntimeConfig) (appletruntime.ProcessState, error) {
	return appletruntime.AppletProcessState{Status: appletruntime.ProcessStatusHealthy}, nil
}
func (m *sequenceManager) Stop(context.Context, string) error { return nil }
func (m *sequenceManager) Health(context.Context, string) (appletruntime.AppletProcessState, error) {
	if len(m.states) == 0 {
		return appletruntime.AppletProcessState{Status: appletruntime.ProcessStatusStopped}, nil
	}
	idx := m.calls
	if idx >= len(m.states) {
		idx = len(m.states) - 1
	}
	m.calls++
	state := m.states[idx]
	state.AppID = "wordle"
	return state, nil
}
func (m *sequenceManager) ProxyTarget(string) (string, bool) { return "", false }
func (m *sequenceManager) Touch(string, int)                 {}
func (m *sequenceManager) SweepInactive(context.Context, time.Time) ([]appletruntime.AppletProcessState, error) {
	return nil, nil
}
