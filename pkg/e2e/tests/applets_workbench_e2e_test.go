package tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"
	"github.com/playwright-community/playwright-go"

	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

const wordleAppletFrameSelector = `iframe[src^='/examples/wordle/app/']`

func TestAppletsWorkbench_WordleRendersWorkbenchShell(t *testing.T) {
	spec.Story(t, "applets workbench renders wordle in document shell").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture("thoughts-workbench.basic")).
		Visit(vamos.Pages.Path("/examples/wordle?context=chat")).
		Expect(vamos.Thoughts.Ready()).
		Expect(spec.ExpectStep(expectWorkbenchDatastarImportMapPresent())).
		Do(openWorkbenchOverflow()).
		Expect(spec.ExpectStep(expectOpenAppletInNewTabLink())).
		Expect(spec.ExpectStep(expectNoWorkbenchOverflowAction("Comment"))).
		Expect(vamos.Thoughts.SidebarVisible()).
		Expect(vamos.Thoughts.CenterPaneVisible()).
		Expect(vamos.Thoughts.RightRailVisible()).
		Expect(spec.ExpectStep(expectAppletSidebarTabs())).
		Expect(spec.ExpectStep(expectAppletRightRailTabs())).
		Expect(spec.ExpectStep(expectWordleIframeLoaded())).
		Expect(spec.ExpectStep(expectAppletLocalChromeRemoved())).
		Expect(spec.ExpectStep(expectAppletIdentityEncoded("examples/wordle/AGENTS.md"))).
		Expect(spec.ExpectStep(expectAppletConsoleClean())).
		Run()
}

func TestAppletsWorkbench_DemandStartRefreshesToIframe(t *testing.T) {
	spec.Story(t, "applets workbench demand starts stopped wordle and refreshes iframe").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture("thoughts-workbench.basic")).
		Do(stopWordleAppletIfRunning()).
		Visit(vamos.Pages.Path("/examples/wordle?context=chat")).
		Expect(vamos.Thoughts.Ready()).
		Expect(spec.ExpectStep(expectWorkbenchDatastarImportMapPresent())).
		Expect(spec.ExpectStep(expectStartingPanelOrIframe())).
		Expect(spec.ExpectStep(expectWordleIframeLoaded())).
		Do(openWorkbenchOverflow()).
		Expect(spec.ExpectStep(expectOpenAppletInNewTabLink())).
		Expect(spec.ExpectStep(expectAppletConsoleClean())).
		Run()
}

func TestAppletsWorkbench_WordleAbsoluteRoutesForwardFromIframe(t *testing.T) {
	spec.Story(t, "applets workbench forwards wordle absolute datastar routes from iframe").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture("thoughts-workbench.basic")).
		Visit(vamos.Pages.Path("/examples/wordle?context=chat")).
		Expect(spec.ExpectStep(expectWorkbenchDatastarImportMapPresent())).
		Expect(spec.ExpectStep(expectWordleIframeLoaded())).
		Do(loginToWordleApplet()).
		Expect(spec.ExpectStep(expectWordleAliasCookiesWorkAfterLogin())).
		Expect(spec.ExpectStep(expectAppletConsoleClean())).
		Run()
}

func expectWorkbenchDatastarImportMapPresent() spec.Step {
	return spec.Custom("Workbench page has Datastar import map before modules", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		result, err := ctx.Page.Evaluate(`() => {
			const scripts = [...document.scripts]
			const maps = [...document.querySelectorAll('script[type="importmap"]')].map((el) => el.textContent || '')
			const resize = document.querySelector('script[src^="/js/workbench-resize.js"]')
			const resizeIndex = resize ? scripts.indexOf(resize) : -1
			const mapIndex = scripts.findIndex((el) => el.type === 'importmap' && (el.textContent || '').includes('@vamos/datastar'))
			return {hasHead: !!document.head, maps, mapIndex, resizeIndex}
		}`, nil)
		if err != nil {
			t.Fatal(err)
		}
		data := result.(map[string]any)
		mapIndex := browserNumberAsInt(data["mapIndex"])
		if data["hasHead"] != true || mapIndex < 0 {
			t.Fatalf("Datastar import map missing: %#v", data)
		}
		if resizeIndex := browserNumberAsInt(data["resizeIndex"]); resizeIndex >= 0 && mapIndex > resizeIndex {
			t.Fatalf("Datastar import map after Workbench module: %#v", data)
		}
	})
}

func openWorkbenchOverflow() spec.Step {
	return spec.Custom("open workbench overflow", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		trigger := ctx.Page.Locator("[data-testid='workbench-overflow-actions'] summary").First()
		if err := trigger.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(30_000)}); err != nil {
			t.Fatalf("workbench overflow trigger missing: %v", err)
		}
		open, err := trigger.Evaluate("el => el.parentElement.open", nil)
		if err != nil {
			t.Fatal(err)
		}
		if open != true {
			if err := trigger.Click(); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func expectOpenAppletInNewTabLink() spec.Step {
	return spec.Custom("Open in new tab uses proxied app route", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		link := ctx.Page.GetByRole(*playwright.AriaRoleLink, playwright.PageGetByRoleOptions{Name: "Open in new tab"}).First()
		if err := link.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(30_000)}); err != nil {
			t.Fatalf("open-new-tab link missing: %v", err)
		}
		href, err := link.GetAttribute("href")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(href, "/examples/wordle/app/") {
			t.Fatalf("open-new-tab href = %q, want proxied /examples/wordle/app/", href)
		}
		if strings.Contains(href, "localhost") || strings.Contains(href, "127.0.0.1") {
			t.Fatalf("open-new-tab href exposes raw backend: %q", href)
		}
	})
}

func expectAppletSidebarTabs() spec.Step {
	return spec.Custom("applet sidebar has Files and Workspaces tabs", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		sidebar := ctx.Page.Locator("#doc-workbench-sidebar-region, #thoughts-shared-sidebar, #thoughts-workbench-sidebar").First()
		if err := sidebar.WaitFor(); err != nil {
			t.Fatal(err)
		}
		for _, label := range []string{"Files", "Workspaces"} {
			if err := sidebar.GetByText(label, playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).First().WaitFor(); err != nil {
				t.Fatalf("sidebar missing %s tab: %v", label, err)
			}
		}
	})
}

func expectAppletRightRailTabs() spec.Step {
	return spec.Custom("applet right rail has Chat and Comments tabs", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		rightRail := ctx.Page.Locator("#doc-workbench-right-region, #doc-right-rail, #doc-workbench-right-rail").First()
		if err := rightRail.WaitFor(); err != nil {
			t.Fatal(err)
		}
		for _, label := range []string{"Chat", "Comments"} {
			if err := rightRail.GetByText(label, playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).First().WaitFor(); err != nil {
				t.Fatalf("right rail missing %s tab: %v", label, err)
			}
		}
	})
}

func expectWordleIframeLoaded() spec.Step {
	return spec.Custom("wordle applet iframe loads", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		frame := ctx.Page.Locator(wordleAppletFrameSelector).First()
		if err := frame.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(60_000)}); err != nil {
			dumpBody(t, ctx, "wordle iframe missing")
		}
		title, err := frame.GetAttribute("title")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(title, "Daily Wordle") {
			t.Fatalf("iframe title = %q, want Daily Wordle", title)
		}
		if err := ctx.Page.FrameLocator(wordleAppletFrameSelector).GetByText("Daily Wordle").First().WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(60_000)}); err != nil {
			t.Fatalf("wordle iframe content did not load: %v", err)
		}
	})
}

func expectAppletLocalChromeRemoved() spec.Step {
	return spec.Custom("healthy applet body omits local chrome", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		html, err := ctx.Page.Locator("#applet-frame-wordle").Evaluate("el => el.innerHTML", nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"datastar applet", "Open in new tab", "/forms/applets/wordle/restart", "/forms/applets/wordle/stop"} {
			if strings.Contains(fmt.Sprint(html), forbidden) {
				t.Fatalf("local applet chrome/control still rendered %q", forbidden)
			}
		}
	})
}

func expectAppletIdentityEncoded(identity string) spec.Step {
	return spec.Custom("applet identity is present in page", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		html, err := ctx.Page.Locator("body").Evaluate("el => el.innerHTML", nil)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(fmt.Sprint(html), identity) {
			t.Fatalf("page does not encode applet identity %q", identity)
		}
	})
}

func stopWordleAppletIfRunning() spec.Step {
	return spec.Custom("stop wordle applet if running", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		visit(t, ctx, "/examples/wordle")
		_, _ = ctx.Page.Evaluate(`async () => {
			await fetch('/forms/applets/wordle/stop', {
				method: 'POST',
				headers: {'Content-Type': 'application/x-www-form-urlencoded'},
				body: new URLSearchParams({identity_path: 'examples/wordle/AGENTS.md'}),
				redirect: 'manual'
			}).catch(() => null)
		}`, nil)
	})
}

func expectStartingPanelOrIframe() spec.Step {
	return spec.Custom("applet shows starting panel or iframe", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		starting := ctx.Page.Locator("[id^='applet-status-wordle']").First()
		iframe := ctx.Page.Locator(wordleAppletFrameSelector).First()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if count, _ := iframe.Count(); count > 0 {
				return
			}
			if count, _ := starting.Count(); count > 0 {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		dumpBody(t, ctx, "starting panel or iframe missing")
	})
}

func loginToWordleApplet() spec.Step {
	return spec.Custom("login to wordle iframe", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		frame := ctx.Page.FrameLocator(wordleAppletFrameSelector)
		input := frame.Locator("input[name='username']").First()
		if err := input.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(60_000)}); err != nil {
			t.Fatalf("wordle login input missing: %v", err)
		}
		if err := input.Fill("e2e"); err != nil {
			t.Fatal(err)
		}
		if err := frame.GetByRole("button", playwright.FrameLocatorGetByRoleOptions{Name: "Play"}).Click(); err != nil {
			t.Fatal(err)
		}
		if err := frame.Locator("[aria-label='Wordle board']").First().WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(30_000)}); err != nil {
			t.Fatalf("wordle board did not render after login: %v", err)
		}
	})
}

func expectWordleAliasCookiesWorkAfterLogin() spec.Step {
	return spec.Custom("wordle alias routes receive login cookie", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		result, err := ctx.Page.FrameLocator(wordleAppletFrameSelector).Locator("body").Evaluate(`async () => {
			const events = await fetch('/events', {headers: {Accept: 'text/event-stream'}})
				.then((r) => ({status: r.status}))
				.catch((e) => ({status: 0, text: String(e)}))
			const guesses = await fetch('/guesses', {
				method: 'POST',
				headers: {'Content-Type': 'application/x-www-form-urlencoded'},
				body: new URLSearchParams({guess: 'adieu'}),
			})
				.then(async (r) => ({status: r.status, text: await r.text().catch(() => '')}))
				.catch((e) => ({status: 0, text: String(e)}))
			return {events, guesses, cookie: document.cookie}
		}`, nil)
		if err != nil {
			t.Fatal(err)
		}
		data := result.(map[string]any)
		for route, raw := range map[string]any{"events": data["events"], "guesses": data["guesses"]} {
			entry := raw.(map[string]any)
			status := browserNumberAsInt(entry["status"])
			if status == http.StatusNotFound || status == 0 || status >= 500 || status == http.StatusUnauthorized || status == http.StatusForbidden {
				t.Fatalf("%s status = %d after login; result=%#v", route, status, data)
			}
		}
	})
}

func expectAppletConsoleClean() spec.Step {
	return spec.Custom("applet console clean", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		time.Sleep(250 * time.Millisecond)
		problems := ctx.Console.Problems()
		filtered := problems[:0]
		for _, problem := range problems {
			if problem.Type == "warning" && strings.Contains(problem.Text, "allow-scripts and allow-same-origin") {
				continue
			}
			filtered = append(filtered, problem)
		}
		if len(filtered) > 0 {
			t.Fatalf("console problems:\n%s", duiruntime.FormatConsoleProblems(filtered))
		}
	})
}

func browserNumberAsInt(value any) int {
	switch n := value.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return -1
	}
}

func visit(t testing.TB, ctx *duiruntime.Context, p string) {
	t.Helper()
	if _, err := ctx.Page.Goto(strings.TrimRight(ctx.Config.BaseURL, "/")+p, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded}); err != nil {
		t.Fatal(err)
	}
}

func dumpBody(t testing.TB, ctx *duiruntime.Context, message string) {
	t.Helper()
	body, _ := ctx.Page.Locator("body").InnerText()
	if len(body) > 2000 {
		body = body[:2000]
	}
	t.Fatalf("%s\nbody:\n%s", message, body)
}
