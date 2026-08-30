package tests

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"
	"github.com/playwright-community/playwright-go"

	"github.com/CoreyCole/vamos/pkg/e2e/fixtures"
	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

func TestWorkbenchV2_ThoughtsDatastarAppletReadinessMorphsOnlyFrame(t *testing.T) {
	identity := "thoughts/v2-wordle/AGENTS.md"
	token := base64.RawURLEncoding.EncodeToString([]byte(identity))
	frameID := "applet-frame-e2e-v2-wordle"
	frame := "#" + frameID + " iframe"

	spec.Story(t, "workbench v2 thoughts datastar applet readiness morphs only frame").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Do(stopV2Applet(token, identity)).
		Visit(vamos.Pages.Path("/thoughts/_render/app/" + token)).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(requireV2AppletStartingStatus("e2e-v2-wordle")).
		Do(rememberAbsentAppletFrame(frameID)).
		Do(rememberWorkbenchRoot()).
		Do(rememberTopDocument()).
		Do(expectV2AppletStartingOrFrame(frameID)).
		Do(expectV2AppletFrame(frame, "Daily Wordle")).
		Do(expectV2AppletSandbox(frame)).
		Do(expectV2AppletDoesNotLeakBackend(frame)).
		Do(expectTopAndRootIdentityUnchanged()).
		Do(loginToV2Wordle(frame)).
		Do(submitV2WordleGuess(frame, "aback")).
		Do(expectOnlyManagedAppletSandboxWarnings()).
		Run()
}

func TestWorkbenchV2_ThoughtsStreamlitReconnectsToManagedProcess(t *testing.T) {
	identity := "thoughts/v2-streamlit/AGENTS.md"
	token := base64.RawURLEncoding.EncodeToString([]byte(identity))
	frame := "#applet-frame-e2e-v2-streamlit iframe"
	probe := &streamlitBrowserProbe{}

	spec.Story(t, "workbench v2 thoughts streamlit reconnects to managed process").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Do(observeStreamlitBrowserTraffic(probe)).
		Visit(vamos.Pages.Path("/thoughts/_render/app/" + token)).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(expectV2AppletFrame(frame, "Streamlit applet smoke test")).
		Do(expectV2AppletSandbox(frame)).
		Do(expectV2AppletDoesNotLeakBackend(frame)).
		Do(rememberV2StreamlitIdentity(frame)).
		Do(rememberStreamlitConnectionCount(probe)).
		Do(incrementV2StreamlitCounter(frame)).
		Visit(vamos.Pages.Thought("workbench-v2/root.md")).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(browserBack()).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(expectV2AppletFrame(frame, "Streamlit applet smoke test")).
		Do(expectNewStreamlitConnection(probe)).
		Do(expectV2StreamlitIdentityReused(frame)).
		Expect(spec.ExpectStep(expectStreamlitNoWebSocketFailures(probe))).
		Do(expectOnlyManagedAppletSandboxWarnings()).
		Run()
}

func rememberTopDocument() spec.Step {
	return spec.Custom(
		"remember top document",
		func(t testing.TB, ctx *duiruntime.Context) {
			value, err := ctx.Page.Evaluate(
				`() => { document.documentElement.dataset.e2eTopIdentity = crypto.randomUUID(); return document.documentElement.dataset.e2eTopIdentity }`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			ctx.Memory["workbench-v2.top-document"] = fmt.Sprint(value)
		},
	)
}

func stopV2Applet(runtimeToken, identity string) spec.Step {
	return spec.Custom(
		"stop fixture-owned applet",
		func(t testing.TB, ctx *duiruntime.Context) {
			visit(t, ctx, "/threads")
			status, err := ctx.Page.Evaluate(
				`async ({id, identity}) => { const r = await fetch('/forms/applets/' + id + '/stop', {method: 'POST', headers: {'Content-Type': 'application/x-www-form-urlencoded'}, body: new URLSearchParams({identity_path: identity}), redirect: 'manual'}); return {status: r.status, type: r.type} }`,
				map[string]any{"id": runtimeToken, "identity": identity},
			)
			if err != nil {
				t.Fatal(err)
			}
			result := status.(map[string]any)
			if result["type"] != "opaqueredirect" &&
				fmt.Sprint(result["status"]) != "200" &&
				fmt.Sprint(result["status"]) != "204" {
				t.Fatalf("applet stop response = %#v", result)
			}
		},
	)
}

func requireV2AppletStartingStatus(appletID string) spec.Step {
	return spec.Custom(
		"fixture applet shows starting status",
		func(t testing.TB, ctx *duiruntime.Context) {
			status := ctx.Page.Locator("#applet-status-" + appletID).First()
			if err := status.WaitFor(
				playwright.LocatorWaitForOptions{Timeout: playwright.Float(30_000)},
			); err != nil {
				t.Fatalf("starting status missing: %v", err)
			}
		},
	)
}

func expectTopAndRootIdentityUnchanged() spec.Step {
	return spec.Custom(
		"applet readiness keeps top document and root",
		func(t testing.TB, ctx *duiruntime.Context) {
			top, err := ctx.Page.Locator("html").GetAttribute("data-e2e-top-identity")
			if err != nil || top != ctx.Memory["workbench-v2.top-document"] {
				t.Fatalf("applet readiness reloaded top document: %q %v", top, err)
			}
			root, err := ctx.Page.Locator("#workbench-root").
				GetAttribute("data-e2e-root-identity")
			if err != nil || root != ctx.Memory["workbench-v2.root"] {
				t.Fatalf("applet readiness replaced workbench root: %q %v", root, err)
			}
		},
	)
}

func rememberAbsentAppletFrame(frameID string) spec.Step {
	return spec.Custom(
		"remember applet frame is absent while starting",
		func(t testing.TB, ctx *duiruntime.Context) {
			count, err := ctx.Page.Locator("#" + frameID + " iframe").Count()
			if err != nil || count != 0 {
				t.Fatalf(
					"healthy applet frame existed before readiness: %d %v",
					count,
					err,
				)
			}
			ctx.Memory["workbench-v2.applet-frame-absent"] = frameID
		},
	)
}

func expectV2AppletStartingOrFrame(frameID string) spec.Step {
	return spec.Custom(
		"v2 applet has starting state or frame",
		func(t testing.TB, ctx *duiruntime.Context) {
			for _, selector := range []string{"#" + frameID, "[id^='applet-status-e2e-v2-wordle']"} {
				count, err := ctx.Page.Locator(selector).Count()
				if err == nil && count > 0 {
					return
				}
			}
			t.Fatal("v2 applet has neither status nor frame")
		},
	)
}

func expectV2AppletFrame(selector, text string) spec.Step {
	return spec.Custom(
		"v2 applet frame is healthy",
		func(t testing.TB, ctx *duiruntime.Context) {
			frame := ctx.Page.Locator(selector).First()
			if err := frame.WaitFor(
				playwright.LocatorWaitForOptions{Timeout: playwright.Float(90_000)},
			); err != nil {
				t.Fatal(err)
			}
			if err := ctx.Page.FrameLocator(selector).
				GetByText(text).
				First().
				WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(90_000)}); err != nil {
				t.Fatal(err)
			}
		},
	)
}

func expectV2AppletSandbox(selector string) spec.Step {
	return spec.Custom(
		"v2 applet keeps managed sandbox",
		func(t testing.TB, ctx *duiruntime.Context) {
			sandbox, err := ctx.Page.Locator(selector).First().GetAttribute("sandbox")
			if err != nil ||
				sandbox != "allow-same-origin allow-forms allow-downloads allow-scripts" {
				t.Fatalf("managed applet sandbox = %q %v", sandbox, err)
			}
		},
	)
}

func expectOnlyManagedAppletSandboxWarnings() spec.Step {
	return spec.Custom(
		"managed applet sandbox warning is the only console problem",
		func(t testing.TB, ctx *duiruntime.Context) {
			for _, problem := range ctx.Console.Problems() {
				if problem.Type == "warning" && strings.Contains(
					problem.Text,
					"both allow-scripts and allow-same-origin",
				) {
					continue
				}
				t.Fatalf(
					"unexpected console problem:\n%s",
					duiruntime.FormatConsoleProblems([]duiruntime.ConsoleEntry{problem}),
				)
			}
		},
	)
}

func expectV2AppletDoesNotLeakBackend(selector string) spec.Step {
	return spec.Custom(
		"v2 applet uses scoped proxy",
		func(t testing.TB, ctx *duiruntime.Context) {
			src, err := ctx.Page.Locator(selector).First().GetAttribute("src")
			if err != nil || !strings.Contains(src, "/thoughts/_render/app/") ||
				strings.Contains(src, "localhost") ||
				strings.Contains(src, "127.0.0.1") {
				t.Fatalf("applet frame exposes raw backend: %q %v", src, err)
			}
		},
	)
}

func loginToV2Wordle(selector string) spec.Step {
	return spec.Custom("login to v2 Wordle", func(t testing.TB, ctx *duiruntime.Context) {
		frame := ctx.Page.FrameLocator(selector)
		if err := frame.Locator("input[name='username']").
			First().
			Fill("workbench-v2-e2e"); err != nil {
			t.Fatal(err)
		}
		if err := frame.GetByRole("button", playwright.FrameLocatorGetByRoleOptions{Name: "Play"}).
			Click(); err != nil {
			t.Fatal(err)
		}
	})
}

func submitV2WordleGuess(selector, guess string) spec.Step {
	return spec.Custom(
		"submit v2 Wordle guess",
		func(t testing.TB, ctx *duiruntime.Context) {
			frame := ctx.Page.FrameLocator(selector)
			for _, letter := range strings.ToUpper(guess) {
				if err := frame.GetByRole("button", playwright.FrameLocatorGetByRoleOptions{Name: "Letter " + string(letter), Exact: playwright.Bool(true)}).
					Click(); err != nil {
					t.Fatal(err)
				}
			}
			if err := frame.GetByRole("button", playwright.FrameLocatorGetByRoleOptions{Name: "Submit guess", Exact: playwright.Bool(true)}).
				Click(); err != nil {
				t.Fatal(err)
			}
			if err := frame.GetByText("1/6", playwright.FrameLocatorGetByTextOptions{Exact: playwright.Bool(true)}).
				WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(30_000)}); err != nil {
				t.Fatal(err)
			}
		},
	)
}

func rememberV2StreamlitIdentity(selector string) spec.Step {
	return spec.Custom(
		"remember v2 Streamlit process identity",
		func(t testing.TB, ctx *duiruntime.Context) {
			marker := ctx.Page.FrameLocator(selector).
				GetByText("Process PID", playwright.FrameLocatorGetByTextOptions{Exact: playwright.Bool(false)}).
				First()
			if err := marker.WaitFor(
				playwright.LocatorWaitForOptions{Timeout: playwright.Float(30_000)},
			); err != nil {
				t.Fatal(err)
			}
			identity, err := readV2StreamlitProcessIdentity(ctx, selector)
			if err != nil ||
				(identity.PID == "" && identity.RunID == "" && identity.StartedAt == "") {
				t.Fatalf("missing v2 Streamlit identity: %+v %v", identity, err)
			}
			ctx.Memory["workbench-v2.streamlit-identity"] = identity.PID + "|" + identity.RunID + "|" + identity.StartedAt
		},
	)
}

func rememberStreamlitConnectionCount(probe *streamlitBrowserProbe) spec.Step {
	return spec.Custom(
		"remember scoped Streamlit connection count",
		func(t testing.TB, ctx *duiruntime.Context) {
			probe.mu.Lock()
			defer probe.mu.Unlock()
			if len(probe.urls) == 0 {
				t.Fatal("initial _stcore/stream connection missing")
			}
			ctx.Memory["workbench-v2.streamlit-connections"] = fmt.Sprint(len(probe.urls))
		},
	)
}

func expectNewStreamlitConnection(probe *streamlitBrowserProbe) spec.Step {
	return spec.Custom(
		"return opens a new scoped Streamlit connection",
		func(t testing.TB, ctx *duiruntime.Context) {
			before := ctx.Memory["workbench-v2.streamlit-connections"]
			deadline := time.Now().Add(30 * time.Second)
			for time.Now().Before(deadline) {
				probe.mu.Lock()
				count := len(probe.urls)
				urls := append([]string(nil), probe.urls...)
				probe.mu.Unlock()
				if fmt.Sprint(count) != before {
					for _, url := range urls {
						if strings.Contains(url, "/thoughts/_render/app/") &&
							strings.Contains(url, "_stcore/stream") {
							return
						}
					}
				}
				time.Sleep(100 * time.Millisecond)
			}
			t.Fatalf(
				"no new scoped Streamlit connection after return; initial count %s",
				before,
			)
		},
	)
}

func expectV2StreamlitIdentityReused(selector string) spec.Step {
	return spec.Custom(
		"v2 Streamlit reuses managed process",
		func(t testing.TB, ctx *duiruntime.Context) {
			identity, err := readV2StreamlitProcessIdentity(ctx, selector)
			got := identity.PID + "|" + identity.RunID + "|" + identity.StartedAt
			if err != nil || got != ctx.Memory["workbench-v2.streamlit-identity"] {
				t.Fatalf("Streamlit process changed after navigation: %q %v", got, err)
			}
		},
	)
}

func readV2StreamlitProcessIdentity(
	ctx *duiruntime.Context,
	selector string,
) (streamlitProcessIdentity, error) {
	return readStreamlitProcessIdentityForFrame(ctx, selector)
}

func incrementV2StreamlitCounter(selector string) spec.Step {
	return spec.Custom(
		"increment v2 Streamlit counter",
		func(t testing.TB, ctx *duiruntime.Context) {
			app := ctx.Page.FrameLocator(selector)
			if err := app.GetByRole("button", playwright.FrameLocatorGetByRoleOptions{Name: "Increment Streamlit session counter"}).
				Click(); err != nil {
				t.Fatal(err)
			}
			if err := app.GetByText("Session counter: 1").
				First().
				WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(30_000)}); err != nil {
				t.Fatal(err)
			}
		},
	)
}
