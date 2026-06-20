package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"

	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
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
		Expect(spec.ExpectStep(spec.Visible(spec.CSS("iframe[src='/thoughts/_render/html/renderer-demo.html']")))).
		Expect(iframeSandboxOmitsSameOrigin()).
		Visit(vamos.Pages.Path("/thoughts/renderer-demo.csv?context=chat")).
		Expect(vamos.Thoughts.Ready()).
		Expect(spec.TextContains(vamos.Thoughts.CenterPane(), "CSV table")).
		Expect(spec.TextContains(vamos.Thoughts.CenterPane(), "<script>")).
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
			"renderer-demo.md":   "# Renderer Markdown Demo\n\nMarkdown parity content.\n",
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

func iframeSandboxOmitsSameOrigin() spec.Expectation {
	return spec.ExpectStep(spec.Custom("iframe sandbox omits allow-same-origin", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		iframe := ctx.Page.Locator("iframe[src='/thoughts/_render/html/renderer-demo.html']").First()
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
