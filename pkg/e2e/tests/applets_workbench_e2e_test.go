package tests

import (
	"fmt"
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
		Expect(vamos.Thoughts.SidebarVisible()).
		Expect(vamos.Thoughts.CenterPaneVisible()).
		Expect(vamos.Thoughts.RightRailVisible()).
		Expect(spec.ExpectStep(expectAppletSidebarTabs())).
		Expect(spec.ExpectStep(expectAppletRightRailTabs())).
		Expect(spec.ExpectStep(expectWordleIframeLoaded())).
		Expect(spec.ExpectStep(expectAppletIdentityEncoded("examples/wordle/AGENTS.md"))).
		Expect(vamos.Console.Clean()).
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
		Expect(vamos.Console.Clean()).
		Run()
}

func TestAppletsWorkbench_WordleAbsoluteRoutesForwardFromIframe(t *testing.T) {
	spec.Story(t, "applets workbench forwards wordle absolute datastar routes from iframe").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture("thoughts-workbench.basic")).
		Visit(vamos.Pages.Path("/examples/wordle?context=chat")).
		Expect(spec.ExpectStep(expectWordleIframeLoaded())).
		Do(loginToWordleApplet()).
		Expect(spec.ExpectStep(expectWordleAbsoluteRoutesReachApplet())).
		Expect(vamos.Console.Clean()).
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
		mapIndex := int(data["mapIndex"].(float64))
		if data["hasHead"] != true || mapIndex < 0 {
			t.Fatalf("Datastar import map missing: %#v", data)
		}
		if resizeIndex := int(data["resizeIndex"].(float64)); resizeIndex >= 0 && mapIndex > resizeIndex {
			t.Fatalf("Datastar import map after Workbench module: %#v", data)
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

func expectWordleAbsoluteRoutesReachApplet() spec.Step {
	return spec.Custom("wordle absolute routes reach applet backend", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		statusAny, err := ctx.Page.FrameLocator(wordleAppletFrameSelector).Locator("body").Evaluate(`async () => {
			const events = await fetch('/events', {headers: {Accept: 'text/event-stream'}}).then(r => r.status).catch(() => 0)
			const guesses = await fetch('/guesses', {
				method: 'POST',
				headers: {'Content-Type': 'application/x-www-form-urlencoded'},
				body: new URLSearchParams({guess: 'adieu'})
			}).then(r => r.status).catch(() => 0)
			return {events, guesses}
		}`, nil)
		if err != nil {
			t.Fatal(err)
		}
		statuses := statusAny.(map[string]any)
		for route, raw := range statuses {
			status := int(raw.(float64))
			if status == httpStatusNotFound || status == 0 || status >= 500 {
				t.Fatalf("%s status = %d, want applet-handled non-5xx/non-404", route, status)
			}
		}
	})
}

const httpStatusNotFound = 404

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
