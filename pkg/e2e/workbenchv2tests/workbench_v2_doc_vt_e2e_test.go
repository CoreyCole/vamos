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
		Do(assertChatPinnedAfterSiblingNav()).
		Expect(vamos.Console.Clean()).
		Run()
}

func assertDesktopDocChromeViewTransitionNames() spec.Step {
	return spec.Custom(
		"desktop chrome VT names: header/threads/chat/path/browser named; regions/artifact none; document named",
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
						threads: read('workbench-v2-threads'),
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
				"header":   "app-header",
				"threads":  "workbench-v2-threads",
				"chat":     "workbench-v2-chat",
				"path":     "thread-artifact-path-header",
				"browser":  "thread-artifact-browser",
				"document": "thread-artifact-document",
				"artifact": "none",
				"regions":  "none",
				"root":     "none",
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

func assertChatPinnedAfterSiblingNav() spec.Step {
	return spec.Custom(
		"after sibling nav #agent-chat-scroll-region scrollTop is near scrollHeight",
		func(t testing.TB, ctx *duiruntime.Context) {
			value, err := ctx.Page.Evaluate(
				`() => {
					const region = document.getElementById('agent-chat-scroll-region');
					if (!region) return { hasRegion: false };
					const max = Math.max(0, region.scrollHeight - region.clientHeight);
					const gap = max - region.scrollTop;
					return {
						hasRegion: true,
						scrollTop: region.scrollTop,
						scrollHeight: region.scrollHeight,
						clientHeight: region.clientHeight,
						max,
						gap,
						nearBottom: max <= 4 || gap <= 8,
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
				t.Fatalf("missing #agent-chat-scroll-region after sibling: %#v", state)
			}
			nearBottom, _ := state["nearBottom"].(bool)
			if !nearBottom {
				t.Fatalf("chat scroll not pinned after sibling (want scrollTop near scrollHeight): %#v", state)
			}
		},
	)
}
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
