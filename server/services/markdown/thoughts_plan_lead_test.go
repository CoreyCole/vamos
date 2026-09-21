package markdown

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
)

func TestThoughtsPlanDocIsFullscreenWithPlanLeadChatLink(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Design\n\nBody.\n"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})

	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/thoughts/owner/plans/alpha/design.md",
		nil,
	)
	c := e.NewContext(req, httptest.NewRecorder())
	page := &PageArgs{
		FilePath:  "owner/plans/alpha/design.md",
		UserEmail: "t@example.com",
		ViewerArgs: ViewerArgs{
			RawMarkdown: "# Design\n",
		},
	}
	state, err := svc.buildThoughtsV2WorkbenchState(c, page)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if err := workbench.Workbench(state).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	for _, want := range []string{
		`id="thread-artifact-document"`,
		`<span>Chat about this plan</span>`,
		`href="/rooms/plan/alpha?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md"`,
		`data-testid="view-chat"`,
		`title="View Chat"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in:\n%s", want, html)
		}
	}
	for _, unwanted := range []string{
		"No Chat content yet.",
		`id="workspace-doc-tree-header"`,
		"Related docs",
		"workspaceDocTreeNode_",
	} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("found %q", unwanted)
		}
	}
	overflowStart := strings.Index(html, `data-testid="workbench-overflow-actions"`)
	if overflowStart < 0 {
		t.Fatal("missing overflow actions")
	}
	overflow := html[overflowStart:]
	if end := strings.Index(overflow, `id="thread-artifact-browser"`); end > 0 {
		overflow = overflow[:end]
	}
	if strings.Contains(overflow, "<span>Thoughts</span>") {
		t.Fatalf("Thoughts still in 3-dot on thoughts:\n%s", overflow)
	}
}

func TestThoughtsDocSSRWiresSharedChatNotEmptyRegion(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "docs", "vamos"))
	mustWriteFile(
		t,
		filepath.Join(root, "docs", "vamos", "index.html"),
		[]byte("<html></html>"),
	)
	mustWriteFile(
		t,
		filepath.Join(root, "docs", "vamos", "AGENTS.md"),
		[]byte("# desk\n"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	renderer := &threadWorkbenchTestRenderer{ensureID: "thread-docs"}
	svc.WithWorkbenchThreadRenderer(renderer)

	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/thoughts/docs/vamos/index.html",
		nil,
	)
	c := e.NewContext(req, httptest.NewRecorder())
	page := &PageArgs{
		FilePath:  "docs/vamos/index.html",
		UserEmail: "t@example.com",
		ViewerArgs: ViewerArgs{
			RawMarkdown: "# Docs\n",
		},
	}
	state, err := svc.buildThoughtsV2WorkbenchState(c, page)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if err := workbench.Workbench(state).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	if strings.Contains(html, "No Chat content yet.") {
		t.Fatal("thoughts SSR used EmptyRegion chat copy")
	}
	for _, want := range []string{
		`id="agent-chat-composer"`,
		`id="roster-row-doc-vamos"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(html, `id="thread-chat"`) {
		t.Fatal("thoughts GET must not auto click-in")
	}
	if strings.Contains(html, "Freeform chat") {
		t.Fatal("thoughts/docs/vamos Mode must not be Freeform chat")
	}
	if !strings.Contains(html, ">docs<") {
		t.Fatalf("thoughts/docs/vamos Mode must be docs: %s", html)
	}
	if renderer.lastEnsureDoc != "" {
		t.Fatalf("GET must not ensure, doc = %q", renderer.lastEnsureDoc)
	}
}

func TestThoughtsDocUnresolvedThreadUsesUnavailableNotEmptyRegion(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "docs", "vamos"))
	mustWriteFile(
		t,
		filepath.Join(root, "docs", "vamos", "index.html"),
		[]byte("<html></html>"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})

	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/thoughts/docs/vamos/index.html",
		nil,
	)
	c := e.NewContext(req, httptest.NewRecorder())
	page := &PageArgs{
		FilePath:  "docs/vamos/index.html",
		UserEmail: "t@example.com",
	}
	state, err := svc.buildThoughtsV2WorkbenchState(c, page)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if err := workbench.Workbench(state).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	if strings.Contains(html, "No Chat content yet.") {
		t.Fatal("unresolved thread used EmptyRegion")
	}
	if !strings.Contains(html, thoughtsSharedThreadUnavailable) {
		t.Fatalf("missing unavailable copy in:\n%s", html)
	}
}

func TestThoughtsDocHonorsChatOpenCookieZero(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "docs", "vamos"))
	mustWriteFile(
		t,
		filepath.Join(root, "docs", "vamos", "index.html"),
		[]byte("<html></html>"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{ensureID: "thread-docs"})

	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/thoughts/docs/vamos/index.html",
		nil,
	)
	req.AddCookie(&http.Cookie{Name: workbench.ChatOpenCookie, Value: "0"})
	c := e.NewContext(req, httptest.NewRecorder())
	page := &PageArgs{
		FilePath:  "docs/vamos/index.html",
		UserEmail: "t@example.com",
	}
	state, err := svc.buildThoughtsV2WorkbenchState(c, page)
	if err != nil {
		t.Fatal(err)
	}
	var chatVisible bool
	for _, region := range state.Regions {
		if region.ID == workbench.WorkbenchV2ChatRegionID {
			chatVisible = region.Visible
		}
	}
	if chatVisible {
		t.Fatal("wb2_chat_open=0 must keep chat closed")
	}
}

func TestThoughtsDocHonorsArtifactOpenCookieZero(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "docs", "vamos"))
	mustWriteFile(
		t,
		filepath.Join(root, "docs", "vamos", "index.html"),
		[]byte("<html></html>"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{ensureID: "thread-docs"})

	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/thoughts/docs/vamos/index.html",
		nil,
	)
	req.AddCookie(&http.Cookie{Name: workbench.ArtifactOpenCookie, Value: "0"})
	c := e.NewContext(req, httptest.NewRecorder())
	page := &PageArgs{
		FilePath:  "docs/vamos/index.html",
		UserEmail: "t@example.com",
	}
	state, err := svc.buildThoughtsV2WorkbenchState(c, page)
	if err != nil {
		t.Fatal(err)
	}
	var artifactVisible bool
	for _, region := range state.Regions {
		if region.ID == workbench.WorkbenchV2ArtifactRegionID {
			artifactVisible = region.Visible
		}
	}
	if artifactVisible {
		t.Fatal("wb2_artifact_open=0 must not force ArtifactOpen true")
	}
}
