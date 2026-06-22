package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"
	"github.com/playwright-community/playwright-go"

	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

const (
	rendererDemoAppletFrameSelector = `iframe[data-vamos-html-applet][src^='/thoughts/_render/html/renderer-demo.html?theme=']`
	styledAppletFrameSelector       = `iframe[data-vamos-html-applet][src^='/thoughts/_render/html/styled-applet.html?theme=']`
)

func TestThoughtsWorkbench_RootOpensDocumentWorkbenchWithChat(t *testing.T) {
	spec.Story(t, "thoughts workbench root opens document workbench with chat").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture("thoughts-workbench.basic")).
		Visit(vamos.Pages.Root()).
		Expect(vamos.Thoughts.Ready()).
		Expect(vamos.Thoughts.SidebarVisible()).
		Expect(vamos.Thoughts.RightRailChatTabVisible()).
		Expect(spec.ExpectStep(spec.TextAbsent("Session history"))).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestThoughtsWorkbench_DocumentSidebarNavigationUsesNormalDocumentLinks(t *testing.T) {
	spec.Story(t, "thoughts workbench document sidebar navigation uses normal document links").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture("thoughts-workbench.basic")).
		Visit(vamos.Pages.Path("/thoughts/example.md?context=chat&thread=th_1")).
		Expect(vamos.Thoughts.Ready()).
		Do(vamos.FollowFirstSidebarDocumentLink()).
		Expect(spec.ExpectStep(spec.URLContains("context=chat"))).
		Expect(spec.ExpectStep(spec.URLContains("thread=th_1"))).
		Expect(vamos.Thoughts.CenterPaneVisible()).
		Run()
}

func TestThoughtsWorkbench_BreadcrumbParentNavigationWorks(t *testing.T) {
	spec.Story(t, "thoughts workbench breadcrumb parent navigation works").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture("thoughts-workbench.basic")).
		Visit(vamos.Pages.Path("/thoughts/owner/plans/demo/outline.md?context=chat&thread=th_1")).
		Expect(vamos.Thoughts.Ready()).
		Do(vamos.FollowFirstBreadcrumbLink()).
		Expect(spec.ExpectStep(spec.URLContains("/thoughts/"))).
		Expect(vamos.Thoughts.CenterPaneVisible()).
		Run()
}

func TestThoughtsWorkbench_RendererFormatsShowExpectedWorkbenchStates(t *testing.T) {
	spec.Story(t, "thoughts workbench renderer formats show expected states").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture("thoughts-workbench.basic")).
		Do(seedRendererThoughtsFiles()).
		Visit(vamos.Pages.Path("/thoughts/renderer-demo.md?context=chat")).
		Expect(vamos.Thoughts.Ready()).
		Expect(spec.TextContains(vamos.Thoughts.CenterPane(), "Renderer Markdown Demo")).
		Visit(vamos.Pages.Path("/thoughts/renderer-demo.html?context=chat")).
		Expect(vamos.Thoughts.Ready()).
		Expect(spec.ExpectStep(spec.TextAbsent("HTML applet:"))).
		Expect(spec.ExpectStep(spec.Visible(spec.CSS(rendererDemoAppletFrameSelector)))).
		Expect(iframeSandboxOmitsSameOrigin()).
		Expect(htmlAppletFillsDocumentSurface(rendererDemoAppletFrameSelector)).
		Visit(vamos.Pages.Path("/thoughts/renderer-demo.csv?context=chat")).
		Expect(vamos.Thoughts.Ready()).
		Expect(spec.ExpectStep(spec.TextAbsent("CSV table:"))).
		Expect(spec.TextContains(vamos.Thoughts.CenterPane(), "<script>")).
		Expect(csvRendererUsesDocumentTableStyles()).
		Visit(vamos.Pages.Path("/thoughts/renderer-demo.json?context=chat")).
		Expect(vamos.Thoughts.Ready()).
		Expect(spec.TextContains(vamos.Thoughts.CenterPane(), "Unsupported document type")).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestThoughtsWorkbench_HTMLChildRouteServesAppletContent(t *testing.T) {
	spec.Story(t, "thoughts workbench HTML child route serves applet content").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture("thoughts-workbench.basic")).
		Do(seedRendererThoughtsFiles()).
		Visit(vamos.Pages.Path("/thoughts/_render/html/renderer-demo.html")).
		Expect(spec.ExpectStep(spec.TextAbsent("HTML applet:"))).
		Expect(spec.ExpectStep(spec.TextAbsent("thoughts-markdown-scroll-region"))).
		Expect(spec.TextContains(spec.CSS("body"), "Renderer HTML Applet")).
		Expect(spec.TextContains(spec.CSS("body"), "Datastar explicit import placeholder")).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestThoughtsWorkbench_HTMLAppletUsesVamosStylesAndTheme(t *testing.T) {
	spec.Story(t, "thoughts workbench HTML applet uses Vamos styles and theme").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture("thoughts-workbench.basic")).
		Do(seedStyledHTMLAppletFile()).
		Visit(vamos.Pages.Path("/thoughts/styled-applet.html?context=chat")).
		Expect(vamos.Thoughts.Ready()).
		Expect(htmlAppletFrameHasThemeQuery()).
		Expect(htmlAppletChildHasInitialTheme()).
		Expect(htmlAppletUsesSharedStyles()).
		Expect(htmlAppletLocalOverrideWins()).
		Do(toggleThemeFromAvatar()).
		Expect(htmlAppletChildThemeChanged()).
		Expect(iframeSandboxOmitsSameOriginFor(styledAppletFrameSelector)).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestThoughtsWorkbench_WorkbenchReloadPreservesDbLayoutState(t *testing.T) {
	spec.Story(t, "thoughts workbench reload preserves db layout state").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture("thoughts-workbench.basic")).
		Visit(vamos.Pages.Path("/thoughts/example.md?context=chat")).
		Expect(vamos.Thoughts.Ready()).
		Do(vamos.SwitchTab("thoughts.sidebar.workspaces")).
		Do(vamos.SwitchTab("thoughts.rightRail.chat")).
		Do(vamos.ToggleRegion("thoughts.workbench.sidebar")).
		Do(vamos.FollowFirstSidebarDocumentLink()).
		Do(vamos.SwitchTab("thoughts.sidebar.workspaces")).
		Expect(spec.ExpectStep(spec.Visible(vamos.Thoughts.WorkspacesTab()))).
		Expect(vamos.Thoughts.RightRailChatTabVisible()).
		Expect(vamos.Thoughts.CenterPaneVisible()).
		Expect(vamos.ExpectInactiveTabPanelsHidden()).
		Run()
}

func TestThoughtsWorkbench_WorkspacesPageProjectFilterIncludesRelatedPlans(t *testing.T) {
	spec.Story(t, "thoughts workbench workspaces page project filter includes related plans").
		App(vamos.App()).
		As(vamos.Robot).
		Do(vamos.SeedMultiProjectPlanFilteringFixture("vamos", "datastarui")).
		Do(vamos.OpenWorkspacesWithProjectFilter("datastarui")).
		Expect(vamos.ExpectProjectFilteredPlanBadgesVisible("workspaces page", "vamos", "datastarui")).
		Run()
}

func TestThoughtsWorkbench_WorkspacesPageSearchHistoryAndProjectFilters(t *testing.T) {
	spec.Story(t, "thoughts workbench workspaces page search history and filters").
		App(vamos.App()).
		As(vamos.Robot).
		Do(vamos.SeedMultiProjectPlanFilteringFixture("vamos", "datastarui")).
		Do(vamos.OpenWorkspacesWithFilters(vamos.WorkspacesStoryFilters{})).
		Expect(vamos.ExpectWorkspaceVisible("E2E Active Workspace")).
		Expect(vamos.ExpectWorkspaceHidden("E2E Merged History")).
		Expect(vamos.ExpectWorkspaceHidden("E2E Cleaned History")).
		Do(vamos.OpenWorkspacesWithFilters(vamos.WorkspacesStoryFilters{History: "all"})).
		Expect(vamos.ExpectWorkspaceVisible("E2E Merged History")).
		Expect(vamos.ExpectWorkspaceVisible("E2E Cleaned History")).
		Do(vamos.OpenWorkspacesWithFilters(vamos.WorkspacesStoryFilters{History: "all", Sort: "name_asc"})).
		Expect(vamos.ExpectWorkspacesInOrder(
			"E2E Active Workspace",
			"E2E Merged History",
			"E2E Cleaned History",
		)).
		Do(vamos.OpenWorkspacesWithFilters(vamos.WorkspacesStoryFilters{History: "all", Group: "needs_attention"})).
		Expect(vamos.ExpectWorkspaceVisible("E2E Active Workspace")).
		Expect(vamos.ExpectWorkspaceVisible("E2E Merged History")).
		Expect(vamos.ExpectWorkspaceHidden("E2E Cleaned History")).
		Expect(vamos.ExpectWorkspacesURLContains(map[string]string{"history": "all", "group": "needs_attention"})).
		Do(vamos.OpenWorkspacesWithFilters(vamos.WorkspacesStoryFilters{Project: "datastarui", Query: "multi-project"})).
		Expect(vamos.ExpectProjectFilteredPlanBadgesVisible("workspaces page", "vamos", "datastarui")).
		Expect(vamos.ExpectWorkspaceHidden("E2E Primary Only Plan")).
		Expect(vamos.ExpectWorkspacesURLContains(map[string]string{"project": "datastarui", "q": "multi-project"})).
		Do(vamos.OpenWorkspacesWithFilters(vamos.WorkspacesStoryFilters{Project: "vamos", Query: "primary-only"})).
		Expect(vamos.ExpectWorkspaceVisible("e2e-vamos-e2e-primary-only-filter")).
		Expect(vamos.ExpectWorkspaceHidden("E2E datastarui workspace")).
		Expect(vamos.ExpectWorkspacesURLContains(map[string]string{"project": "vamos", "q": "primary-only"})).
		Do(vamos.OpenWorkspacesWithFilters(vamos.WorkspacesStoryFilters{Project: "vamos", Query: "active"})).
		Expect(vamos.ExpectWorkspaceVisible("E2E Active Workspace")).
		Expect(vamos.ExpectWorkspaceHidden("E2E datastarui workspace")).
		Expect(vamos.ExpectWorkspacesURLContains(map[string]string{"project": "vamos", "q": "active"})).
		Do(vamos.OpenWorkspacesWithFilters(vamos.WorkspacesStoryFilters{History: "all"})).
		Do(vamos.ChangeWorkspacesFilters(vamos.WorkspacesStoryFilters{History: "all", Query: "primary-only", Sort: "name_asc"})).
		Expect(vamos.ExpectWorkspaceVisible("e2e-vamos-e2e-primary-only-filter")).
		Expect(vamos.ExpectWorkspaceHidden("E2E datastarui workspace")).
		Expect(vamos.ExpectWorkspacesURLContains(map[string]string{"history": "all", "q": "primary-only", "sort": "name_asc"})).
		Run()
}

func TestThoughtsWorkbench_WorkbenchRegionsRemainUsableAcrossViewportClassesMobile(t *testing.T) {
	runWorkbenchRegionsRemainUsable(t, duiruntime.ViewportMobile, "/")
}

func TestThoughtsWorkbench_WorkbenchRegionsRemainUsableAcrossViewportClassesMobileThoughts(t *testing.T) {
	runWorkbenchRegionsRemainUsable(t, duiruntime.ViewportMobile, "/thoughts")
}

func TestThoughtsWorkbench_WorkbenchRegionsRemainUsableAcrossViewportClassesMobileThoughtsExampleMdContextChat(t *testing.T) {
	runWorkbenchRegionsRemainUsable(t, duiruntime.ViewportMobile, "/thoughts/example.md?context=chat")
}

func TestThoughtsWorkbench_WorkbenchRegionsRemainUsableAcrossViewportClassesDesktopHalf(t *testing.T) {
	runWorkbenchRegionsRemainUsable(t, duiruntime.ViewportDesktopHalf, "/")
}

func TestThoughtsWorkbench_WorkbenchRegionsRemainUsableAcrossViewportClassesDesktopHalfThoughts(t *testing.T) {
	runWorkbenchRegionsRemainUsable(t, duiruntime.ViewportDesktopHalf, "/thoughts")
}

func TestThoughtsWorkbench_WorkbenchRegionsRemainUsableAcrossViewportClassesDesktopHalfThoughtsExampleMdContextChat(t *testing.T) {
	runWorkbenchRegionsRemainUsable(t, duiruntime.ViewportDesktopHalf, "/thoughts/example.md?context=chat")
}

func TestThoughtsWorkbench_WorkbenchRegionsRemainUsableAcrossViewportClassesDesktopFull(t *testing.T) {
	runWorkbenchRegionsRemainUsable(t, duiruntime.ViewportDesktopFull, "/")
}

func TestThoughtsWorkbench_WorkbenchRegionsRemainUsableAcrossViewportClassesDesktopFullThoughts(t *testing.T) {
	runWorkbenchRegionsRemainUsable(t, duiruntime.ViewportDesktopFull, "/thoughts")
}

func TestThoughtsWorkbench_WorkbenchRegionsRemainUsableAcrossViewportClassesDesktopFullThoughtsExampleMdContextChat(t *testing.T) {
	runWorkbenchRegionsRemainUsable(t, duiruntime.ViewportDesktopFull, "/thoughts/example.md?context=chat")
}

func TestThoughtsWorkbench_SavedMobileActiveStateDoesNotPinDesktopRefreshMobileThoughtsExampleMdContextChat(t *testing.T) {
	runSavedMobileActiveStateDoesNotPinDesktopRefresh(t, duiruntime.ViewportMobile)
}

func TestThoughtsWorkbench_SavedMobileActiveStateDoesNotPinDesktopRefreshDesktopFullThoughtsExampleMdContextChat(t *testing.T) {
	runSavedMobileActiveStateDoesNotPinDesktopRefresh(t, duiruntime.ViewportDesktopFull)
}

func seedRendererThoughtsFiles() spec.Step {
	return spec.Custom("seed renderer thoughts files", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		root := strings.TrimSpace(os.Getenv("VAMOS_E2E_THOUGHTS_ROOT"))
		if root == "" {
			root = filepath.Join(ctx.Config.RepoRoot, "thoughts")
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		files := map[string]string{
			"renderer-demo.md":   "# Renderer Markdown Demo\n\nMarkdown parity content.\n\n| name | value |\n| --- | --- |\n| alpha | beta |\n",
			"renderer-demo.csv":  "name,value\n<script>,escaped text\n",
			"renderer-demo.json": `{"unsupported": true}`,
			"renderer-demo.html": `<!doctype html>
<html>
<head>
  <title>Renderer HTML Applet</title>
  <style>body { background: rgb(1, 2, 3); }</style>
  <script type="module" src="/js/datastar-pro-v1.js"></script>
</head>
<body>
  <h1>Renderer HTML Applet</h1>
  <p>Datastar explicit import placeholder</p>
  <script>
    try { window.parent.document.body.dataset.htmlAppletEscape = "bad" } catch (e) { document.body.dataset.parentBlocked = "true" }
  </script>
</body>
</html>
`,
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func seedStyledHTMLAppletFile() spec.Step {
	return spec.Custom("seed styled HTML applet file", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		root := strings.TrimSpace(os.Getenv("VAMOS_E2E_THOUGHTS_ROOT"))
		if root == "" {
			root = filepath.Join(ctx.Config.RepoRoot, "thoughts")
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		content := `<!doctype html>
<html>
<head>
  <title>Styled HTML Applet</title>
  <link rel="stylesheet" href="/css/out.css">
  <script type="module" src="/js/vamos-html-applet.js"></script>
  <style>
    #override-check { color: rgb(12, 34, 56); }
  </style>
</head>
<body>
  <main class="bg-background text-foreground">
    <h1>Styled HTML Applet</h1>
    <p id="token-check" class="text-foreground">Token styled text</p>
    <p id="override-check" class="text-foreground">Override wins</p>
    <script>
      try { window.parent.document.body.dataset.htmlAppletEscape = "bad" }
      catch (e) { document.body.dataset.parentBlocked = "true" }
    </script>
  </main>
</body>
</html>
`
		if err := os.WriteFile(filepath.Join(root, "styled-applet.html"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	})
}

func csvRendererUsesDocumentTableStyles() spec.Expectation {
	return spec.ExpectStep(spec.Custom("CSV table uses shared document table styles", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		csvWrapper := ctx.Page.Locator(".document-table-content .table-wrapper").First()
		if err := csvWrapper.WaitFor(); err != nil {
			t.Fatalf("CSV table wrapper missing: %v", err)
		}
		borderRadius, err := csvWrapper.Evaluate("el => getComputedStyle(el).borderRadius", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := borderRadius.(string); got == "" || got == "0px" {
			t.Fatalf("CSV wrapper missing shared rounded style, borderRadius=%q", got)
		}
		borderWidth, err := csvWrapper.Evaluate("el => getComputedStyle(el).borderTopWidth", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := borderWidth.(string); got == "" || got == "0px" {
			t.Fatalf("CSV wrapper missing shared border style, borderTopWidth=%q", got)
		}
		headBackground, err := ctx.Page.Locator(".document-table-content thead").First().Evaluate("el => getComputedStyle(el).backgroundColor", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := headBackground.(string); got == "" || got == "rgba(0, 0, 0, 0)" {
			t.Fatalf("CSV header missing shared background style, background=%q", got)
		}
	}))
}

func htmlAppletFillsDocumentSurface(selector string) spec.Expectation {
	return spec.ExpectStep(spec.Custom("HTML applet fills document surface", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		iframe := ctx.Page.Locator(selector).First()
		if err := iframe.WaitFor(); err != nil {
			t.Fatal(err)
		}
		classes, err := iframe.GetAttribute("class")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(classes, "max-w-6xl") || strings.Contains(classes, "mx-auto") {
			t.Fatalf("iframe has capped layout classes: %q", classes)
		}
		metrics, err := iframe.Evaluate(`el => {
			const iframe = el.getBoundingClientRect();
			const scroll = document.querySelector('#thoughts-markdown-scroll-region').getBoundingClientRect();
			return { iframeWidth: iframe.width, iframeHeight: iframe.height, scrollWidth: scroll.width, scrollHeight: scroll.height };
		}`, nil)
		if err != nil {
			t.Fatal(err)
		}
		m := metrics.(map[string]any)
		iframeWidth := numberAsFloat64(t, m["iframeWidth"])
		scrollWidth := numberAsFloat64(t, m["scrollWidth"])
		iframeHeight := numberAsFloat64(t, m["iframeHeight"])
		scrollHeight := numberAsFloat64(t, m["scrollHeight"])
		if iframeWidth < scrollWidth*0.70 {
			t.Fatalf("iframe too narrow: iframe=%f scroll=%f", iframeWidth, scrollWidth)
		}
		if iframeHeight < scrollHeight*0.50 {
			t.Fatalf("iframe too short: iframe=%f scroll=%f", iframeHeight, scrollHeight)
		}
	}))
}

func numberAsFloat64(t testing.TB, value any) float64 {
	t.Helper()
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	default:
		t.Fatalf("expected numeric metric, got %T (%v)", value, value)
		return 0
	}
}

func htmlAppletFrameHasThemeQuery() spec.Expectation {
	return spec.ExpectStep(spec.Custom("HTML applet frame has theme query", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		src := htmlAppletFrameSrc(t, ctx, styledAppletFrameSelector)
		if !strings.Contains(src, "theme=dark") && !strings.Contains(src, "theme=light") {
			t.Fatalf("iframe src missing theme query: %q", src)
		}
	}))
}

func htmlAppletChildHasInitialTheme() spec.Expectation {
	return spec.ExpectStep(spec.Custom("HTML applet child applies initial theme", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		src := htmlAppletFrameSrc(t, ctx, styledAppletFrameSelector)
		wantDark := strings.Contains(src, "theme=dark")
		if wantDark {
			if err := ctx.Page.FrameLocator(styledAppletFrameSelector).Locator("html.dark").WaitFor(); err != nil {
				t.Fatalf("child did not apply initial dark theme from %q: %v", src, err)
			}
		}
		gotDark := htmlAppletChildDark(t, ctx, styledAppletFrameSelector)
		if gotDark != wantDark {
			t.Fatalf("child dark=%v, want %v from %q", gotDark, wantDark, src)
		}
		if err := ctx.Page.FrameLocator(styledAppletFrameSelector).Locator("body[data-parent-blocked='true']").WaitFor(); err != nil {
			t.Fatalf("child did not record blocked parent DOM access: %v", err)
		}
	}))
}

func htmlAppletUsesSharedStyles() spec.Expectation {
	return spec.ExpectStep(spec.Custom("HTML applet uses shared Vamos styles", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		background, err := ctx.Page.FrameLocator(styledAppletFrameSelector).Locator("main").Evaluate("el => getComputedStyle(el).backgroundColor", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := background.(string); got == "" || got == "rgba(0, 0, 0, 0)" {
			t.Fatalf("shared bg-background style did not resolve, background=%q", got)
		}
	}))
}

func htmlAppletLocalOverrideWins() spec.Expectation {
	return spec.ExpectStep(spec.Custom("HTML applet local override wins", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		color, err := ctx.Page.FrameLocator(styledAppletFrameSelector).Locator("#override-check").Evaluate("el => getComputedStyle(el).color", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := color.(string); got != "rgb(12, 34, 56)" {
			t.Fatalf("override color=%q, want rgb(12, 34, 56)", got)
		}
	}))
}

func toggleThemeFromAvatar() spec.Step {
	return spec.Custom("toggle parent theme", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		initial := htmlAppletChildDark(t, ctx, styledAppletFrameSelector)
		ctx.Memory["styled_applet_initial_dark"] = boolString(initial)
		trigger := ctx.Page.Locator("header [data-slot='dropdown-menu-trigger'][data-on\\:click*='user_profile']").First()
		if err := trigger.Click(); err != nil {
			t.Fatal(err)
		}
		content := ctx.Page.Locator("[data-slot='dropdown-menu-content'][data-show='$user_profile.open']").First()
		if err := content.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
			t.Fatalf("avatar menu did not become visible: %v", err)
		}
		if err := content.GetByText("Toggle theme").First().Click(); err != nil {
			t.Fatal(err)
		}
	})
}

func htmlAppletChildThemeChanged() spec.Expectation {
	return spec.ExpectStep(spec.Custom("HTML applet child follows parent theme toggle", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		initial := ctx.Memory["styled_applet_initial_dark"] == "true"
		for i := 0; i < 20; i++ {
			if got := htmlAppletChildDark(t, ctx, styledAppletFrameSelector); got != initial {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("child dark class did not change from %v after parent toggle", initial)
	}))
}

func htmlAppletFrameSrc(t testing.TB, ctx *duiruntime.Context, selector string) string {
	t.Helper()
	iframe := ctx.Page.Locator(selector).First()
	if err := iframe.WaitFor(); err != nil {
		t.Fatal(err)
	}
	src, err := iframe.GetAttribute("src")
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func htmlAppletChildDark(t testing.TB, ctx *duiruntime.Context, selector string) bool {
	t.Helper()
	value, err := ctx.Page.FrameLocator(selector).Locator("html").Evaluate("el => el.classList.contains('dark')", nil)
	if err != nil {
		t.Fatal(err)
	}
	return value.(bool)
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func iframeSandboxOmitsSameOrigin() spec.Expectation {
	return iframeSandboxOmitsSameOriginFor(rendererDemoAppletFrameSelector)
}

func iframeSandboxOmitsSameOriginFor(selector string) spec.Expectation {
	return spec.ExpectStep(spec.Custom("iframe sandbox omits allow-same-origin", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		iframe := ctx.Page.Locator(selector).First()
		if err := iframe.WaitFor(); err != nil {
			t.Fatal(err)
		}
		sandbox, err := iframe.GetAttribute("sandbox")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sandbox, "allow-scripts") {
			t.Fatalf("sandbox=%q missing allow-scripts", sandbox)
		}
		if strings.Contains(sandbox, "allow-same-origin") {
			t.Fatalf("sandbox=%q permits same-origin", sandbox)
		}
	}))
}

func runWorkbenchRegionsRemainUsable(t *testing.T, viewport duiruntime.ViewportClass, p string) {
	t.Helper()
	spec.Story(t, "thoughts workbench regions remain usable across viewport classes").
		App(vamos.App()).
		Viewport(viewport).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture("thoughts-workbench.basic")).
		Visit(vamos.Pages.Path(p)).
		Expect(vamos.Thoughts.Ready()).
		Expect(vamos.Thoughts.SidebarVisible()).
		Expect(vamos.Thoughts.CenterPaneVisible()).
		Expect(vamos.Thoughts.RightRailVisible()).
		Expect(spec.ExpectStep(spec.TextAbsent("Session history"))).
		Expect(vamos.Console.Clean()).
		Run()
}

func runSavedMobileActiveStateDoesNotPinDesktopRefresh(t *testing.T, viewport duiruntime.ViewportClass) {
	t.Helper()
	spec.Story(t, "thoughts workbench saved mobile active state does not pin desktop refresh").
		App(vamos.App()).
		Viewport(viewport).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture("thoughts-workbench.basic")).
		Visit(vamos.Pages.Path("/thoughts/example.md?context=chat")).
		Expect(vamos.Thoughts.Ready()).
		Expect(vamos.Thoughts.SidebarVisible()).
		Expect(vamos.Thoughts.CenterPaneVisible()).
		Expect(vamos.Thoughts.RightRailVisible()).
		Expect(vamos.Console.Clean()).
		Run()
}
