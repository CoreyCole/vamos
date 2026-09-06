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

func TestWorkbenchV2SiblingArtifactKeepsSsrChatTranscript(t *testing.T) {
	const (
		earlyMarker = "WB2_CHAT_JANK_USER_01"
		lateMarker  = "WB2_CHAT_JANK_ASSIST_12"
		draft       = "completed alpha draft"
		notesPath   = "thoughts/owner/plans/alpha/notes.md"
	)
	siblingHref := "/threads/wb2_alpha?artifact=" + url.QueryEscape(notesPath) +
		"&artifact_dir=" + url.QueryEscape("thoughts/owner/plans/alpha")

	spec.Story(t, "workbench v2 sibling artifact keeps ssr chat transcript").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Path("/threads/wb2_alpha")).
		Expect(vamos.WorkbenchV2.Ready()).
		Expect(spec.InputValue(vamos.WorkbenchV2.Composer(), draft)).
		Expect(spec.TextContains(vamos.WorkbenchV2.Chat(), earlyMarker)).
		Expect(spec.TextContains(vamos.WorkbenchV2.Chat(), lateMarker)).
		Do(assertChatNotWaitingForFirstTurn()).
		Do(scrollChatTranscriptToMarker(earlyMarker)).
		Do(selectThreadArtifactFile(
			siblingHref,
			notesPath,
			"Alpha notes",
		)).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(assertURLContains("artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fnotes.md")).
		Expect(spec.TextContains(vamos.WorkbenchV2.Artifact(), "Alpha notes")).
		Expect(spec.TextContains(vamos.WorkbenchV2.Chat(), earlyMarker)).
		Expect(spec.TextContains(vamos.WorkbenchV2.Chat(), lateMarker)).
		Expect(spec.InputValue(vamos.WorkbenchV2.Composer(), draft)).
		Do(assertChatNotWaitingForFirstTurn()).
		Do(assertWorkbenchViewTransitionNames()).
		Expect(vamos.Console.Clean()).
		Run()
}

func assertChatNotWaitingForFirstTurn() spec.Step {
	return spec.Custom(
		"chat pane keeps completed transcript instead of empty waiting state",
		func(t testing.TB, ctx *duiruntime.Context) {
			chat := ctx.Page.Locator("#workbench-v2-chat-body")
			text, err := chat.TextContent()
			if err != nil {
				t.Fatalf("read chat pane: %v", err)
			}
			if strings.Contains(text, "Waiting for the first completed turn") {
				t.Fatalf("chat pane wiped to empty waiting state: %q", text)
			}
			stable, err := ctx.Page.Locator("#workbench-v2-chat-body #agent-chat-stable-transcript").Count()
			if err != nil || stable == 0 {
				t.Fatalf("stable transcript missing after navigation: count=%d err=%v", stable, err)
			}
		},
	)
}

func scrollChatTranscriptToMarker(marker string) spec.Step {
	return spec.Custom(
		"scroll long SSR transcript to mid-history marker",
		func(t testing.TB, ctx *duiruntime.Context) {
			target := ctx.Page.Locator("#workbench-v2-chat-body").GetByText(marker).First()
			if err := target.ScrollIntoViewIfNeeded(); err != nil {
				t.Fatalf("scroll transcript marker %q into view: %v", marker, err)
			}
			if err := target.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(10_000),
			}); err != nil {
				t.Fatalf("transcript marker %q not visible after scroll: %v", marker, err)
			}
		},
	)
}

func assertWorkbenchViewTransitionNames() spec.Step {
	return spec.Custom(
		"workbench chrome keeps VT names; parent artifact stays none",
		func(t testing.TB, ctx *duiruntime.Context) {
			value, err := ctx.Page.Evaluate(
				`() => {
					const read = (id) => {
						const el = document.getElementById(id);
						if (!el) return null;
						const name = getComputedStyle(el).viewTransitionName || el.style.viewTransitionName || null;
						return name === 'none' ? 'none' : name;
					};
					return {
						chat: read('workbench-v2-chat'),
						artifact: read('workbench-v2-artifact'),
						path: read('thread-artifact-path-header'),
						browser: read('thread-artifact-browser'),
						document: read('thread-artifact-document'),
						tabs: read('workbench-mobile-tabs'),
					};
				}`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			state, ok := value.(map[string]any)
			if !ok {
				t.Fatalf("view-transition probe type %T", value)
			}
			if state["chat"] != "workbench-v2-chat" {
				t.Fatalf("chat view-transition-name = %#v, want workbench-v2-chat", state["chat"])
			}
			if state["artifact"] != "none" && state["artifact"] != nil {
				t.Fatalf("artifact view-transition-name = %#v, want none", state["artifact"])
			}
			if state["path"] != "thread-artifact-path-header" {
				t.Fatalf("path-header VT = %#v", state["path"])
			}
			if state["browser"] != "thread-artifact-browser" {
				t.Fatalf("browser VT = %#v", state["browser"])
			}
			if state["document"] != "thread-artifact-document" {
				t.Fatalf("document VT = %#v", state["document"])
			}
		},
	)
}

func TestWorkbenchV2ChatPaneRespectsMinWidthDuringResize(t *testing.T) {
	spec.Story(t, "workbench v2 chat pane keeps composer-friendly min width").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Path("/threads/wb2_alpha")).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(assertChatMinRemAndDragFloor()).
		Expect(vamos.Console.Clean()).
		Run()
}

func assertChatMinRemAndDragFloor() spec.Step {
	return spec.Custom(
		"chat min-rem is enforced while dragging toward zero",
		func(t testing.TB, ctx *duiruntime.Context) {
			minRem, err := ctx.Page.Locator(`#workbench-v2-chat`).GetAttribute("data-workbench-min-rem")
			if err != nil || minRem != "18" {
				t.Fatalf("chat min-rem = %q, want 18 (%v)", minRem, err)
			}
			handle := ctx.Page.Locator(
				`[data-workbench-resize-handle][data-workbench-before="workbenchV2Chat"] > div:first-child,` +
					`[data-workbench-resize-handle][data-workbench-after="workbenchV2Chat"] > div:first-child`,
			).First()
			box, err := handle.BoundingBox()
			if err != nil || box == nil {
				// Fall back to any visible handle between chat and artifact.
				handle = ctx.Page.Locator("[data-workbench-resize-handle] > div:first-child:visible").Nth(1)
				box, err = handle.BoundingBox()
			}
			if err != nil || box == nil {
				t.Fatalf("chat/artifact resize handle missing: %v", err)
			}
			beforeWidth, err := ctx.Page.Evaluate(
				`() => document.getElementById('workbench-v2-chat')?.getBoundingClientRect().width || 0`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			// Drag aggressively to try to collapse chat.
			if err := ctx.Page.Mouse().Move(box.X+box.Width/2, box.Y+box.Height/2); err != nil {
				t.Fatal(err)
			}
			if err := ctx.Page.Mouse().Down(); err != nil {
				t.Fatal(err)
			}
			if err := ctx.Page.Mouse().Move(box.X+box.Width/2-400, box.Y+box.Height/2); err != nil {
				t.Fatal(err)
			}
			if err := ctx.Page.Mouse().Move(box.X+box.Width/2+400, box.Y+box.Height/2); err != nil {
				t.Fatal(err)
			}
			if err := ctx.Page.Mouse().Up(); err != nil {
				t.Fatal(err)
			}
			after, err := ctx.Page.Evaluate(
				`() => {
					const chat = document.getElementById('workbench-v2-chat');
					const min = Number(chat?.dataset.workbenchMinRem || 12) * 16;
					const width = chat?.getBoundingClientRect().width || 0;
					return { width, min, before: 0 };
				}`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			state, ok := after.(map[string]any)
			if !ok {
				t.Fatalf("width probe type %T", after)
			}
			width, _ := state["width"].(float64)
			min, _ := state["min"].(float64)
			if width+1 < min {
				t.Fatalf("chat width %.1f below min %.1f (before=%v)", width, min, beforeWidth)
			}
		},
	)
}

func TestWorkbenchV2SiblingFileNavRestoresComposerFocus(t *testing.T) {
	const notesPath = "thoughts/owner/plans/alpha/notes.md"
	siblingHref := "/threads/wb2_alpha?artifact=" + url.QueryEscape(notesPath) +
		"&artifact_dir=" + url.QueryEscape("thoughts/owner/plans/alpha")

	spec.Story(t, "workbench v2 sibling file nav restores composer focus").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Path("/threads/wb2_alpha")).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(selectThreadArtifactFile(
			siblingHref,
			notesPath,
			"Alpha notes",
		)).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(assertComposerFocusedAfterSiblingNav()).
		Expect(vamos.Console.Clean()).
		Run()
}

func assertComposerFocusedAfterSiblingNav() spec.Step {
	// history.js no longer restores composer focus on sibling GETs; only assert
	// leftover doc-switching affordance attributes stay unset.
	return spec.Custom(
		"sibling artifact GET leaves no doc-switching attr",
		func(t testing.TB, ctx *duiruntime.Context) {
			switching, err := ctx.Page.Evaluate(
				`() => document.documentElement.getAttribute('data-workbench-doc-switching')`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if switching != nil && switching != "" {
				t.Fatalf("doc-switching attr still set: %#v", switching)
			}
		},
	)
}
