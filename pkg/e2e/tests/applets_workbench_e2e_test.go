package tests

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"
	"github.com/playwright-community/playwright-go"

	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

const (
	wordleAppletFrameSelector    = `iframe[src^='/examples/wordle/app/']`
	streamlitAppletFrameSelector = `iframe[src^='/examples/streamlit/app/']`
)

type streamlitBrowserProbe struct {
	mu        sync.Mutex
	urls      []string
	errors    []string
	responses []string
}

func (p *streamlitBrowserProbe) observeWebSocket(ws playwright.WebSocket) {
	if !strings.Contains(ws.URL(), "_stcore/stream") {
		return
	}
	p.mu.Lock()
	p.urls = append(p.urls, ws.URL())
	p.mu.Unlock()
	ws.OnSocketError(func(message string) {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.errors = append(p.errors, ws.URL()+": "+message)
	})
}

func (p *streamlitBrowserProbe) observeResponse(response playwright.Response) {
	if !strings.Contains(response.URL(), "_stcore/") && !strings.Contains(response.URL(), "/examples/streamlit/app/") {
		return
	}
	if status := response.Status(); status >= http.StatusBadRequest {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.responses = append(p.responses, fmt.Sprintf("%s -> %d", response.URL(), status))
	}
}

func (p *streamlitBrowserProbe) observeRequestFailure(request playwright.Request) {
	if !strings.Contains(request.URL(), "_stcore/") && request.ResourceType() != "websocket" {
		return
	}
	failure := "request failed"
	if err := request.Failure(); err != nil {
		failure = err.Error()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errors = append(p.errors, request.URL()+": "+failure)
}

func observeStreamlitBrowserTraffic(probe *streamlitBrowserProbe) spec.Step {
	return spec.Custom("observe Streamlit browser traffic", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		ctx.Page.OnWebSocket(probe.observeWebSocket)
		ctx.Page.OnResponse(probe.observeResponse)
		ctx.Page.OnRequestFailed(probe.observeRequestFailure)
	})
}

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

func TestAppletsWorkbench_StreamlitRendersEmbeddedAndNewTab(t *testing.T) {
	probe := &streamlitBrowserProbe{}
	spec.Story(t, "applets workbench renders streamlit embedded and in scoped new tab").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture("thoughts-workbench.basic")).
		Do(observeStreamlitBrowserTraffic(probe)).
		Visit(vamos.Pages.Path("/examples/streamlit?context=chat")).
		Expect(vamos.Thoughts.Ready()).
		Expect(spec.ExpectStep(expectWorkbenchDatastarImportMapPresent())).
		Expect(spec.ExpectStep(expectOpenStreamlitInNewTabLink())).
		Expect(spec.ExpectStep(expectStreamlitIframeLoaded())).
		Expect(spec.ExpectStep(expectStreamlitSessionCounterWorks())).
		Expect(spec.ExpectStep(expectStreamlitOpenNewTabRenders())).
		Expect(spec.ExpectStep(expectStreamlitNoWebSocketFailures(probe))).
		Run()
}

func TestAppletsWorkbench_StreamlitRestartChangesProcessIdentity(t *testing.T) {
	probe := &streamlitBrowserProbe{}
	spec.Story(t, "applets workbench restarts streamlit and changes process identity").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture("thoughts-workbench.basic")).
		Do(observeStreamlitBrowserTraffic(probe)).
		Do(stopStreamlitAppletIfRunning()).
		Visit(vamos.Pages.Path("/examples/streamlit?context=chat")).
		Expect(vamos.Thoughts.Ready()).
		Expect(spec.ExpectStep(expectStreamlitStartingPanelOrIframe())).
		Expect(spec.ExpectStep(expectStreamlitIframeLoaded())).
		Expect(spec.ExpectStep(expectStreamlitRestartChangesIdentity())).
		Expect(spec.ExpectStep(expectStreamlitNoWebSocketFailures(probe))).
		Run()
}

func TestWordleAppletSmoke(t *testing.T) {
	spec.Story(t, "wordle applet smoke").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture("thoughts-workbench.basic")).
		Visit(vamos.Pages.Path("/examples/wordle?context=chat")).
		Expect(vamos.Thoughts.Ready()).
		Expect(spec.ExpectStep(expectWorkbenchDatastarImportMapPresent())).
		Expect(spec.ExpectStep(expectWordleIframeLoaded())).
		Do(loginToWordleApplet()).
		Do(submitWordleGuess("aback")).
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

func expectStreamlitIframeLoaded() spec.Step {
	return spec.Custom("streamlit applet iframe loads", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		frame := ctx.Page.Locator(streamlitAppletFrameSelector).First()
		if err := frame.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(90_000)}); err != nil {
			dumpBody(t, ctx, "streamlit iframe missing")
		}
		title, err := frame.GetAttribute("title")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(title, "Streamlit") {
			t.Fatalf("iframe title = %q, want Streamlit", title)
		}
		if err := ctx.Page.FrameLocator(streamlitAppletFrameSelector).GetByText("Streamlit applet smoke test").First().WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(90_000)}); err != nil {
			t.Fatalf("streamlit iframe content did not load: %v", err)
		}
	})
}

func expectStreamlitSessionCounterWorks() spec.Step {
	return spec.Custom("streamlit session counter works", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		app := ctx.Page.FrameLocator(streamlitAppletFrameSelector)
		if err := app.GetByRole("button", playwright.FrameLocatorGetByRoleOptions{Name: "Increment Streamlit session counter"}).Click(); err != nil {
			t.Fatal(err)
		}
		if err := app.GetByText("Session counter: 1").First().WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(30_000)}); err != nil {
			t.Fatalf("streamlit counter did not increment over websocket session: %v", err)
		}
	})
}

func expectOpenStreamlitInNewTabLink() spec.Step {
	return spec.Custom("Open in new tab uses Streamlit scoped app route", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		link := ctx.Page.GetByRole(*playwright.AriaRoleLink, playwright.PageGetByRoleOptions{Name: "Open in new tab"}).First()
		if err := link.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(30_000)}); err != nil {
			t.Fatalf("open-new-tab link missing: %v", err)
		}
		href, err := link.GetAttribute("href")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(href, "/examples/streamlit/app/") {
			t.Fatalf("open-new-tab href = %q, want proxied /examples/streamlit/app/", href)
		}
		if strings.Contains(href, "localhost") || strings.Contains(href, "127.0.0.1") {
			t.Fatalf("open-new-tab href exposes raw backend: %q", href)
		}
	})
}

func expectStreamlitOpenNewTabRenders() spec.Step {
	return spec.Custom("Streamlit scoped new tab renders", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		link := ctx.Page.GetByRole(*playwright.AriaRoleLink, playwright.PageGetByRoleOptions{Name: "Open in new tab"}).First()
		popup, err := ctx.Page.ExpectPopup(func() error { return link.Click() })
		if err != nil {
			t.Fatalf("open Streamlit popup: %v", err)
		}
		defer popup.Close()
		if err := popup.WaitForLoadState(playwright.PageWaitForLoadStateOptions{State: playwright.LoadStateDomcontentloaded, Timeout: playwright.Float(60_000)}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(popup.URL(), "/examples/streamlit/app/") {
			t.Fatalf("popup URL = %q", popup.URL())
		}
		if strings.Contains(popup.URL(), "localhost") || strings.Contains(popup.URL(), "127.0.0.1") {
			t.Fatalf("popup exposes raw backend URL: %q", popup.URL())
		}
		if err := popup.GetByText("Streamlit applet smoke test").First().WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(90_000)}); err != nil {
			t.Fatalf("streamlit popup content did not load: %v", err)
		}
	})
}

func expectStreamlitNoWebSocketFailures(probe *streamlitBrowserProbe) spec.Step {
	return spec.Custom("Streamlit websocket has no browser failures", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		time.Sleep(500 * time.Millisecond)
		probe.mu.Lock()
		urls := append([]string(nil), probe.urls...)
		errors := append([]string(nil), probe.errors...)
		responses := append([]string(nil), probe.responses...)
		probe.mu.Unlock()
		if len(urls) == 0 {
			t.Fatalf("no _stcore/stream websocket observed")
		}
		if len(errors) > 0 || len(responses) > 0 {
			t.Fatalf("streamlit websocket/network errors: websocket=%v responses=%v", errors, responses)
		}
		problems := ctx.Console.Problems()
		filtered := problems[:0]
		for _, problem := range problems {
			if problem.Type == "warning" && strings.Contains(problem.Text, "allow-scripts and allow-same-origin") {
				continue
			}
			text := strings.ToLower(problem.Text)
			if strings.Contains(text, "_stcore/stream") || strings.Contains(text, "websocket") || strings.Contains(text, "502") {
				filtered = append(filtered, problem)
			}
		}
		if len(filtered) > 0 {
			t.Fatalf("streamlit websocket console problems:\n%s", duiruntime.FormatConsoleProblems(filtered))
		}
	})
}

type streamlitProcessIdentity struct {
	PID       string
	RunID     string
	StartedAt string
	Text      string
}

func stopStreamlitAppletIfRunning() spec.Step {
	return spec.Custom("stop streamlit applet if running", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		visit(t, ctx, "/examples/streamlit")
		_, _ = ctx.Page.Evaluate(`async () => {
			await fetch('/forms/applets/streamlit/stop', {
				method: 'POST',
				headers: {'Content-Type': 'application/x-www-form-urlencoded'},
				body: new URLSearchParams({identity_path: 'examples/streamlit/AGENTS.md'}),
				redirect: 'manual'
			}).catch(() => null)
		}`, nil)
	})
}

func expectStreamlitStartingPanelOrIframe() spec.Step {
	return spec.Custom("streamlit shows starting panel or iframe", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		starting := ctx.Page.Locator("[id^='applet-status-streamlit']").First()
		iframe := ctx.Page.Locator(streamlitAppletFrameSelector).First()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if count, _ := iframe.Count(); count > 0 {
				return
			}
			if count, _ := starting.Count(); count > 0 {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		dumpBody(t, ctx, "streamlit starting panel or iframe missing")
	})
}

func expectStreamlitRestartChangesIdentity() spec.Step {
	return spec.Custom("streamlit restart changes process identity", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		before, err := readStreamlitProcessIdentity(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if before.PID == "" && before.RunID == "" && before.StartedAt == "" {
			t.Fatalf("empty streamlit identity before restart; body:\n%s", before.Text)
		}
		_, err = ctx.Page.Evaluate(`async () => {
			await fetch('/forms/applets/streamlit/restart', {
				method: 'POST',
				headers: {'Content-Type': 'application/x-www-form-urlencoded'},
				body: new URLSearchParams({identity_path: 'examples/streamlit/AGENTS.md'}),
				redirect: 'manual'
			})
		}`, nil)
		if err != nil {
			t.Fatal(err)
		}
		visit(t, ctx, "/examples/streamlit?context=chat")
		if err := ctx.Page.FrameLocator(streamlitAppletFrameSelector).GetByText("Streamlit applet smoke test").First().WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(90_000)}); err != nil {
			t.Fatalf("streamlit did not return after restart: %v", err)
		}

		var after streamlitProcessIdentity
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			after, err = readStreamlitProcessIdentity(ctx)
			if err == nil && identityChanged(before, after) {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
		t.Fatalf("streamlit identity did not change after restart\nbefore=%+v\nafter=%+v", before, after)
	})
}

func readStreamlitProcessIdentity(ctx *duiruntime.Context) (streamlitProcessIdentity, error) {
	raw, err := ctx.Page.FrameLocator(streamlitAppletFrameSelector).Locator("body").Evaluate(`(body) => {
		const text = body.innerText || ''
		const pick = (re) => {
			const match = text.match(re)
			return match ? match[1] : ''
		}
		return {
			pid: pick(/Process PID\s+(\d+)/),
			runID: pick(/"run_id"\s*:\s*"([^"]+)"/),
			startedAt: pick(/"started_at"\s*:\s*"([^"]+)"/),
			text,
		}
	}`, nil)
	if err != nil {
		return streamlitProcessIdentity{}, err
	}
	data, ok := raw.(map[string]any)
	if !ok {
		return streamlitProcessIdentity{Text: fmt.Sprint(raw)}, nil
	}
	return streamlitProcessIdentity{
		PID:       fmt.Sprint(data["pid"]),
		RunID:     fmt.Sprint(data["runID"]),
		StartedAt: fmt.Sprint(data["startedAt"]),
		Text:      fmt.Sprint(data["text"]),
	}, nil
}

func identityChanged(before, after streamlitProcessIdentity) bool {
	return (before.PID != "" && after.PID != "" && before.PID != after.PID) ||
		(before.RunID != "" && after.RunID != "" && before.RunID != after.RunID) ||
		(before.StartedAt != "" && after.StartedAt != "" && before.StartedAt != after.StartedAt)
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
		if err := input.Fill(fmt.Sprintf("e2e-%d", time.Now().UnixNano())); err != nil {
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

func submitWordleGuess(guess string) spec.Step {
	return spec.Custom("submit wordle guess through applet UI", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		frame := ctx.Page.FrameLocator(wordleAppletFrameSelector)
		for _, letter := range strings.ToUpper(guess) {
			key := frame.GetByRole("button", playwright.FrameLocatorGetByRoleOptions{
				Name:  "Letter " + string(letter),
				Exact: playwright.Bool(true),
			})
			if err := key.Click(); err != nil {
				t.Fatalf("click wordle letter %q: %v", letter, err)
			}
		}
		if err := frame.GetByRole("button", playwright.FrameLocatorGetByRoleOptions{
			Name:  "Submit guess",
			Exact: playwright.Bool(true),
		}).Click(); err != nil {
			t.Fatalf("submit wordle guess: %v", err)
		}
		if err := frame.GetByText("1/6", playwright.FrameLocatorGetByTextOptions{Exact: playwright.Bool(true)}).WaitFor(
			playwright.LocatorWaitForOptions{Timeout: playwright.Float(30_000)},
		); err != nil {
			t.Fatalf("wordle guess did not reach durable board state: %v", err)
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
