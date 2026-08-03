package markdown

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/layouts/workbench"
)

type recordingHermesThreadsRenderer struct {
	request     HermesThreadsRenderRequest
	replacement string
	err         error
}

func (renderer *recordingHermesThreadsRenderer) RenderHermesThreadsPanel(
	_ context.Context,
	request HermesThreadsRenderRequest,
) (templ.Component, HermesThreadsURLReplacement, error) {
	renderer.request = request
	if renderer.err != nil {
		return nil, HermesThreadsURLReplacement{}, renderer.err
	}
	return templ.Raw(`<div id="recorded-hermes-threads">threads</div>`), HermesThreadsURLReplacement{URL: renderer.replacement}, nil
}

type recordingEmbeddedChatRenderer struct {
	replacement string
}

func (renderer *recordingEmbeddedChatRenderer) RenderEmbeddedChatPanel(
	_ context.Context,
	_ EmbeddedChatRenderRequest,
) (templ.Component, EmbeddedChatURLReplacement, error) {
	return templ.Raw(`<div id="recorded-embedded-chat">chat</div>`), EmbeddedChatURLReplacement{URL: renderer.replacement}, nil
}

func newThoughtsThreadsContext(rawURL string) echo.Context {
	req := httptest.NewRequest("GET", rawURL, nil)
	return echo.New().NewContext(req, httptest.NewRecorder())
}

func TestThoughtsContextThreadsOpensSplitThreadsRail(t *testing.T) {
	renderer := &recordingHermesThreadsRenderer{}
	service := &Service{hermesThreadsRenderer: renderer}
	context := newThoughtsThreadsContext("/thoughts/owner/plans/alpha/design.md?context=threads")
	page := &PageArgs{
		FilePath:  "owner/plans/alpha/design.md",
		UserEmail: "reader@example.com",
		WorkbenchLinkState: ThoughtsWorkbenchLinkState{
			Context: "threads", SelectedPlanPath: "owner/plans/alpha",
		},
		QRSPIMetadata: QRSPIMetadata{PlanDir: "thoughts/owner/plans/alpha"},
	}

	state, err := service.buildThoughtsWorkbenchState(context, page)
	if err != nil {
		t.Fatal(err)
	}
	if state.View != workbench.WorkbenchViewSplit || state.ContextMode != "threads" {
		t.Fatalf("state view/context = %q/%q", state.View, state.ContextMode)
	}
	if state.Regions[2].Kind != workbench.RegionArtifact || !state.Regions[2].Visible {
		t.Fatalf("right region = %#v", state.Regions[2])
	}
}

func TestThoughtsHermesRendererReceivesSelectedPathHintAndSeparateSelections(t *testing.T) {
	renderer := &recordingHermesThreadsRenderer{}
	service := &Service{hermesThreadsRenderer: renderer}
	context := newThoughtsThreadsContext("/thoughts/owner/plans/alpha/design.md?context=threads&chat_workspace=ws_1&thread=chat_1&run=run_1&hermes_thread=hermes_1")
	state := ThoughtsWorkbenchLinkStateFromRequest(context, "owner/plans/alpha")
	page := &PageArgs{
		FilePath: "owner/plans/alpha/design.md", UserEmail: "reader@example.com",
		WorkbenchLinkState: state,
		QRSPIMetadata:      QRSPIMetadata{PlanDir: "thoughts/owner/plans/alpha"},
	}

	if _, err := service.buildThoughtsWorkbenchState(context, page); err != nil {
		t.Fatal(err)
	}
	request := renderer.request
	if request.SelectedPlanPath != "owner/plans/alpha" || request.PlanHint != "thoughts/owner/plans/alpha" {
		t.Fatalf("selected/hint = %q/%q", request.SelectedPlanPath, request.PlanHint)
	}
	if request.LinkState.ChatThreadID != "chat_1" || request.LinkState.HermesThreadID != "hermes_1" {
		t.Fatalf("selections = %#v", request.LinkState)
	}
}

func TestThoughtsHermesRendererRejectsDifferentContainedPlanHint(t *testing.T) {
	renderer := &recordingHermesThreadsRenderer{err: errors.New("plan hint does not identify selected plan")}
	service := &Service{hermesThreadsRenderer: renderer}
	context := newThoughtsThreadsContext("/thoughts/owner/plans/alpha/design.md?context=threads")
	page := &PageArgs{
		FilePath:           "owner/plans/alpha/design.md",
		WorkbenchLinkState: ThoughtsWorkbenchLinkState{Context: "threads", SelectedPlanPath: "owner/plans/alpha"},
		QRSPIMetadata:      QRSPIMetadata{PlanDir: "thoughts/owner/plans/beta"},
	}

	if _, err := service.buildThoughtsWorkbenchState(context, page); err == nil {
		t.Fatal("different contained plan hint was accepted")
	}
	if renderer.request.SelectedPlanPath != "owner/plans/alpha" || renderer.request.PlanHint != "thoughts/owner/plans/beta" {
		t.Fatalf("request = %#v", renderer.request)
	}
}

func TestThoughtsNoPlanRendersLocalEmptyThreadsPanelWithoutDiscovery(t *testing.T) {
	renderer := &recordingHermesThreadsRenderer{}
	service := &Service{hermesThreadsRenderer: renderer}
	context := newThoughtsThreadsContext("/thoughts/notes.md?context=threads")
	page := &PageArgs{
		FilePath:           "notes.md",
		WorkbenchLinkState: ThoughtsWorkbenchLinkState{Context: "threads"},
	}

	if _, err := service.buildThoughtsWorkbenchState(context, page); err != nil {
		t.Fatal(err)
	}
	if !renderer.request.NoPlan || renderer.request.SelectedPlanPath != "" {
		t.Fatalf("request = %#v", renderer.request)
	}
}

func TestThoughtsHermesSelectionRejectsMalformedOrCrossPlanThread(t *testing.T) {
	state := ThoughtsWorkbenchLinkState{
		Context: "threads", ChatWorkspaceID: "ws", ChatThreadID: "chat", ChatRunID: "run",
		HermesThreadID: "hermes", SelectedPlanPath: "owner/plans/alpha",
	}.WithoutHermesThread()
	got := state.Preserve("/thoughts/owner/plans/beta/design.md?hermes_thread=bad%2Fid")
	query, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if query.Query().Get("hermes_thread") != "" {
		t.Fatalf("cross-plan Hermes selection survived: %q", got)
	}
	for key, want := range map[string]string{"chat_workspace": "ws", "thread": "chat", "run": "run", "context": "threads"} {
		if value := query.Query().Get(key); value != want {
			t.Fatalf("query[%s] = %q, want %q in %q", key, value, want, got)
		}
	}
}

func TestThoughtsRendererURLReplacementsComposeIndependentStateFamilies(t *testing.T) {
	for _, test := range []struct {
		name        string
		isDirectory bool
	}{
		{name: "document"},
		{name: "directory", isDirectory: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			chat := &recordingEmbeddedChatRenderer{replacement: "/thoughts/owner/plans/alpha/design.md?context=chat&thread=chat_canonical&run=run_canonical&unrelated=keep"}
			hermes := &recordingHermesThreadsRenderer{replacement: "/thoughts/owner/plans/alpha/design.md?context=chat&chat_workspace=stale&thread=chat_stale&run=run_stale&unrelated=keep"}
			service := &Service{embeddedChatRenderer: chat, hermesThreadsRenderer: hermes}
			context := newThoughtsThreadsContext("/thoughts/owner/plans/alpha/design.md?context=chat&chat_workspace=ws_stale&thread=chat_stale&run=run_stale&hermes_thread=bad%2Fid&unrelated=keep")
			state := ThoughtsWorkbenchLinkStateFromRequest(context, "owner/plans/alpha")
			var workbenchState workbench.WorkbenchState
			var err error
			if test.isDirectory {
				workbenchState, err = service.buildThoughtsDirectoryWorkbenchState(context, &DirectoryArgs{Path: "owner/plans/alpha", WorkbenchLinkState: state})
			} else {
				workbenchState, err = service.buildThoughtsWorkbenchState(context, &PageArgs{FilePath: "owner/plans/alpha/design.md", WorkbenchLinkState: state})
			}
			if err != nil {
				t.Fatal(err)
			}
			var rendered strings.Builder
			if err := workbench.Workbench(workbenchState).Render(t.Context(), &rendered); err != nil {
				t.Fatal(err)
			}
			html := rendered.String()
			if count := strings.Count(html, `data-replace-url=`); count != 1 {
				t.Fatalf("URL replacement operation count = %d, want 1: %s", count, html)
			}
			start := strings.Index(html, `data-replace-url="`)
			if start < 0 {
				t.Fatalf("composed replacement missing: %s", html)
			}
			replacement := html[start:]
			if end := strings.Index(replacement[len(`data-replace-url="`):], `"`); end >= 0 {
				replacement = replacement[:len(`data-replace-url="`)+end]
			}
			for _, want := range []string{"context=chat", "thread=chat_canonical", "run=run_canonical", "unrelated=keep"} {
				if !strings.Contains(replacement, want) {
					t.Fatalf("composed replacement missing %q: %s", want, replacement)
				}
			}
			for _, unwanted := range []string{"hermes_thread=", "chat_workspace=", "thread=chat_stale", "run=run_stale"} {
				if strings.Contains(replacement, unwanted) {
					t.Fatalf("composed replacement retained %q: %s", unwanted, replacement)
				}
			}
		})
	}
}

func TestThoughtsWorkbenchNavigationBuildersPreserveIndependentSelections(t *testing.T) {
	state := ThoughtsWorkbenchLinkState{
		Context: "threads", ChatWorkspaceID: "ws_1", ChatThreadID: "chat_1", ChatRunID: "run_1",
		HermesThreadID: "hermes_1", SelectedPlanPath: "owner/plans/alpha",
	}
	page := &PageArgs{
		FilePath: "owner/plans/alpha/design.md", WorkbenchLinkState: state,
		FileTree: []FileTreeNode{{Name: "outline.md", Path: "owner/plans/alpha/outline.md"}},
	}
	sidebarHref := BuildThoughtsSidebarArgs(page).Files.Nodes[0].Href
	tree := BuildWorkspaceDocTreeArgs("ws", page.FilePath, workbench.DocEntryModeThoughts, []db.WorkspaceDoc{{
		WorkspaceID: "ws", DocPath: "owner/plans/alpha/outline.md", RelPath: ".", Kind: string(workbench.WorkspaceDocKindFile),
	}}, state)
	if tree == nil || len(tree.Nodes) != 1 {
		t.Fatalf("workspace tree = %#v", tree)
	}
	directoryHref := DirectoryItemHref(DirectoryItem{Name: "review", Path: "owner/plans/alpha/review", IsDir: true}, state)
	service := &Service{}
	context := newThoughtsThreadsContext(state.Preserve("/thoughts/owner/plans/alpha/design.md"))
	workbenchState, err := service.buildThoughtsWorkbenchState(context, page)
	if err != nil {
		t.Fatal(err)
	}
	hrefs := []string{sidebarHref, tree.Nodes[0].Href, directoryHref}
	for _, href := range hrefs {
		parsed, err := url.Parse(href)
		if err != nil {
			t.Fatal(err)
		}
		query := parsed.Query()
		for key, want := range map[string]string{"chat_workspace": "ws_1", "thread": "chat_1", "run": "run_1", "hermes_thread": "hermes_1"} {
			if query.Get(key) != want {
				t.Fatalf("%q query[%s] = %q, want %q", href, key, query.Get(key), want)
			}
		}
	}
	var rendered strings.Builder
	if err := workbench.Workbench(workbenchState).Render(t.Context(), &rendered); err != nil {
		t.Fatal(err)
	}
	html := rendered.String()
	for _, contextMode := range []string{"chat", "comments", "threads"} {
		if !strings.Contains(html, "context="+contextMode) {
			t.Fatalf("rendered workbench missing %s tab link: %s", contextMode, html)
		}
	}
	for _, want := range []string{"chat_workspace=ws_1", "thread=chat_1", "run=run_1", "hermes_thread=hermes_1"} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered workbench tabs missing %q: %s", want, html)
		}
	}
}
