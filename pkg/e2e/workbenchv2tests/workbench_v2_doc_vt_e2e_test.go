package tests

import (
	"net/url"
	"strings"
	"testing"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"
	"github.com/playwright-community/playwright-go"

	"github.com/CoreyCole/vamos/pkg/e2e/fixtures"
	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

func TestWorkbenchV2MobileSiblingDocKeepsChromeUnderTabs(t *testing.T) {
	const (
		designPath = "thoughts/owner/plans/alpha/design.md"
		notesPath  = "thoughts/owner/plans/alpha/notes.md"
	)
	designHref := "/threads/wb2_alpha?artifact=" + url.QueryEscape(designPath) +
		"&artifact_dir=" + url.QueryEscape("thoughts/owner/plans/alpha")
	notesHref := "/threads/wb2_alpha?artifact=" + url.QueryEscape(notesPath) +
		"&artifact_dir=" + url.QueryEscape("thoughts/owner/plans/alpha")

	spec.Story(t, "workbench v2 mobile sibling doc keeps chrome under tabs").
		App(vamos.App()).
		Viewport(duiruntime.ViewportMobile).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Path(designHref)).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(assertDocsTabSSRSelected()).
		Do(assertMobileDocChromeViewTransitionNames()).
		Do(clickSiblingAndAssertNoUnderTabsBlackout(notesHref, "Alpha notes")).
		Expect(vamos.WorkbenchV2.Ready()).
		Expect(spec.TextContains(vamos.WorkbenchV2.Artifact(), "Alpha notes")).
		Do(assertDocsTabStillSelected()).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestWorkbenchV2DesktopSiblingDocKeepsChromeUnderHeader(t *testing.T) {
	const (
		designPath = "thoughts/owner/plans/alpha/design.md"
		notesPath  = "thoughts/owner/plans/alpha/notes.md"
	)
	designHref := "/threads/wb2_alpha?artifact=" + url.QueryEscape(designPath) +
		"&artifact_dir=" + url.QueryEscape("thoughts/owner/plans/alpha")
	notesHref := "/threads/wb2_alpha?artifact=" + url.QueryEscape(notesPath) +
		"&artifact_dir=" + url.QueryEscape("thoughts/owner/plans/alpha")

	spec.Story(t, "workbench v2 desktop sibling doc keeps chrome under header").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Path(designHref)).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(assertDesktopDocChromeViewTransitionNames()).
		Do(clickSiblingAndAssertNoUnderHeaderBlackout(notesHref, "Alpha notes")).
		Expect(vamos.WorkbenchV2.Ready()).
		Expect(spec.TextContains(vamos.WorkbenchV2.Artifact(), "Alpha notes")).
		Do(assertChatPinnedTight("after sibling nav real overflow scroller room <=2px")).
		Do(assertChatPinnedAfterReload()).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestWorkbenchV2DesktopSiblingDocKeepsThreadsReopenChrome(t *testing.T) {
	const (
		designPath = "thoughts/owner/plans/alpha/design.md"
		notesPath  = "thoughts/owner/plans/alpha/notes.md"
	)
	designHref := "/threads/wb2_alpha?artifact=" + url.QueryEscape(designPath) +
		"&artifact_dir=" + url.QueryEscape("thoughts/owner/plans/alpha")
	notesHref := "/threads/wb2_alpha?artifact=" + url.QueryEscape(notesPath) +
		"&artifact_dir=" + url.QueryEscape("thoughts/owner/plans/alpha")

	spec.Story(t, "workbench v2 desktop sibling doc keeps threads reopen chrome").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Path(designHref)).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(hideWorkbenchThreadsSidebar()).
		Do(assertDesktopThreadsReopenViewTransitionName()).
		Do(clickSiblingAndAssertThreadsReopenSurvives(notesHref, "Alpha notes")).
		Expect(vamos.WorkbenchV2.Ready()).
		Expect(spec.TextContains(vamos.WorkbenchV2.Artifact(), "Alpha notes")).
		Do(assertThreadsStillClosedWithReopen()).
		Expect(vamos.Console.Clean()).
		Run()
}

func hideWorkbenchThreadsSidebar() spec.Step {
	return spec.Custom(
		"hide threads sidebar so reopen chrome is the live named control",
		func(t testing.TB, ctx *duiruntime.Context) {
			region := ctx.Page.Locator("#workbench-v2-threads")
			if err := region.Locator("button[data-workbench-threads-toggle]").
				Click(); err != nil {
				t.Fatal(err)
			}
			if err := region.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateHidden,
				Timeout: playwright.Float(10_000),
			}); err != nil {
				t.Fatalf("threads sidebar did not hide: %v", err)
			}
			reopen := ctx.Page.Locator("#workbench-v2-threads-reopen")
			if err := reopen.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(10_000),
			}); err != nil {
				t.Fatalf("threads reopen chrome did not appear: %v", err)
			}
		},
	)
}

func TestWorkbenchV2DesktopThreadSwitchKeepsChatUnderHeader(t *testing.T) {
	// Residual: large black gap under chat chrome after thread->thread GET (Alpha),
	// with threads sidebar open. Distinct from sibling-artifact under-header Story
	// (same thread; artifact only). Chat column remounts; VT still names+freezes
	// #workbench-v2-chat as unchanged chrome.
	spec.Story(t, "workbench v2 desktop thread switch keeps chat under header").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Path("/threads/wb2_beta")).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(assertThreadsSidebarOpen()).
		Do(assertDesktopDocChromeViewTransitionNames()).
		Do(clickThreadAndAssertNoChatUnderHeaderGap(
			"#workbench-v2-threads-body a[href='/threads/wb2_alpha']",
			"WB2_CHAT_JANK_ASSIST_12",
		)).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(assertThreadsSidebarOpen()).
		Expect(spec.TextContains(vamos.WorkbenchV2.Chat(), "WB2_CHAT_JANK_USER_01")).
		Expect(spec.TextContains(vamos.WorkbenchV2.Chat(), "WB2_CHAT_JANK_ASSIST_12")).
		Do(assertChatPinnedTight("after thread switch real overflow scroller room <=2px")).
		Do(assertNoLargeGapUnderAppHeaderInChatColumn("after thread switch settle")).
		Expect(vamos.Console.Clean()).
		Run()
}

func assertDesktopThreadsReopenViewTransitionName() spec.Step {
	return spec.Custom(
		"desktop #workbench-v2-threads-reopen has stable view-transition-name",
		func(t testing.TB, ctx *duiruntime.Context) {
			value, err := ctx.Page.Evaluate(
				`() => {
					const el = document.getElementById('workbench-v2-threads-reopen');
					if (!el) return null;
					const name = getComputedStyle(el).viewTransitionName || el.style.viewTransitionName || null;
					return (!name || name === 'none') ? 'none' : name;
				}`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if value != "workbench-v2-threads-reopen" {
				t.Fatalf("threads-reopen view-transition-name = %#v, want workbench-v2-threads-reopen", value)
			}
		},
	)
}

func clickSiblingAndAssertThreadsReopenSurvives(href, wantText string) spec.Step {
	return spec.Custom(
		"sibling GET keeps closed-threads reopen chrome painted under surviving header",
		func(t testing.TB, ctx *duiruntime.Context) {
			link := ctx.Page.Locator(
				"[data-thread-artifact-browser] a[data-thread-artifact-file][href='" + href + "']",
			).First()
			if err := link.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(15_000),
			}); err != nil {
				t.Fatalf("sibling link missing: %v", err)
			}

			_, err := ctx.Page.Evaluate(
				`() => {
					sessionStorage.removeItem('wb2DesktopVtReopenSamples');
					window.__wb2DesktopVtReopenSamples = [];
					window.__wb2DesktopVtReopenSampling = true;
					const box = (el) => {
						if (!el) return null;
						const r = el.getBoundingClientRect();
						const cs = getComputedStyle(el);
						return {
							h: r.height, w: r.width,
							opacity: cs.opacity,
							visibility: cs.visibility,
							display: cs.display,
							inDom: document.contains(el),
						};
					};
					const take = (phase) => {
						window.__wb2DesktopVtReopenSamples.push({
							phase,
							t: performance.now(),
							header: box(document.getElementById('app-header')),
							reopen: box(document.getElementById('workbench-v2-threads-reopen')),
							chat: box(document.getElementById('workbench-v2-chat')),
							path: box(document.getElementById('thread-artifact-path-header')),
							browser: box(document.getElementById('thread-artifact-browser')),
						});
					};
					const sample = () => {
						if (!window.__wb2DesktopVtReopenSampling) return;
						take('pre');
						if (window.__wb2DesktopVtReopenSamples.length < 20) {
							requestAnimationFrame(sample);
						}
					};
					const persist = () => {
						try {
							sessionStorage.setItem(
								'wb2DesktopVtReopenSamples',
								JSON.stringify(window.__wb2DesktopVtReopenSamples || []),
							);
						} catch (_) {}
					};
					window.addEventListener('pagehide', () => { take('pagehide'); persist(); });
					window.addEventListener('pageshow', () => {
						window.__wb2DesktopVtReopenSamples = JSON.parse(
							sessionStorage.getItem('wb2DesktopVtReopenSamples') || '[]',
						);
						window.__wb2DesktopVtReopenSampling = true;
						let n = 0;
						const post = () => {
							take('post');
							persist();
							if (++n < 12) requestAnimationFrame(post);
							else window.__wb2DesktopVtReopenSampling = false;
						};
						requestAnimationFrame(post);
					});
					requestAnimationFrame(sample);
					return true;
				}`,
				nil,
			)
			if err != nil {
				t.Fatalf("start rAF sampler: %v", err)
			}

			_, err = ctx.Page.ExpectNavigation(
				func() error { return link.Click() },
				playwright.PageExpectNavigationOptions{
					WaitUntil: playwright.WaitUntilStateDomcontentloaded,
				},
			)
			if err != nil {
				t.Fatalf("sibling GET navigation not observed: %v", err)
			}

			_, _ = ctx.Page.Evaluate(
				`() => new Promise(r => {
					let n = 0;
					const tick = () => { if (++n >= 16) r(true); else requestAnimationFrame(tick); };
					requestAnimationFrame(tick);
				})`,
				nil,
			)

			samples, err := ctx.Page.Evaluate(
				`() => {
					const fromMem = window.__wb2DesktopVtReopenSamples;
					if (fromMem && fromMem.length) return fromMem;
					try { return JSON.parse(sessionStorage.getItem('wb2DesktopVtReopenSamples') || '[]'); }
					catch (_) { return []; }
				}`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			list, ok := samples.([]any)
			if !ok || len(list) < 3 {
				t.Fatalf("expected >=3 rAF samples, got %#v", samples)
			}

			for i, raw := range list {
				s, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				header, _ := s["header"].(map[string]any)
				reopen, _ := s["reopen"].(map[string]any)
				chat, _ := s["chat"].(map[string]any)
				path, _ := s["path"].(map[string]any)
				browser, _ := s["browser"].(map[string]any)
				if !boxVisible(header) {
					continue
				}
				for name, b := range map[string]map[string]any{
					"reopen":  reopen,
					"chat":    chat,
					"path":    path,
					"browser": browser,
				} {
					if b == nil || !boxInDOM(b) {
						t.Fatalf("sample %d: %s missing from DOM while header visible", i, name)
					}
					if boxCollapsed(b) || boxHidden(b) {
						t.Fatalf(
							"sample %d: under-header black — %s collapsed/hidden while header visible: %#v",
							i, name, b,
						)
					}
				}
				if op, _ := reopen["opacity"].(string); op != "" && op != "1" {
					t.Fatalf("sample %d: threads-reopen opacity = %q want 1: %#v", i, op, reopen)
				}
			}

			if err := ctx.Page.Locator("#thread-artifact-document").
				GetByText(wantText).
				First().
				WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(30_000),
				}); err != nil {
				t.Fatalf("document missing %q after sibling: %v", wantText, err)
			}
		},
	)
}

func assertThreadsStillClosedWithReopen() spec.Step {
	return spec.Custom(
		"threads stay closed after sibling GET; reopen chrome + wb2_threads_open=0",
		func(t testing.TB, ctx *duiruntime.Context) {
			region := ctx.Page.Locator("#workbench-v2-threads")
			visible, err := region.IsVisible()
			if err != nil {
				t.Fatal(err)
			}
			if visible {
				t.Fatal("threads sidebar reopened across sibling GET")
			}
			reopen := ctx.Page.Locator("#workbench-v2-threads-reopen")
			if err := reopen.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(10_000),
			}); err != nil {
				t.Fatalf("threads reopen chrome missing after sibling: %v", err)
			}
			cookie, err := ctx.Page.Evaluate(
				`() => {
					const match = document.cookie.match(/(?:^|; )wb2_threads_open=([^;]*)/);
					return match ? match[1] : null;
				}`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if cookie != "0" {
				t.Fatalf("wb2_threads_open = %#v, want 0", cookie)
			}
			name, err := ctx.Page.Evaluate(
				`() => {
					const el = document.getElementById('workbench-v2-threads-reopen');
					if (!el) return null;
					const n = getComputedStyle(el).viewTransitionName || el.style.viewTransitionName || null;
					return (!n || n === 'none') ? 'none' : n;
				}`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if name != "workbench-v2-threads-reopen" {
				t.Fatalf("post-nav threads-reopen VT name = %#v", name)
			}
		},
	)
}

func assertDesktopDocChromeViewTransitionNames() spec.Step {
	return spec.Custom(
		"desktop chrome VT names: header/threads/threads-reopen/chat/path/browser named; mobile-tabs/regions/artifact none; document named",
		func(t testing.TB, ctx *duiruntime.Context) {
			value, err := ctx.Page.Evaluate(
				`() => {
					const read = (id) => {
						const el = document.getElementById(id);
						if (!el) return null;
						const name = getComputedStyle(el).viewTransitionName || el.style.viewTransitionName || null;
						return (!name || name === 'none') ? 'none' : name;
					};
					return {
						header: read('app-header'),
						tabs: read('workbench-mobile-tabs'),
						threads: read('workbench-v2-threads'),
						threadsReopen: read('workbench-v2-threads-reopen'),
						chat: read('workbench-v2-chat'),
						regions: read('workbench-regions'),
						root: read('workbench-root'),
						artifact: read('workbench-v2-artifact'),
						path: read('thread-artifact-path-header'),
						browser: read('thread-artifact-browser'),
						document: read('thread-artifact-document'),
					};
				}`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			state, ok := value.(map[string]any)
			if !ok {
				t.Fatalf("VT probe type %T", value)
			}
			checks := map[string]string{
				"header":        "app-header",
				"tabs":          "none",
				"threads":       "workbench-v2-threads",
				"threadsReopen": "workbench-v2-threads-reopen",
				"chat":          "workbench-v2-chat",
				"path":          "thread-artifact-path-header",
				"browser":       "thread-artifact-browser",
				"document":      "thread-artifact-document",
				"artifact":      "none",
				"regions":       "none",
				"root":          "none",
			}
			for key, want := range checks {
				if state[key] != want {
					t.Fatalf("%s view-transition-name = %#v, want %s", key, state[key], want)
				}
			}
		},
	)
}

func clickSiblingAndAssertNoUnderHeaderBlackout(href, wantText string) spec.Step {
	return spec.Custom(
		"sibling GET never blacks out threads/chat/path/browser under surviving header",
		func(t testing.TB, ctx *duiruntime.Context) {
			link := ctx.Page.Locator(
				"[data-thread-artifact-browser] a[data-thread-artifact-file][href='" + href + "']",
			).First()
			if err := link.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(15_000),
			}); err != nil {
				t.Fatalf("sibling link missing: %v", err)
			}

			_, err := ctx.Page.Evaluate(
				`() => {
					sessionStorage.removeItem('wb2DesktopVtSamples');
					window.__wb2DesktopVtSamples = [];
					window.__wb2DesktopVtSampling = true;
					const box = (el) => {
						if (!el) return null;
						const r = el.getBoundingClientRect();
						const cs = getComputedStyle(el);
						return {
							h: r.height, w: r.width,
							opacity: cs.opacity,
							visibility: cs.visibility,
							display: cs.display,
							inDom: document.contains(el),
						};
					};
					const take = (phase) => {
						window.__wb2DesktopVtSamples.push({
							phase,
							t: performance.now(),
							header: box(document.getElementById('app-header')),
							threads: box(document.getElementById('workbench-v2-threads')),
							chat: box(document.getElementById('workbench-v2-chat')),
							path: box(document.getElementById('thread-artifact-path-header')),
							browser: box(document.getElementById('thread-artifact-browser')),
						});
					};
					const sample = () => {
						if (!window.__wb2DesktopVtSampling) return;
						take('pre');
						if (window.__wb2DesktopVtSamples.length < 20) {
							requestAnimationFrame(sample);
						}
					};
					const persist = () => {
						try {
							sessionStorage.setItem('wb2DesktopVtSamples', JSON.stringify(window.__wb2DesktopVtSamples || []));
						} catch (_) {}
					};
					window.addEventListener('pagehide', () => { take('pagehide'); persist(); });
					window.addEventListener('pageshow', () => {
						window.__wb2DesktopVtSamples = JSON.parse(sessionStorage.getItem('wb2DesktopVtSamples') || '[]');
						window.__wb2DesktopVtSampling = true;
						let n = 0;
						const post = () => {
							take('post');
							persist();
							if (++n < 12) requestAnimationFrame(post);
							else window.__wb2DesktopVtSampling = false;
						};
						requestAnimationFrame(post);
					});
					requestAnimationFrame(sample);
					return true;
				}`,
				nil,
			)
			if err != nil {
				t.Fatalf("start rAF sampler: %v", err)
			}

			_, err = ctx.Page.ExpectNavigation(
				func() error { return link.Click() },
				playwright.PageExpectNavigationOptions{
					WaitUntil: playwright.WaitUntilStateDomcontentloaded,
				},
			)
			if err != nil {
				t.Fatalf("sibling GET navigation not observed: %v", err)
			}

			_, _ = ctx.Page.Evaluate(
				`() => new Promise(r => {
					let n = 0;
					const tick = () => { if (++n >= 16) r(true); else requestAnimationFrame(tick); };
					requestAnimationFrame(tick);
				})`,
				nil,
			)

			samples, err := ctx.Page.Evaluate(
				`() => {
					const fromMem = window.__wb2DesktopVtSamples;
					if (fromMem && fromMem.length) return fromMem;
					try { return JSON.parse(sessionStorage.getItem('wb2DesktopVtSamples') || '[]'); }
					catch (_) { return []; }
				}`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			list, ok := samples.([]any)
			if !ok || len(list) < 3 {
				t.Fatalf("expected >=3 rAF samples, got %#v", samples)
			}

			for i, raw := range list {
				s, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				header, _ := s["header"].(map[string]any)
				threads, _ := s["threads"].(map[string]any)
				chat, _ := s["chat"].(map[string]any)
				path, _ := s["path"].(map[string]any)
				browser, _ := s["browser"].(map[string]any)
				headerVisible := boxVisible(header)
				if headerVisible {
					for name, b := range map[string]map[string]any{
						"threads": threads,
						"chat":    chat,
						"path":    path,
						"browser": browser,
					} {
						if b == nil || !boxInDOM(b) {
							t.Fatalf("sample %d: %s missing from DOM while header visible", i, name)
						}
						if boxCollapsed(b) || boxHidden(b) {
							t.Fatalf("sample %d: under-header black — %s collapsed/hidden while header visible: %#v", i, name, b)
						}
					}
					// Chat must stay fully opaque across sibling VT frames (no remount flash).
					if op, _ := chat["opacity"].(string); op != "" && op != "1" {
						t.Fatalf("sample %d: chat opacity = %q want 1 (column flash): %#v", i, op, chat)
					}
					if vis, _ := chat["visibility"].(string); vis != "" && vis != "visible" {
						t.Fatalf("sample %d: chat visibility = %q want visible: %#v", i, vis, chat)
					}
				}
			}

			if err := ctx.Page.Locator("#thread-artifact-document").
				GetByText(wantText).
				First().
				WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(30_000),
				}); err != nil {
				t.Fatalf("document missing %q after sibling: %v", wantText, err)
			}
		},
	)
}

func assertChatPinnedTight(label string) spec.Step {
	return spec.Custom(
		label,
		func(t testing.TB, ctx *duiruntime.Context) {
			// Settle a couple frames so pageshow/rAF/fonts.ready pin can finish.
			for i := 0; i < 4; i++ {
				if _, err := ctx.Page.Evaluate(`() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))`, nil); err != nil {
					t.Fatalf("rAF settle: %v", err)
				}
			}
			value, err := ctx.Page.Evaluate(
				`() => {
					const candidates = [
						document.getElementById('agent-chat-messages'),
						document.getElementById('agent-chat-scroll-region'),
					].filter(Boolean);
					if (!candidates.length) return { hasRegion: false };
					let region = candidates[0];
					let bestOverflow = -1;
					for (const el of candidates) {
						const overflow = el.scrollHeight - el.clientHeight;
						if (overflow > bestOverflow) {
							region = el;
							bestOverflow = overflow;
						}
					}
					const room = Math.max(0, region.scrollHeight - region.scrollTop - region.clientHeight);
					return {
						hasRegion: true,
						id: region.id,
						scrollTop: region.scrollTop,
						scrollHeight: region.scrollHeight,
						clientHeight: region.clientHeight,
						room,
						pinned: room <= 2,
					};
				}`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			state, ok := value.(map[string]any)
			if !ok {
				t.Fatalf("chat pin probe type %T", value)
			}
			hasRegion, _ := state["hasRegion"].(bool)
			if !hasRegion {
				t.Fatalf("missing chat overflow scroller: %#v", state)
			}
			pinned, _ := state["pinned"].(bool)
			if !pinned {
				t.Fatalf("chat still has scroll room (want scrollHeight-scrollTop-clientHeight <= 2): %#v", state)
			}
		},
	)
}

func assertChatPinnedAfterReload() spec.Step {
	return spec.Custom(
		"after full reload real overflow scroller room <=2px",
		func(t testing.TB, ctx *duiruntime.Context) {
			if _, err := ctx.Page.Reload(playwright.PageReloadOptions{
				WaitUntil: playwright.WaitUntilStateLoad,
				Timeout:   playwright.Float(30_000),
			}); err != nil {
				t.Fatalf("reload: %v", err)
			}
			if err := ctx.Page.Locator("#workbench-v2-chat, #agent-chat-messages, #agent-chat-scroll-region").First().
				WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(30_000),
				}); err != nil {
				t.Fatalf("chat missing after reload: %v", err)
			}
			// Inline the same probe (Custom steps are not re-entrant via .Fn).
			for i := 0; i < 4; i++ {
				if _, err := ctx.Page.Evaluate(`() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))`, nil); err != nil {
					t.Fatalf("rAF settle: %v", err)
				}
			}
			value, err := ctx.Page.Evaluate(
				`() => {
					const candidates = [
						document.getElementById('agent-chat-messages'),
						document.getElementById('agent-chat-scroll-region'),
					].filter(Boolean);
					if (!candidates.length) return { hasRegion: false };
					let region = candidates[0];
					let bestOverflow = -1;
					for (const el of candidates) {
						const overflow = el.scrollHeight - el.clientHeight;
						if (overflow > bestOverflow) {
							region = el;
							bestOverflow = overflow;
						}
					}
					const room = Math.max(0, region.scrollHeight - region.scrollTop - region.clientHeight);
					return {
						hasRegion: true,
						id: region.id,
						scrollTop: region.scrollTop,
						scrollHeight: region.scrollHeight,
						clientHeight: region.clientHeight,
						room,
						pinned: room <= 2,
					};
				}`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			state, ok := value.(map[string]any)
			if !ok {
				t.Fatalf("chat pin probe type %T", value)
			}
			hasRegion, _ := state["hasRegion"].(bool)
			if !hasRegion {
				t.Fatalf("missing chat overflow scroller after reload: %#v", state)
			}
			pinned, _ := state["pinned"].(bool)
			if !pinned {
				t.Fatalf("chat still has scroll room after reload (want <= 2px): %#v", state)
			}
		},
	)
}

func assertDocsTabSSRSelected() spec.Step {
	return spec.Custom(
		"Docs tab paints selected in SSR HTML before sibling click",
		func(t testing.TB, ctx *duiruntime.Context) {
			// Prefer live DOM outerHTML; also re-GET document to confirm SSR attributes
			// (not only post-Datastar wait state).
			pageURL := ctx.Page.URL()
			api := ctx.Page.Context().Request()
			resp, err := api.Get(pageURL)
			if err != nil {
				t.Fatalf("GET SSR HTML: %v", err)
			}
			body, err := resp.Text()
			if err != nil {
				t.Fatalf("read SSR body: %v", err)
			}
			if !strings.Contains(body, `id="workbench-mobile-tabs"`) {
				t.Fatalf("SSR missing workbench-mobile-tabs")
			}
			docsIdx := strings.Index(body, ">Docs</button>")
			if docsIdx < 0 {
				// templ may insert whitespace/newlines
				docsIdx = strings.Index(body, ">Docs<")
			}
			if docsIdx < 0 {
				t.Fatalf("SSR missing Docs tab button text")
			}
			// Look back to the opening <button for Docs.
			start := strings.LastIndex(body[:docsIdx], "<button")
			if start < 0 {
				t.Fatalf("Docs button open tag missing")
			}
			btn := body[start:docsIdx]
			if !strings.Contains(btn, `aria-selected="true"`) {
				t.Fatalf("Docs tab SSR aria-selected not true: %s", btn)
			}
			if !strings.Contains(btn, "bg-muted") || !strings.Contains(btn, "text-foreground") {
				t.Fatalf("Docs tab SSR missing selected classes: %s", btn)
			}
			// data-signals may be HTML-attribute escaped (&quot;) in raw GET body.
			hasSignal := strings.Contains(body, `"activeRegionID":"workbenchV2Artifact"`) ||
				strings.Contains(body, `activeRegionID&quot;:&quot;workbenchV2Artifact`) ||
				strings.Contains(body, `data-workbench-mobile-active="workbenchV2Artifact"`)
			if !hasSignal {
				t.Fatalf("SSR missing workbenchV2Artifact activeRegionID signal/attr")
			}
		},
	)
}

func assertDocsTabStillSelected() spec.Step {
	return spec.Custom(
		"Docs tab remains aria-selected after sibling nav",
		func(t testing.TB, ctx *duiruntime.Context) {
			selected, err := ctx.Page.Locator(`#workbench-mobile-tabs button[role="tab"]`).
				Filter(playwright.LocatorFilterOptions{HasText: "Docs"}).
				GetAttribute("aria-selected")
			if err != nil || selected != "true" {
				t.Fatalf("Docs aria-selected=%q err=%v", selected, err)
			}
		},
	)
}

func assertMobileDocChromeViewTransitionNames() spec.Step {
	return spec.Custom(
		"mobile chrome VT names: tabs/path/browser/chat named; artifact none; document named",
		func(t testing.TB, ctx *duiruntime.Context) {
			value, err := ctx.Page.Evaluate(
				`() => {
					const read = (id) => {
						const el = document.getElementById(id);
						if (!el) return null;
						const name = getComputedStyle(el).viewTransitionName || el.style.viewTransitionName || null;
						return (!name || name === 'none') ? 'none' : name;
					};
					return {
						tabs: read('workbench-mobile-tabs'),
						chat: read('workbench-v2-chat'),
						artifact: read('workbench-v2-artifact'),
						path: read('thread-artifact-path-header'),
						browser: read('thread-artifact-browser'),
						document: read('thread-artifact-document'),
					};
				}`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			state, ok := value.(map[string]any)
			if !ok {
				t.Fatalf("VT probe type %T", value)
			}
			checks := map[string]string{
				"tabs":     "workbench-mobile-tabs",
				"chat":     "workbench-v2-chat",
				"path":     "thread-artifact-path-header",
				"browser":  "thread-artifact-browser",
				"document": "thread-artifact-document",
				"artifact": "none",
			}
			for key, want := range checks {
				if state[key] != want {
					t.Fatalf("%s view-transition-name = %#v, want %s", key, state[key], want)
				}
			}
		},
	)
}

func clickSiblingAndAssertNoUnderTabsBlackout(href, wantText string) spec.Step {
	return spec.Custom(
		"sibling GET never blacks out path/browser under surviving tabs",
		func(t testing.TB, ctx *duiruntime.Context) {
			link := ctx.Page.Locator(
				"[data-thread-artifact-browser] a[data-thread-artifact-file][href='" + href + "']",
			).First()
			if err := link.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(15_000),
			}); err != nil {
				t.Fatalf("sibling link missing: %v", err)
			}

			// Persist rAF samples across cross-document VT via sessionStorage
			// (in-page JS state dies on navigation commit).
			_, err := ctx.Page.Evaluate(
				`() => {
					sessionStorage.removeItem('wb2VtSamples');
					window.__wb2VtSamples = [];
					window.__wb2VtSampling = true;
					const box = (el) => {
						if (!el) return null;
						const r = el.getBoundingClientRect();
						const cs = getComputedStyle(el);
						return {
							h: r.height, w: r.width,
							opacity: cs.opacity,
							visibility: cs.visibility,
							display: cs.display,
							inDom: document.contains(el),
						};
					};
					const take = (phase) => {
						const tabs = document.getElementById('workbench-mobile-tabs');
						const path = document.getElementById('thread-artifact-path-header');
						const browser = document.getElementById('thread-artifact-browser');
						const docs = [...(tabs?.querySelectorAll('button[role="tab"]') || [])]
							.find(b => (b.textContent || '').trim() === 'Docs');
						window.__wb2VtSamples.push({
							phase,
							t: performance.now(),
							docsSelected: docs?.getAttribute('aria-selected'),
							tabs: box(tabs),
							path: box(path),
							browser: box(browser),
						});
					};
					const sample = () => {
						if (!window.__wb2VtSampling) return;
						take('pre');
						if (window.__wb2VtSamples.length < 20) {
							requestAnimationFrame(sample);
						}
					};
					const persist = () => {
						try {
							sessionStorage.setItem('wb2VtSamples', JSON.stringify(window.__wb2VtSamples || []));
						} catch (_) {}
					};
					window.addEventListener('pagehide', () => { take('pagehide'); persist(); });
					window.addEventListener('pageshow', () => {
						window.__wb2VtSamples = JSON.parse(sessionStorage.getItem('wb2VtSamples') || '[]');
						window.__wb2VtSampling = true;
						let n = 0;
						const post = () => {
							take('post');
							persist();
							if (++n < 12) requestAnimationFrame(post);
							else window.__wb2VtSampling = false;
						};
						requestAnimationFrame(post);
					});
					requestAnimationFrame(sample);
					return true;
				}`,
				nil,
			)
			if err != nil {
				t.Fatalf("start rAF sampler: %v", err)
			}

			_, err = ctx.Page.ExpectNavigation(
				func() error { return link.Click() },
				playwright.PageExpectNavigationOptions{
					WaitUntil: playwright.WaitUntilStateDomcontentloaded,
				},
			)
			if err != nil {
				t.Fatalf("sibling GET navigation not observed: %v", err)
			}

			// Continue a few post-nav frames (pageshow handler + extra rAFs).
			_, _ = ctx.Page.Evaluate(
				`() => new Promise(r => {
					let n = 0;
					const tick = () => { if (++n >= 16) r(true); else requestAnimationFrame(tick); };
					requestAnimationFrame(tick);
				})`,
				nil,
			)

			samples, err := ctx.Page.Evaluate(
				`() => {
					const fromMem = window.__wb2VtSamples;
					if (fromMem && fromMem.length) return fromMem;
					try { return JSON.parse(sessionStorage.getItem('wb2VtSamples') || '[]'); }
					catch (_) { return []; }
				}`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			list, ok := samples.([]any)
			if !ok || len(list) < 3 {
				t.Fatalf("expected >=3 rAF samples, got %#v", samples)
			}

			for i, raw := range list {
				s, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				if sel, _ := s["docsSelected"].(string); sel == "false" {
					t.Fatalf("sample %d: Docs aria-selected went false", i)
				}
				tabs, _ := s["tabs"].(map[string]any)
				path, _ := s["path"].(map[string]any)
				browser, _ := s["browser"].(map[string]any)
				tabsVisible := boxVisible(tabs)
				// Fail on under-tabs black: tabs still visible but path/browser height≈0 / hidden.
				if tabsVisible {
					if path == nil || !boxInDOM(path) {
						t.Fatalf("sample %d: path-header missing from DOM while tabs visible", i)
					}
					if browser == nil || !boxInDOM(browser) {
						t.Fatalf("sample %d: browser missing from DOM while tabs visible", i)
					}
					if boxCollapsed(path) || boxHidden(path) {
						t.Fatalf("sample %d: under-tabs black — path-header collapsed/hidden while tabs visible: %#v", i, path)
					}
					if boxCollapsed(browser) || boxHidden(browser) {
						t.Fatalf("sample %d: under-tabs black — browser collapsed/hidden while tabs visible: %#v", i, browser)
					}
				}
			}

			if err := ctx.Page.Locator("#thread-artifact-document").
				GetByText(wantText).
				First().
				WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(30_000),
				}); err != nil {
				t.Fatalf("document missing %q after sibling: %v", wantText, err)
			}
		},
	)
}

func assertThreadsSidebarOpen() spec.Step {
	return spec.Custom(
		"threads sidebar region is open/visible on desktop",
		func(t testing.TB, ctx *duiruntime.Context) {
			region := ctx.Page.Locator("#workbench-v2-threads")
			if err := region.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(10_000),
			}); err != nil {
				t.Fatalf("threads sidebar not visible: %v", err)
			}
			box, err := region.BoundingBox()
			if err != nil || box == nil || box.Width < 8 || box.Height < 8 {
				t.Fatalf("threads sidebar collapsed: %#v err=%v", box, err)
			}
		},
	)
}

func assertNoLargeGapUnderAppHeaderInChatColumn(label string) spec.Step {
	return spec.Custom(
		label,
		func(t testing.TB, ctx *duiruntime.Context) {
			value, err := ctx.Page.Evaluate(
				`() => {
					const header = document.getElementById('app-header');
					const chat = document.getElementById('workbench-v2-chat');
					const body = document.getElementById('workbench-v2-chat-body');
					const scroll = document.getElementById('agent-chat-scroll-region');
					if (!header || !chat || !body || !scroll) {
						return { ok: false, reason: 'missing nodes' };
					}
					const hr = header.getBoundingClientRect();
					const cr = chat.getBoundingClientRect();
					const br = body.getBoundingClientRect();
					const sr = scroll.getBoundingClientRect();
					const headerToChat = cr.top - hr.bottom;
					const chatToScroll = sr.top - cr.top;
					const chatToBody = br.top - cr.top;
					return {
						ok: true,
						headerToChat,
						chatToScroll,
						chatToBody,
						chatH: cr.height,
						scrollH: sr.height,
						bodyH: br.height,
					};
				}`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			state, ok := value.(map[string]any)
			if !ok {
				t.Fatalf("gap probe type %T", value)
			}
			if okFlag, _ := state["ok"].(bool); !okFlag {
				t.Fatalf("gap probe failed: %#v", state)
			}
			headerToChat, _ := state["headerToChat"].(float64)
			chatToScroll, _ := state["chatToScroll"].(float64)
			if headerToChat > 48 {
				t.Fatalf("large gap under app-header above chat column: headerToChat=%.1fpx %#v", headerToChat, state)
			}
			if chatToScroll > 24 {
				t.Fatalf("large gap under chat column top before scroll region: chatToScroll=%.1fpx %#v", chatToScroll, state)
			}
			chatH, _ := state["chatH"].(float64)
			if chatH < 80 {
				t.Fatalf("chat column unexpectedly short: %#v", state)
			}
		},
	)
}

func clickThreadAndAssertNoChatUnderHeaderGap(linkSelector, wantChatText string) spec.Step {
	return spec.Custom(
		"thread->thread GET never blacks out/collapses chat under surviving header",
		func(t testing.TB, ctx *duiruntime.Context) {
			link := ctx.Page.Locator(linkSelector).First()
			if err := link.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(15_000),
			}); err != nil {
				t.Fatalf("thread link missing: %v", err)
			}

			_, err := ctx.Page.Evaluate(
				`() => {
					sessionStorage.removeItem('wb2ThreadVtSamples');
					window.__wb2ThreadVtSamples = [];
					window.__wb2ThreadVtSampling = true;
					const box = (el) => {
						if (!el) return null;
						const r = el.getBoundingClientRect();
						const cs = getComputedStyle(el);
						return {
							h: r.height, w: r.width, top: r.top,
							opacity: cs.opacity,
							visibility: cs.visibility,
							display: cs.display,
							inDom: document.contains(el),
						};
					};
					const take = (phase) => {
						const header = document.getElementById('app-header');
						const chat = document.getElementById('workbench-v2-chat');
						const scroll = document.getElementById('agent-chat-scroll-region');
						const hb = box(header);
						const cb = box(chat);
						const sb = box(scroll);
						let headerToChat = null;
						let chatToScroll = null;
						if (hb && cb) headerToChat = cb.top - (hb.top + hb.h);
						if (cb && sb) chatToScroll = sb.top - cb.top;
						window.__wb2ThreadVtSamples.push({
							phase,
							t: performance.now(),
							header: hb,
							threads: box(document.getElementById('workbench-v2-threads')),
							chat: cb,
							scroll: sb,
							headerToChat,
							chatToScroll,
						});
					};
					const sample = () => {
						if (!window.__wb2ThreadVtSampling) return;
						take('pre');
						if (window.__wb2ThreadVtSamples.length < 20) {
							requestAnimationFrame(sample);
						}
					};
					const persist = () => {
						try {
							sessionStorage.setItem('wb2ThreadVtSamples', JSON.stringify(window.__wb2ThreadVtSamples || []));
						} catch (_) {}
					};
					window.addEventListener('pagehide', () => { take('pagehide'); persist(); });
					window.addEventListener('pageshow', () => {
						window.__wb2ThreadVtSamples = JSON.parse(sessionStorage.getItem('wb2ThreadVtSamples') || '[]');
						window.__wb2ThreadVtSampling = true;
						let n = 0;
						const post = () => {
							take('post');
							persist();
							if (++n < 12) requestAnimationFrame(post);
							else window.__wb2ThreadVtSampling = false;
						};
						requestAnimationFrame(post);
					});
					requestAnimationFrame(sample);
					return true;
				}`,
				nil,
			)
			if err != nil {
				t.Fatalf("start rAF sampler: %v", err)
			}

			_, err = ctx.Page.ExpectNavigation(
				func() error { return link.Click() },
				playwright.PageExpectNavigationOptions{
					WaitUntil: playwright.WaitUntilStateDomcontentloaded,
				},
			)
			if err != nil {
				t.Fatalf("thread GET navigation not observed: %v", err)
			}

			_, _ = ctx.Page.Evaluate(
				`() => new Promise(r => {
					let n = 0;
					const tick = () => { if (++n >= 16) r(true); else requestAnimationFrame(tick); };
					requestAnimationFrame(tick);
				})`,
				nil,
			)

			samples, err := ctx.Page.Evaluate(
				`() => {
					const fromMem = window.__wb2ThreadVtSamples;
					if (fromMem && fromMem.length) return fromMem;
					try { return JSON.parse(sessionStorage.getItem('wb2ThreadVtSamples') || '[]'); }
					catch (_) { return []; }
				}`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			list, ok := samples.([]any)
			if !ok || len(list) < 3 {
				t.Fatalf("expected >=3 rAF samples, got %#v", samples)
			}

			for i, raw := range list {
				s, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				header, _ := s["header"].(map[string]any)
				threads, _ := s["threads"].(map[string]any)
				chat, _ := s["chat"].(map[string]any)
				headerVisible := boxVisible(header)
				if !headerVisible {
					continue
				}
				for name, b := range map[string]map[string]any{
					"threads": threads,
					"chat":    chat,
				} {
					if b == nil || !boxInDOM(b) {
						t.Fatalf("sample %d: %s missing from DOM while header visible", i, name)
					}
					if boxCollapsed(b) || boxHidden(b) {
						t.Fatalf("sample %d: under-header black — %s collapsed/hidden while header visible: %#v", i, name, b)
					}
				}
				if op, _ := chat["opacity"].(string); op != "" && op != "1" {
					t.Fatalf("sample %d: chat opacity = %q want 1 (column flash): %#v", i, op, chat)
				}
				if vis, _ := chat["visibility"].(string); vis != "" && vis != "visible" {
					t.Fatalf("sample %d: chat visibility = %q want visible: %#v", i, vis, chat)
				}
				if gap, ok := s["headerToChat"].(float64); ok && gap > 64 {
					t.Fatalf("sample %d: large gap under header above chat: headerToChat=%.1f %#v", i, gap, s)
				}
				if gap, ok := s["chatToScroll"].(float64); ok && gap > 32 {
					t.Fatalf("sample %d: large gap under chat top before scroll region: chatToScroll=%.1f %#v", i, gap, s)
				}
			}

			if err := ctx.Page.Locator("#workbench-v2-chat-body").
				GetByText(wantChatText).
				First().
				WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(30_000),
				}); err != nil {
				t.Fatalf("chat missing %q after thread switch: %v", wantChatText, err)
			}
		},
	)
}

func boxVisible(b map[string]any) bool {
	if b == nil {
		return false
	}
	h, _ := b["h"].(float64)
	w, _ := b["w"].(float64)
	return h > 1 && w > 1 && !boxHidden(b)
}

func boxInDOM(b map[string]any) bool {
	if b == nil {
		return false
	}
	in, _ := b["inDom"].(bool)
	return in
}

func boxCollapsed(b map[string]any) bool {
	if b == nil {
		return true
	}
	h, _ := b["h"].(float64)
	return h < 1
}

func boxHidden(b map[string]any) bool {
	if b == nil {
		return true
	}
	op, _ := b["opacity"].(string)
	vis, _ := b["visibility"].(string)
	disp, _ := b["display"].(string)
	return op == "0" || vis == "hidden" || disp == "none"
}
