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

func FollowFirstSidebarDocumentLink(t testing.TB, ctx *e2e.Context, _ string) {
	t.Helper()
	followFirstLink(t, ctx, "#thoughts-workbench-sidebar a[href*='/thoughts/'], [data-e2e='thoughts.workbench.sidebar'] a[href*='/thoughts/']")
}

func FollowFirstBreadcrumbLink(t testing.TB, ctx *e2e.Context, _ string) {
	t.Helper()
	followFirstLink(t, ctx, "nav[aria-label='Breadcrumb'] a[href], [data-slot='breadcrumb'] a[href], header a[href*='/thoughts/']")
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
