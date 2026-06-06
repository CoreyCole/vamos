package tests

import (
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
			"E2E Cleaned History",
			"E2E Merged History",
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
		Expect(vamos.ExpectWorkspaceVisible("E2E Primary Only Plan")).
		Expect(vamos.ExpectWorkspaceHidden("E2E datastarui workspace")).
		Expect(vamos.ExpectWorkspacesURLContains(map[string]string{"project": "vamos", "q": "primary-only"})).
		Do(vamos.OpenWorkspacesWithFilters(vamos.WorkspacesStoryFilters{Project: "vamos", Query: "active"})).
		Expect(vamos.ExpectWorkspaceVisible("E2E Active Workspace")).
		Expect(vamos.ExpectWorkspaceHidden("E2E datastarui workspace")).
		Expect(vamos.ExpectWorkspacesURLContains(map[string]string{"project": "vamos", "q": "active"})).
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
