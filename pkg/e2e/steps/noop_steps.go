package steps

import (
	"strings"
	"testing"

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
