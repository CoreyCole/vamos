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
		"workbench chat and artifact regions keep view-transition names",
		func(t testing.TB, ctx *duiruntime.Context) {
			value, err := ctx.Page.Evaluate(
				`() => {
					const read = (id) => {
						const el = document.getElementById(id);
						if (!el) return null;
						return getComputedStyle(el).viewTransitionName || el.style.viewTransitionName || null;
					};
					return {
						chat: read('workbench-v2-chat'),
						artifact: read('workbench-v2-artifact'),
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
			if state["artifact"] != "workbench-v2-artifact" {
				t.Fatalf("artifact view-transition-name = %#v, want workbench-v2-artifact", state["artifact"])
			}
		},
	)
}
