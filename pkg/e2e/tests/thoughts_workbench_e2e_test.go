package tests

import (
	"testing"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"

	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

func TestThoughtsWorkbench_RootOpensDocumentWorkbenchWithChat(t *testing.T) {
	spec.Feature("Thoughts workbench").
		Scenario("root opens document workbench with chat").
		Given(
			vamos.AuthenticatedAs("playwright@localhost"),
			vamos.WorkspaceFixture("thoughts-workbench.basic").Build(),
			spec.Visit("/"),
			vamos.WaitForFeatureReady("thoughts.workbench"),
		).
		Then(
			spec.Visible(vamos.Sidebar()),
			spec.Visible(vamos.RightRailChatTab()),
			spec.TextAbsent("Session history"),
			vamos.ExpectConsoleClean(),
		).
		Run(t)
}

func TestThoughtsWorkbench_DocumentSidebarNavigationUsesNormalDocumentLinks(t *testing.T) {
	spec.Feature("Thoughts workbench").
		Scenario("document sidebar navigation uses normal document links").
		Given(
			vamos.AuthenticatedAs("playwright@localhost"),
			vamos.WorkspaceFixture("thoughts-workbench.basic").Build(),
			spec.Visit("/thoughts/example.md?context=chat&thread=th_1"),
			vamos.WaitForFeatureReady("thoughts.workbench"),
		).
		When(vamos.FollowFirstSidebarDocumentLink()).
		Then(
			spec.URLContains("context=chat"),
			spec.URLContains("thread=th_1"),
			spec.Visible(vamos.CenterPane()),
		).
		Run(t)
}

func TestThoughtsWorkbench_BreadcrumbParentNavigationWorks(t *testing.T) {
	spec.Feature("Thoughts workbench").
		Scenario("breadcrumb parent navigation works").
		Given(
			vamos.AuthenticatedAs("playwright@localhost"),
			vamos.WorkspaceFixture("thoughts-workbench.basic").Build(),
			spec.Visit("/thoughts/owner/plans/demo/outline.md?context=chat&thread=th_1"),
			vamos.WaitForFeatureReady("thoughts.workbench"),
		).
		When(vamos.FollowFirstBreadcrumbLink()).
		Then(
			spec.URLContains("/thoughts/"),
			spec.Visible(vamos.CenterPane()),
		).
		Run(t)
}

func TestThoughtsWorkbench_WorkbenchReloadPreservesDbLayoutState(t *testing.T) {
	spec.Feature("Thoughts workbench").
		Scenario("workbench reload preserves db layout state").
		Given(
			vamos.AuthenticatedAs("playwright@localhost"),
			vamos.WorkspaceFixture("thoughts-workbench.basic").Build(),
			spec.Visit("/thoughts/example.md?context=chat"),
			vamos.WaitForFeatureReady("thoughts.workbench"),
		).
		When(
			vamos.SwitchTab("thoughts.sidebar.workspaces"),
			vamos.SwitchTab("thoughts.rightRail.chat"),
			vamos.ToggleRegion("thoughts.workbench.sidebar"),
			vamos.FollowFirstSidebarDocumentLink(),
		).
		Then(
			spec.Visible(vamos.Selector("thoughts.sidebar.workspaces")),
			spec.Visible(vamos.RightRailChatTab()),
			spec.Visible(vamos.CenterPane()),
			vamos.ExpectInactiveTabPanelsHidden(),
		).
		Run(t)
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

func runWorkbenchRegionsRemainUsable(t *testing.T, viewport duiruntime.ViewportClass, path string) {
	t.Helper()
	spec.Feature("Thoughts workbench").
		Scenario("workbench regions remain usable across viewport classes").
		Viewport(viewport).
		Given(
			vamos.AuthenticatedAs("playwright@localhost"),
			vamos.WorkspaceFixture("thoughts-workbench.basic").Build(),
			spec.Visit(path),
			vamos.WaitForFeatureReady("thoughts.workbench"),
		).
		Then(
			spec.Visible(vamos.Sidebar()),
			spec.Visible(vamos.CenterPane()),
			spec.Visible(vamos.RightRail()),
			spec.TextAbsent("Session history"),
			vamos.ExpectConsoleClean(),
		).
		Run(t)
}

func runSavedMobileActiveStateDoesNotPinDesktopRefresh(t *testing.T, viewport duiruntime.ViewportClass) {
	t.Helper()
	spec.Feature("Thoughts workbench").
		Scenario("saved mobile active state does not pin desktop refresh").
		Viewport(viewport).
		Given(
			vamos.AuthenticatedAs("playwright@localhost"),
			vamos.WorkspaceFixture("thoughts-workbench.basic").Build(),
			spec.Visit("/thoughts/example.md?context=chat"),
			vamos.WaitForFeatureReady("thoughts.workbench"),
		).
		Then(
			spec.Visible(vamos.Sidebar()),
			spec.Visible(vamos.CenterPane()),
			spec.Visible(vamos.RightRail()),
			vamos.ExpectConsoleClean(),
		).
		Run(t)
}
