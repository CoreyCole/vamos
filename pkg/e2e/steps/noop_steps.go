package steps

import (
	"strings"
	"testing"
	"time"

	"github.com/playwright-community/playwright-go"

	"github.com/CoreyCole/vamos/pkg/e2e/fixtures"
	e2e "github.com/CoreyCole/vamos/pkg/e2e/runtime"
)

func AuthenticatedAs(t testing.TB, ctx *e2e.Context, email string) {
	t.Helper()
	c, cancel := e2e.ContextWithTimeout()
	defer cancel()
	if err := e2e.Authenticate(c, ctx.Page, ctx.Config, email); err != nil {
		t.Fatal(err)
	}
}

func LoadFixture(t testing.TB, ctx *e2e.Context, name string) fixtures.State {
	t.Helper()
	c, cancel := e2e.ContextWithTimeout()
	defer cancel()
	db, err := e2e.OpenWorkspaceDB(c, ctx.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace := fixtures.WorkspaceIdentity{
		Slug:         ctx.Config.Workspace.Slug,
		CheckoutPath: ctx.Config.Workspace.CheckoutPath,
		DBPath:       ctx.Config.Workspace.DBPath,
		ManagerURL:   ctx.Config.Workspace.ManagerURL,
	}
	state, err := fixtures.Load(c, db, workspace, name)
	if err != nil {
		t.Fatal(err)
	}
	ctx.Fixture = state
	return state
}

func Visit(t testing.TB, ctx *e2e.Context, path string) {
	t.Helper()
	_, err := ctx.Page.Goto(
		ctx.Config.BaseURL+path,
		playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func WaitForFeatureReady(t testing.TB, ctx *e2e.Context, feature string) {
	t.Helper()
	entry, err := ctx.Selectors.Resolve("feature." + feature)
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.Page.Locator(entry.CSS).WaitFor(); err != nil {
		t.Fatal(err)
	}
}

func ExpectRegionVisible(t testing.TB, ctx *e2e.Context, key string) {
	t.Helper()
	expectVisible(t, ctx, key)
}

func ExpectRegionReachable(t testing.TB, ctx *e2e.Context, key string) {
	t.Helper()
	expectVisible(t, ctx, key)
}

func ExpectTabSelected(t testing.TB, ctx *e2e.Context, key string) {
	t.Helper()
	expectVisible(t, ctx, key)
}

func ExpectTextAbsent(t testing.TB, ctx *e2e.Context, text string) {
	t.Helper()
	body, err := ctx.Page.Locator("body").InnerText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, text) {
		t.Fatalf("text %q present", text)
	}
}

func ExpectBrowserURLContains(t testing.TB, ctx *e2e.Context, text string) {
	t.Helper()
	if !strings.Contains(ctx.Page.URL(), text) {
		t.Fatalf("browser URL %q does not contain %q", ctx.Page.URL(), text)
	}
}

func ExpectConsoleClean(t testing.TB, ctx *e2e.Context, _ string) {
	t.Helper()
	time.Sleep(250 * time.Millisecond)
	if problems := ctx.Console.Problems(); len(problems) > 0 {
		t.Fatalf("browser console errors/warnings:\n%s", e2e.FormatConsoleProblems(problems))
	}
}

func ExpectInactiveTabPanelsHidden(t testing.TB, ctx *e2e.Context, _ string) {
	t.Helper()
	visible, err := ctx.Page.Locator("[data-show][class*='hidden']").First().IsVisible()
	if err != nil {
		return
	}
	if visible {
		t.Fatal("inactive tab panel with hidden class is visible")
	}
}

func ExpectWorkspaceVisible(t testing.TB, ctx *e2e.Context, name string) {
	t.Helper()
	if err := ctx.Page.GetByText(name).First().WaitFor(); err != nil {
		t.Fatal(err)
	}
}

func ExpectWorkspaceAbsent(t testing.TB, ctx *e2e.Context, name string) {
	t.Helper()
	body, err := ctx.Page.Locator("body").InnerText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, name) {
		t.Fatalf("workspace %q present", name)
	}
}

func ExpectWorkspaceBefore(t testing.TB, ctx *e2e.Context, first, second string) {
	t.Helper()
	body, err := ctx.Page.Locator("body").InnerText()
	if err != nil {
		t.Fatal(err)
	}
	firstIdx := strings.Index(body, first)
	secondIdx := strings.Index(body, second)
	if firstIdx < 0 || secondIdx < 0 || firstIdx > secondIdx {
		t.Fatalf("workspace order mismatch: %q index %d, %q index %d", first, firstIdx, second, secondIdx)
	}
}

func ExpectWorkspaceCleanupSucceeds(t testing.TB, ctx *e2e.Context, _ string) {
	t.Helper()
	ExpectConsoleClean(t, ctx, "cleanup")
}

func FollowFirstSidebarDocumentLink(t testing.TB, ctx *e2e.Context, _ string) {
	t.Helper()
	followFirstLink(t, ctx, "#thoughts-workbench-sidebar a[href*='/thoughts/'], [data-e2e='thoughts.workbench.sidebar'] a[href*='/thoughts/']")
}

func FollowFirstBreadcrumbLink(t testing.TB, ctx *e2e.Context, _ string) {
	t.Helper()
	followFirstLink(t, ctx, "nav[aria-label='Breadcrumb'] a[href], [data-slot='breadcrumb'] a[href], header a[href*='/thoughts/']")
}

func SwitchTab(t testing.TB, ctx *e2e.Context, key string) {
	t.Helper()
	entry, err := ctx.Selectors.Resolve(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.Page.Locator(entry.CSS).First().Click(); err != nil {
		t.Fatal(err)
	}
}

func ToggleRegion(t testing.TB, ctx *e2e.Context, key string) {
	t.Helper()
	entry, err := ctx.Selectors.Resolve(key)
	if err != nil {
		t.Fatal(err)
	}
	region := ctx.Page.Locator(entry.CSS).First()
	button := region.Locator("button[aria-expanded], button[data-workbench-save-on-click], button").First()
	if err := button.Click(); err != nil {
		t.Fatal(err)
	}
}

func EnableShowHistoricalWorkspaces(t testing.TB, ctx *e2e.Context, _ string) {
	t.Helper()
	locator := ctx.Page.Locator("label:has-text('Show historical'), button:has-text('Show historical'), input[name='showHistorical'], input[name='show_historical']").First()
	if err := locator.Click(); err != nil {
		t.Fatal(err)
	}
}

func CleanupWorkspace(t testing.TB, ctx *e2e.Context, name string) {
	t.Helper()
	if err := ctx.Page.GetByText(name).First().WaitFor(); err != nil {
		t.Fatal(err)
	}
	button := ctx.Page.Locator("button:has-text('Clean up'), button:has-text('Close')").First()
	if err := button.Click(); err != nil {
		t.Fatal(err)
	}
}

func followFirstLink(t testing.TB, ctx *e2e.Context, selector string) {
	t.Helper()
	link := ctx.Page.Locator(selector).First()
	if err := link.WaitFor(); err != nil {
		t.Fatal(err)
	}
	if err := link.Click(); err != nil {
		t.Fatal(err)
	}
	_ = ctx.Page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{State: playwright.LoadStateDomcontentloaded})
}

func expectVisible(t testing.TB, ctx *e2e.Context, key string) {
	t.Helper()
	entry, err := ctx.Selectors.Resolve(key)
	if err != nil {
		t.Fatal(err)
	}
	visible, err := ctx.Page.Locator(entry.CSS).IsVisible()
	if err != nil {
		t.Fatal(err)
	}
	if !visible {
		t.Fatalf("selector %s for key %s not visible", entry.CSS, key)
	}
}
