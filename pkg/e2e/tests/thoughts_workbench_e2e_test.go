package tests

import (
	"testing"

	"github.com/coreycole/datastarui/e2e/spec"

	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

func TestThoughtsWorkbench_RootOpensDocumentWorkbenchWithChat(t *testing.T) {
	spec.Feature("Thoughts workbench").
		Scenario("root opens document workbench with chat").
		Given(
			vamos.AuthenticatedAs("playwright@localhost"),
			vamos.WorkspaceFixture("thoughts-workbench.basic").Build(),
			spec.OpenPage(vamos.ThoughtsRootPage()),
		).
		Then(
			vamos.WaitForFeatureReady("thoughts.workbench"),
			spec.Visible(vamos.Sidebar()),
			spec.Visible(vamos.RightRailChatTab()),
			spec.TextAbsent("Session history"),
			spec.ConsoleClean(),
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
		).
		When(vamos.FollowFirstSidebarDocumentLink()).
		Then(
			spec.URLContains("context=chat"),
			spec.URLContains("thread=th_1"),
			spec.Visible(vamos.CenterPane()),
		).
		Run(t)
}
