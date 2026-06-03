package vamos

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/playwright-community/playwright-go"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"
)

func OpenManagerWorkspaces() spec.Step {
	return customStep("open manager workspaces", func(t testing.TB, ctx *duiruntime.Context) {
		gotoURL(t, ctx, mustURL(t, ctx.Config.BaseURL, "/workspaces"))
		if err := ctx.Page.Locator("body").WaitFor(); err != nil {
			t.Fatal(err)
		}
	})
}

func SwitchToPublicWorkspaceFromEnv() spec.Step {
	return customStep("switch to public workspace", func(t testing.TB, ctx *duiruntime.Context) {
		slug := requiredEnv(t, "VAMOS_E2E_WORKSPACE_SLUG")
		switchURL := mustURL(t, ctx.Config.BaseURL, "/workspaces/switch/"+url.PathEscape(slug))
		q := switchURL.Query()
		q.Set("redirect", "/")
		switchURL.RawQuery = q.Encode()
		gotoURL(t, ctx, switchURL)
	})
}

func OpenPublicWorkspaceHostFromEnv() spec.Step {
	return customStep("open public workspace host", func(t testing.TB, ctx *duiruntime.Context) {
		gotoURL(t, ctx, publicWorkspaceURLFromEnv(t))
	})
}

func PublicWorkspaceAppReachableFromEnv() spec.Expectation {
	return expectation{customStep("public workspace app reachable", func(t testing.TB, ctx *duiruntime.Context) {
		expected := publicWorkspaceURLFromEnv(t).String()
		if !strings.HasPrefix(ctx.Page.URL(), expected) {
			t.Fatalf("expected child URL prefix %s, got %s", expected, ctx.Page.URL())
		}
		body := pageBodyText(t, ctx)
		if strings.Contains(strings.ToLower(body), "workspace unavailable") {
			t.Fatalf("child rendered unavailable page while expected running app")
		}
	})}
}

func PublicWorkspaceUnavailableFromEnv() spec.Expectation {
	return expectation{customStep("public workspace unavailable", func(t testing.TB, ctx *duiruntime.Context) {
		body := strings.ToLower(pageBodyText(t, ctx))
		if !strings.Contains(body, "workspace unavailable") {
			t.Fatalf("expected workspace unavailable page")
		}
	})}
}

func publicWorkspaceURLFromEnv(t testing.TB) *url.URL {
	t.Helper()
	slug := requiredEnv(t, "VAMOS_E2E_WORKSPACE_SLUG")
	domain := strings.Trim(requiredEnv(t, "VAMOS_E2E_WORKSPACE_DOMAIN"), ".")
	u, err := url.Parse("https://" + slug + "." + domain + "/")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func requiredEnv(t testing.TB, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}

func mustURL(t testing.TB, base, p string) *url.URL {
	t.Helper()
	u, err := url.Parse(strings.TrimRight(base, "/") + p)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func gotoURL(t testing.TB, ctx *duiruntime.Context, u *url.URL) {
	t.Helper()
	if _, err := ctx.Page.Goto(u.String(), playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded}); err != nil {
		t.Fatal(err)
	}
	if err := ctx.Page.Locator("body").WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateAttached}); err != nil {
		t.Fatal(err)
	}
}

func pageBodyText(t testing.TB, ctx *duiruntime.Context) string {
	t.Helper()
	text, err := ctx.Page.Locator("body").InnerText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Sprintf("empty body at %s", ctx.Page.URL())
	}
	return text
}
