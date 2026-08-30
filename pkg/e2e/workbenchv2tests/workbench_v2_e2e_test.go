package tests

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"
	"github.com/playwright-community/playwright-go"

	"github.com/CoreyCole/vamos/pkg/e2e/fixtures"
	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

func TestWorkbenchV2_ThoughtsNavigationUsesBrowserResources(t *testing.T) {
	spec.Story(t, "workbench v2 thoughts navigation uses browser resources").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Thought("workbench-v2/")).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(assertNormalThoughtsAnchors()).
		Do(filterCurrentDirectory("root")).
		Do(assertDirectoryFilter("root", "nested")).
		Do(clearDirectoryFilter()).
		Do(followWorkbenchLink("#workbench-v2-artifact-body a[href='/thoughts/workbench-v2/nested']")).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(filterCurrentDirectory("needle")).
		Do(assertDirectoryFilter("needle", "source.go")).
		Do(modifierOpenThoughtsLink("#workbench-v2-artifact-body a[href='/thoughts/workbench-v2/nested/needle.md']")).
		Do(followWorkbenchLink("#workbench-v2-artifact-body a[href='/thoughts/workbench-v2/nested/needle.md']")).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(assertDocumentActionsOwnDocumentMenuItems()).
		Do(assertCrossDocumentViewTransitionEvidence()).
		Do(browserBack()).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(browserForward()).
		Expect(vamos.WorkbenchV2.Ready()).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestWorkbenchV2_ThreadsKeepThreadAndArtifactIdentityIndependent(t *testing.T) {
	crossFile := "thoughts/owner/plans/beta/cross-plan-artifact.md"
	spec.Story(t, "workbench v2 threads select default and cross plan artifacts").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Path("/threads")).
		Expect(vamos.WorkbenchV2.Ready()).
		Expect(spec.TextContains(vamos.WorkbenchV2.Artifact(), "Select a thread")).
		Do(toggleThreadProjectFolder("alpha", "Workbench Alpha")).
		Do(toggleWorkbenchThreadsRegion()).
		Do(followWorkbenchLink("#workbench-v2-threads-body a[href='/threads/wb2_alpha']")).
		Expect(vamos.WorkbenchV2.Ready()).
		Expect(spec.TextContains(vamos.WorkbenchV2.Artifact(), "Alpha design")).
		Visit(vamos.Pages.Path("/threads/wb2_alpha?artifact=" + url.QueryEscape(crossFile))).
		Expect(vamos.WorkbenchV2.Ready()).
		Expect(spec.TextContains(vamos.WorkbenchV2.Artifact(), "Cross-plan artifact")).
		Do(rememberWorkbenchRoot()).
		Do(assertThreadIdentity("wb2_alpha")).
		Do(assertNormalThoughtsAnchors()).
		Do(assertThreadDocumentLinksStayFullscreen()).
		Do(assertThreadArtifactCWD("thoughts/owner/plans/beta")).
		Do(toggleThreadArtifactDirectory("cross-plan-directory", "entry")).
		Do(selectThreadArtifactFile(
			"/threads/wb2_alpha?artifact=thoughts%2Fowner%2Fplans%2Fbeta%2Fcross-plan-directory%2Fentry.md&artifact_dir=thoughts%2Fowner%2Fplans%2Fbeta",
			"thoughts/owner/plans/beta/cross-plan-directory/entry.md",
			"Cross-plan directory entry",
		)).
		Expect(spec.ExpectStep(expectWorkbenchRootUnchanged())).
		Do(assertThreadIdentity("wb2_alpha")).
		Do(assertThreadArtifactCWD("thoughts/owner/plans/beta")).
		Do(assertThreadArtifactListingContains("design")).
		Do(enterThreadArtifactDirectory(
			"cross-plan-directory",
			"thoughts/owner/plans/beta/cross-plan-directory",
		)).
		Expect(spec.ExpectStep(expectWorkbenchRootUnchanged())).
		Do(expectArtifactIdentity("thoughts/owner/plans/beta/cross-plan-directory/entry.md")).
		Do(upThreadArtifactDirectory("thoughts/owner/plans/beta")).
		Expect(spec.ExpectStep(expectWorkbenchRootUnchanged())).
		Do(expectArtifactIdentity("thoughts/owner/plans/beta/cross-plan-directory/entry.md")).
		Do(browserBack()).
		Do(assertThreadArtifactCWD("thoughts/owner/plans/beta/cross-plan-directory")).
		Expect(spec.TextContains(vamos.WorkbenchV2.Artifact(), "Cross-plan directory entry")).
		Do(browserBack()).
		Do(assertThreadArtifactCWD("thoughts/owner/plans/beta")).
		Expect(spec.TextContains(vamos.WorkbenchV2.Artifact(), "Cross-plan directory entry")).
		Do(browserBack()).
		Expect(spec.TextContains(vamos.WorkbenchV2.Artifact(), "Cross-plan artifact")).
		Do(assertURLContains("artifact=thoughts%2Fowner%2Fplans%2Fbeta%2Fcross-plan-artifact.md")).
		Do(browserForward()).
		Expect(spec.TextContains(vamos.WorkbenchV2.Artifact(), "Cross-plan directory entry")).
		Do(assertThreadArtifactCWD("thoughts/owner/plans/beta")).
		Do(browserBack()).
		Expect(spec.TextContains(vamos.WorkbenchV2.Artifact(), "Cross-plan artifact")).
		Do(followWorkbenchLink("#thread-artifact-document a[href='/thoughts/workbench-v2/root.md']")).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(assertURLContains("/thoughts/workbench-v2/root.md")).
		Do(assertV2RouteVisibility("artifact")).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestWorkbenchV2_CompletedDraftsRestoreByThreadWithoutBrowserStorage(t *testing.T) {
	spec.Story(t, "workbench v2 completed drafts restore by thread without browser storage").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Path("/threads/wb2_alpha")).
		Expect(vamos.WorkbenchV2.Ready()).
		Expect(spec.InputValue(vamos.WorkbenchV2.Composer(), "completed alpha draft")).
		Do(assertNoBrowserDraftStorage()).
		Visit(vamos.Pages.Path("/threads/wb2_beta")).
		Expect(vamos.WorkbenchV2.Ready()).
		Expect(spec.InputValue(vamos.WorkbenchV2.Composer(), "completed beta draft")).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestWorkbenchV2_CommentsBindToDisplayedArtifactAndKeepRoot(t *testing.T) {
	comment := "workbench v2 cross-plan comment"
	spec.Story(t, "workbench v2 comments bind to displayed artifact").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Path("/threads/wb2_alpha?artifact=" + url.QueryEscape("thoughts/owner/plans/beta/cross-plan-artifact.md"))).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(rememberWorkbenchRoot()).
		Do(submitV2WholeDocumentComment(comment)).
		Expect(spec.ExpectStep(expectV2CommentsContain(comment))).
		Expect(spec.ExpectStep(expectWorkbenchRootUnchanged())).
		Visit(vamos.Pages.Path("/threads/wb2_alpha?artifact=" + url.QueryEscape("thoughts/owner/plans/beta/cross-plan-artifact.md"))).
		Expect(spec.ExpectStep(expectV2CommentsAttached(comment))).
		Visit(vamos.Pages.Path("/threads/wb2_alpha")).
		Expect(spec.ExpectStep(expectV2CommentsAbsent(comment))).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestWorkbenchV2_MarkdownSelectionCommentMutationsStayInV2(t *testing.T) {
	comment := "workbench v2 markdown selection"
	reply := "workbench v2 reply"
	spec.Story(t, "workbench v2 markdown selected comment mutations stay in v2").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Path("/threads/wb2_alpha?artifact=" + url.QueryEscape("thoughts/owner/plans/beta/cross-plan-artifact.md"))).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(rememberWorkbenchRoot()).
		Do(selectV2MarkdownText("Cross-plan selected text.")).
		Do(submitV2SelectedComment(comment)).
		Expect(spec.ExpectStep(expectV2InlineCommentContext(comment))).
		Do(openV2CommentInDocument(comment)).
		Expect(spec.ExpectStep(expectWorkbenchRootUnchanged())).
		Do(expandV2InlineComment()).
		Do(replyToV2Comment(comment, reply)).
		Expect(spec.ExpectStep(expectV2CommentsContain(reply))).
		Do(resolveV2Comment(comment)).
		Expect(spec.ExpectStep(expectV2ResolvedMutationStaysInV2(comment))).
		Do(openAndCancelV2Selection()).
		Expect(spec.ExpectStep(expectArtifactIdentity("thoughts/owner/plans/beta/cross-plan-artifact.md"))).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestWorkbenchV2_StaticHTMLSelectedCommentMutationsStayInV2(t *testing.T) {
	comment := "workbench v2 static selection"
	reply := "workbench v2 static reply"
	frame := "#workbench-v2-artifact-body iframe[data-vamos-html-applet]"
	spec.Story(t, "workbench v2 static html selected comment mutations stay in v2").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Do(rememberWorkbenchV2StaticHTML()).
		Visit(vamos.Pages.Thought("workbench-v2/static.html")).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(rememberWorkbenchRoot()).
		Do(assertV2OpaqueFrame()).
		Do(selectIframeText(frame, "h1")).
		Do(submitV2SelectedComment(comment)).
		Expect(spec.ExpectStep(expectV2InlineCommentContext(comment))).
		Do(openV2CommentInDocument(comment)).
		Expect(spec.ExpectStep(expectWorkbenchRootUnchanged())).
		Do(replyToV2Comment(comment, reply)).
		Expect(spec.ExpectStep(expectV2CommentsContain(reply))).
		Do(resolveV2Comment(comment)).
		Expect(spec.ExpectStep(expectV2ResolvedMutationStaysInV2(comment))).
		Do(selectIframeText(frame, "h1")).
		Do(openAndCancelV2Selection()).
		Expect(spec.ExpectStep(expectWorkbenchV2StaticHTMLUnchanged())).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestWorkbenchV2_StaticHTMLCommentsPreserveOpaqueBridgeAndSource(t *testing.T) {
	comment := "workbench v2 opaque comment"
	spec.Story(t, "workbench v2 static html comments preserve opaque bridge").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Do(rememberWorkbenchV2StaticHTML()).
		Visit(vamos.Pages.Thought("workbench-v2/static.html")).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(assertV2OpaqueFrame()).
		Do(submitV2WholeDocumentComment(comment)).
		Expect(spec.ExpectStep(expectV2CommentsContain(comment))).
		Expect(spec.ExpectStep(expectWorkbenchV2StaticHTMLUnchanged())).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestWorkbenchV2_RoutesAndUnsafeArtifactsHaveSafeOutcomes(t *testing.T) {
	spec.Story(t, "workbench v2 routes and unsafe artifacts have safe outcomes").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Root()).
		Expect(vamos.WorkbenchV2.Ready()).
		Do(expectRouteStatus("/thoughts/", 200)).
		Do(expectRouteStatus("/thoughts/workbench-v2/nested/", 200)).
		Do(expectRouteStatus("/thoughts/workbench-v2/nested/source.go", 200)).
		Do(expectRouteStatus("/thoughts/workbench-v2/nested/data.csv", 200)).
		Do(expectRouteStatus("/thoughts/workbench-v2/static.html", 200)).
		Do(expectRouteStatus("/threads", 200)).
		Do(expectRouteStatus("/threads/wb2_alpha", 200)).
		Do(expectRouteStatus("/threads/not-a-thread", 404)).
		Do(expectSafeArtifactFailure("/thoughts/workbench-v2/missing.md")).
		Do(expectEncodedThoughtsRejection("/thoughts/%2e%2e/thoughts-sibling/secret.md")).
		Do(expectEncodedThoughtsRejection("/thoughts/workbench-v2-sibling/root.md")).
		Do(expectEncodedThoughtsRejection("/thoughts/workbench-v2/symlink-escape/secret.md")).
		Run()
}

func TestWorkbenchV2_RouteDefaultsAndRatiosAcrossViewports(t *testing.T) {
	for _, viewport := range []duiruntime.ViewportClass{duiruntime.ViewportDesktopFull, duiruntime.ViewportDesktopHalf, duiruntime.ViewportMobile} {
		spec.Story(t, "workbench v2 route defaults and ratios across viewports").
			App(vamos.App()).
			Viewport(viewport).
			As(vamos.Robot).With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
			Do(configureViewportClass(viewport)).
			Visit(vamos.Pages.Thought("workbench-v2/root.md")).
			Expect(vamos.WorkbenchV2.Ready()).
			Do(assertV2RouteVisibility("artifact")).
			Visit(vamos.Pages.Path("/threads")).Expect(vamos.WorkbenchV2.Ready()).
			Do(assertV2RouteVisibility("index")).
			Visit(vamos.Pages.Path("/threads/wb2_alpha")).
			Expect(vamos.WorkbenchV2.Ready()).
			Do(assertV2RouteVisibility("thread")).
			Do(dragAndRememberDesktopRatio()).
			Do(reloadPage()).
			Do(assertDesktopRatioRestored()).
			Do(resizeWindowWithoutClipping()).
			Do(mutateVisibilityThenNavigateRouteDefaults()).
			Do(assertMobileTabBehavior()).
			Expect(vamos.Console.Clean()).Run()
	}
}

func configureViewportClass(viewport duiruntime.ViewportClass) spec.Step {
	return spec.Custom(
		"configure server viewport class",
		func(t testing.TB, ctx *duiruntime.Context) {
			if err := ctx.Page.SetExtraHTTPHeaders(map[string]string{
				"X-Vamos-Viewport-Class": string(viewport),
			}); err != nil {
				t.Fatal(err)
			}
		},
	)
}

func dragAndRememberDesktopRatio() spec.Step {
	return spec.Custom(
		"drag and persist desktop ratio",
		func(t testing.TB, ctx *duiruntime.Context) {
			viewport, err := ctx.Page.Locator("#workbench-root").
				GetAttribute("data-workbench-viewport-class")
			if err != nil {
				t.Fatal(err)
			}
			if viewport == "mobile" {
				return
			}
			handle := ctx.Page.Locator("[data-workbench-resize-handle] > div:first-child:visible").
				First()
			box, err := handle.BoundingBox()
			if err != nil || box == nil {
				t.Fatalf("resize handle missing: %v", err)
			}
			before, err := ctx.Page.Evaluate(
				`() => [...document.querySelectorAll('[data-workbench-region][data-workbench-visible="true"]')].map(el => Number(el.dataset.workbenchRatio))`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			response, err := ctx.Page.ExpectResponse(
				"**/api/layout-preferences",
				func() error {
					if err := ctx.Page.Mouse().
						Move(box.X+box.Width/2, box.Y+box.Height/2); err != nil {
						return err
					}
					if err := ctx.Page.Mouse().Down(); err != nil {
						return err
					}
					if err := ctx.Page.Mouse().
						Move(box.X+box.Width/2+24, box.Y+box.Height/2); err != nil {
						return err
					}
					return ctx.Page.Mouse().Up()
				},
			)
			if err != nil {
				t.Fatalf("layout save response not observed: %v", err)
			}
			if response.Status() < 200 || response.Status() >= 300 {
				t.Fatalf("layout save status = %d", response.Status())
			}
			after, err := ctx.Page.Evaluate(
				`() => [...document.querySelectorAll('[data-workbench-region][data-workbench-visible="true"]')].map(el => Number(el.dataset.workbenchRatio))`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			ctx.Memory["workbench-v2.ratio-before"] = fmt.Sprint(before)
			ctx.Memory["workbench-v2.ratio-after"] = fmt.Sprint(after)
			if ctx.Memory["workbench-v2.ratio-before"] == ctx.Memory["workbench-v2.ratio-after"] {
				t.Fatal("resize did not change ratios")
			}
		},
	)
}

func assertDesktopRatioRestored() spec.Step {
	return spec.Custom(
		"desktop ratio persists after reload",
		func(t testing.TB, ctx *duiruntime.Context) {
			viewport, err := ctx.Page.Locator("#workbench-root").
				GetAttribute("data-workbench-viewport-class")
			if err != nil {
				t.Fatal(err)
			}
			if viewport == "mobile" {
				return
			}
			if ctx.Memory["workbench-v2.ratio-after"] == "" {
				t.Fatal("desktop resize did not record a persisted ratio")
			}
			value, err := ctx.Page.Evaluate(
				`() => [...document.querySelectorAll('[data-workbench-region][data-workbench-visible="true"]')].map(el => Number(el.dataset.workbenchRatio))`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(value) != ctx.Memory["workbench-v2.ratio-after"] {
				t.Fatalf(
					"ratio after reload = %v, want %s",
					value,
					ctx.Memory["workbench-v2.ratio-after"],
				)
			}
		},
	)
}

func resizeWindowWithoutClipping() spec.Step {
	return spec.Custom(
		"window resize reflows regions without clipping",
		func(t testing.TB, ctx *duiruntime.Context) {
			viewport, err := ctx.Page.Locator("#workbench-root").
				GetAttribute("data-workbench-viewport-class")
			if err != nil || viewport == "mobile" {
				return
			}
			original := ctx.Page.ViewportSize()
			if original == nil {
				t.Fatal("browser viewport size is unavailable")
			}
			beforeRatios, err := ctx.Page.Evaluate(
				`() => [...document.querySelectorAll('[data-workbench-region]')].map(el => el.dataset.workbenchRatio)`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := ctx.Page.SetViewportSize(820, original.Height); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := ctx.Page.SetViewportSize(
					original.Width,
					original.Height,
				); err != nil {
					t.Errorf("restore viewport: %v", err)
				}
			}()
			if _, err := ctx.Page.Evaluate(
				`() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))`,
				nil,
			); err != nil {
				t.Fatal(err)
			}
			layout, err := ctx.Page.Evaluate(`() => {
				const container = document.getElementById('workbench-regions').getBoundingClientRect()
				const regions = [...document.querySelectorAll('[data-workbench-region]')].filter(el => getComputedStyle(el).display !== 'none').map(el => {
					const rect = el.getBoundingClientRect()
					return {id: el.id, left: rect.left, right: rect.right, width: rect.width}
				})
				return {container: {left: container.left, right: container.right}, regions}
			}`, nil)
			if err != nil {
				t.Fatal(err)
			}
			data := layout.(map[string]any)
			container := data["container"].(map[string]any)
			left := evaluatedFloat64(t, container["left"])
			right := evaluatedFloat64(t, container["right"])
			artifactWidth := float64(0)
			for _, raw := range data["regions"].([]any) {
				region := raw.(map[string]any)
				if evaluatedFloat64(t, region["left"]) < left-1 ||
					evaluatedFloat64(t, region["right"]) > right+1 {
					t.Fatalf(
						"region clips after window resize: %#v within %#v",
						region,
						container,
					)
				}
				if region["id"] == "workbench-v2-artifact" {
					artifactWidth = evaluatedFloat64(t, region["width"])
				}
			}
			if artifactWidth <= 0 {
				t.Fatalf("artifact width after window resize = %v", artifactWidth)
			}
			afterRatios, err := ctx.Page.Evaluate(
				`() => [...document.querySelectorAll('[data-workbench-region]')].map(el => el.dataset.workbenchRatio)`,
				nil,
			)
			if err != nil || fmt.Sprint(afterRatios) != fmt.Sprint(beforeRatios) {
				t.Fatalf(
					"window resize changed ratios: %v -> %v (%v)",
					beforeRatios,
					afterRatios,
					err,
				)
			}
		},
	)
}

func evaluatedFloat64(t testing.TB, value any) float64 {
	switch value := value.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int8:
		return float64(value)
	case int16:
		return float64(value)
	case int32:
		return float64(value)
	case int64:
		return float64(value)
	case uint:
		return float64(value)
	case uint8:
		return float64(value)
	case uint16:
		return float64(value)
	case uint32:
		return float64(value)
	case uint64:
		return float64(value)
	default:
		t.Fatalf("evaluated value is not numeric: %T (%v)", value, value)
		return 0
	}
}

func mutateVisibilityThenNavigateRouteDefaults() spec.Step {
	return spec.Custom(
		"route hydration overrides temporary visibility",
		func(t testing.TB, ctx *duiruntime.Context) {
			viewport, err := ctx.Page.Locator("#workbench-root").
				GetAttribute("data-workbench-viewport-class")
			if err != nil {
				t.Fatal(err)
			}
			if viewport == "mobile" {
				return
			}
			response, err := ctx.Page.ExpectResponse(
				"**/api/layout-preferences",
				func() error {
					_, err := ctx.Page.Evaluate(`async () => {
					const { mergePaths } = await import('@vamos/datastar')
					mergePaths([['workbench.regions.workbenchV2Comments.visible', true]])
					document.getElementById('workbench-root').dispatchEvent(new CustomEvent('workbench-layout-save', {bubbles: true}))
				}`, nil)
					return err
				},
			)
			if err != nil {
				t.Fatalf("visibility save response not observed: %v", err)
			}
			if response.Status() < 200 || response.Status() >= 300 {
				t.Fatalf("visibility save status = %d", response.Status())
			}
			visible, err := ctx.Page.Locator("#workbench-v2-comments").
				GetAttribute("data-workbench-visible")
			if err != nil || visible != "true" {
				t.Fatalf("comments did not become visible: %q %v", visible, err)
			}
			if _, err := ctx.Page.Goto(
				ctx.Config.BaseURL+"/thoughts/workbench-v2/root.md",
				playwright.PageGotoOptions{
					WaitUntil: playwright.WaitUntilStateDomcontentloaded,
				},
			); err != nil {
				t.Fatal(err)
			}
			if _, err := ctx.Page.Goto(
				ctx.Config.BaseURL+"/threads/wb2_alpha",
				playwright.PageGotoOptions{
					WaitUntil: playwright.WaitUntilStateDomcontentloaded,
				},
			); err != nil {
				t.Fatal(err)
			}
			visible, err = ctx.Page.Locator("#workbench-v2-comments").
				GetAttribute("data-workbench-visible")
			if err != nil || visible != "false" {
				t.Fatalf("route default did not close comments: %q %v", visible, err)
			}
		},
	)
}

func assertV2RouteVisibility(route string) spec.Step {
	return spec.Custom(
		"v2 route default visibility",
		func(t testing.TB, ctx *duiruntime.Context) {
			value, err := ctx.Page.Evaluate(
				`() => [...document.querySelectorAll('[data-workbench-region]')].map(el => ({id: el.id, visible: el.getAttribute('data-workbench-visible') === 'true'}))`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			regions := value.([]any)
			if len(regions) != 4 {
				t.Fatalf("regions = %#v", regions)
			}
			visible := map[string]bool{}
			for _, raw := range regions {
				row := raw.(map[string]any)
				visible[row["id"].(string)] = row["visible"].(bool)
			}
			want := map[string]bool{}
			switch route {
			case "artifact":
				want = map[string]bool{"workbench-v2-artifact": true}
			case "index":
				want = map[string]bool{
					"workbench-v2-threads":  true,
					"workbench-v2-artifact": true,
				}
			case "thread":
				want = map[string]bool{
					"workbench-v2-threads":  true,
					"workbench-v2-chat":     true,
					"workbench-v2-artifact": true,
				}
			}
			for id, got := range visible {
				if got != want[id] {
					t.Fatalf("%s visibility %s = %v, want %v", route, id, got, want[id])
				}
			}
		},
	)
}

func assertMobileTabBehavior() spec.Step {
	return spec.Custom(
		"mobile tabs select one usable region",
		func(t testing.TB, ctx *duiruntime.Context) {
			viewport, err := ctx.Page.Locator("#workbench-root").
				GetAttribute("data-workbench-viewport-class")
			if err != nil {
				t.Fatal(err)
			}
			if viewport != "mobile" {
				return
			}
			if err := ctx.Page.GetByRole(*playwright.AriaRoleTab, playwright.PageGetByRoleOptions{Name: "Docs"}).
				Click(); err != nil {
				t.Fatal(err)
			}
			active, err := ctx.Page.Locator("#workbench-root").
				GetAttribute("data-workbench-mobile-active")
			if err != nil || active != "workbenchV2Artifact" {
				t.Fatalf("mobile active region = %q %v", active, err)
			}
			visible, err := ctx.Page.Locator("[data-workbench-region]:visible").Count()
			if err != nil || visible != 1 {
				t.Fatalf("mobile visible region count = %d %v", visible, err)
			}
		},
	)
}

func assertNormalThoughtsAnchors() spec.Step {
	return spec.Custom(
		"artifact links are normal thoughts anchors",
		func(t testing.TB, ctx *duiruntime.Context) {
			t.Helper()
			value, err := ctx.Page.Evaluate(
				`() => [...document.querySelectorAll('#workbench-v2-artifact-body a[href^="/thoughts/"]')].map(a => ({href: a.getAttribute('href'), action: a.getAttribute('data-on:click') || a.getAttribute('data-on-click') || ''}))`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			links := value.([]any)
			if len(links) == 0 {
				t.Fatal("expected at least one thoughts anchor")
			}
			for _, raw := range links {
				link := raw.(map[string]any)
				if strings.TrimSpace(link["href"].(string)) == "" ||
					link["action"].(string) != "" {
					t.Fatalf("artifact link is not native navigation: %#v", link)
				}
			}
		},
	)
}

func filterCurrentDirectory(query string) spec.Step {
	return spec.Custom(
		"filter current directory",
		func(t testing.TB, ctx *duiruntime.Context) {
			t.Helper()
			search := ctx.Page.Locator("#workbench-v2-artifact-body input[type='search']").
				First()
			if err := search.Fill(query); err != nil {
				t.Fatal(err)
			}
			visible, err := ctx.Page.Locator("#workbench-v2-artifact-body a:visible").
				AllTextContents()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(visible, " "), query) {
				t.Fatalf("directory filter did not retain %q: %v", query, visible)
			}
		},
	)
}

func assertDirectoryFilter(match, hidden string) spec.Step {
	return spec.Custom(
		"directory filter only changes rendered rows",
		func(t testing.TB, ctx *duiruntime.Context) {
			visible, err := ctx.Page.Locator("#workbench-v2-artifact-body a:visible").
				AllTextContents()
			if err != nil {
				t.Fatal(err)
			}
			text := strings.Join(visible, " ")
			if !strings.Contains(text, match) || strings.Contains(text, hidden) {
				t.Fatalf("directory filter visible rows = %q", text)
			}
		},
	)
}

func clearDirectoryFilter() spec.Step {
	return spec.Custom(
		"clear directory filter",
		func(t testing.TB, ctx *duiruntime.Context) {
			if err := ctx.Page.Locator("#workbench-v2-artifact-body input[type='search']").
				First().
				Fill(""); err != nil {
				t.Fatal(err)
			}
		},
	)
}

func toggleWorkbenchThreadsRegion() spec.Step {
	return spec.Custom(
		"threads sidebar hides and reopens without navigation",
		func(t testing.TB, ctx *duiruntime.Context) {
			before := ctx.Page.URL()
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
			reopen := ctx.Page.Locator("button[data-workbench-threads-reopen]")
			if err := reopen.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(10_000),
			}); err != nil {
				t.Fatalf("threads reopen control did not appear: %v", err)
			}
			if err := reopen.Click(); err != nil {
				t.Fatal(err)
			}
			if err := region.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(10_000),
			}); err != nil {
				t.Fatalf("threads sidebar did not reopen: %v", err)
			}
			if ctx.Page.URL() != before {
				t.Fatalf("threads sidebar toggle navigated: %s", ctx.Page.URL())
			}
		},
	)
}

func toggleThreadProjectFolder(project, thread string) spec.Step {
	return spec.Custom(
		"thread project folder toggles without navigation",
		func(t testing.TB, ctx *duiruntime.Context) {
			folder := ctx.Page.Locator("details[data-thread-project-folder]").
				Filter(playwright.LocatorFilterOptions{HasText: project}).
				First()
			summary := folder.Locator("summary").First()
			link := folder.GetByText(thread, playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).
				First()
			open, err := folder.Evaluate("el => el.open", nil)
			if err != nil || open != true {
				t.Fatalf("project folder is not initially open: %v %v", open, err)
			}
			before := ctx.Page.URL()
			if err := summary.Focus(); err != nil {
				t.Fatal(err)
			}
			if err := summary.Press("Enter"); err != nil {
				t.Fatal(err)
			}
			open, err = folder.Evaluate("el => el.open", nil)
			if err != nil || open != false {
				t.Fatalf("project folder did not close: %v %v", open, err)
			}
			visible, err := link.IsVisible()
			if err != nil || visible {
				t.Fatalf("collapsed project thread remains visible: %v %v", visible, err)
			}
			if ctx.Page.URL() != before {
				t.Fatalf("project folder toggle navigated: %s", ctx.Page.URL())
			}
			search := ctx.Page.Locator("#workbench-thread-search")
			if err := search.Fill(thread); err != nil {
				t.Fatal(err)
			}
			open, err = folder.Evaluate("el => el.open", nil)
			if err != nil || open != true {
				t.Fatalf("search did not open matching project folder: %v %v", open, err)
			}
			visible, err = link.IsVisible()
			if err != nil || !visible {
				t.Fatalf("searched project thread is hidden: %v %v", visible, err)
			}
			if err := search.Fill(""); err != nil {
				t.Fatal(err)
			}
		},
	)
}

func followWorkbenchLink(selector string) spec.Step {
	return spec.Custom(
		"follow workbench anchor",
		func(t testing.TB, ctx *duiruntime.Context) {
			link := ctx.Page.Locator(selector).First()
			href, err := link.GetAttribute("href")
			if err != nil || href == "" {
				t.Fatalf("workbench link href: %q %v", href, err)
			}
			_, err = ctx.Page.ExpectNavigation(
				func() error { return link.Click() },
				playwright.PageExpectNavigationOptions{
					WaitUntil: playwright.WaitUntilStateDomcontentloaded,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(
				ctx.Page.URL(),
				strings.TrimRight(ctx.Config.BaseURL, "/")+href,
			) {
				t.Fatalf("navigation URL = %s, want %s", ctx.Page.URL(), href)
			}
		},
	)
}

func modifierOpenThoughtsLink(selector string) spec.Step {
	return spec.Custom(
		"Thoughts anchor opens its resource in a new tab",
		func(t testing.TB, ctx *duiruntime.Context) {
			href, err := ctx.Page.Locator(selector).First().GetAttribute("href")
			if err != nil || href == "" {
				t.Fatalf("new-tab link href = %q %v", href, err)
			}
			browserContext, err := ctx.Browser.NewContext()
			if err != nil {
				t.Fatal(err)
			}
			defer browserContext.Close()
			tab, err := browserContext.NewPage()
			if err != nil {
				t.Fatal(err)
			}
			if err := vamos.Authenticate(
				t.Context(),
				tab,
				ctx.Config,
				"playwright@localhost",
			); err != nil {
				t.Fatal(err)
			}
			if _, err := tab.Goto(
				strings.TrimRight(ctx.Config.BaseURL, "/")+href,
				playwright.PageGotoOptions{
					WaitUntil: playwright.WaitUntilStateDomcontentloaded,
				},
			); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(tab.URL(), "/thoughts/") ||
				strings.Contains(tab.URL(), "context=") {
				t.Fatalf("new-tab URL = %s", tab.URL())
			}
			if count, err := tab.Locator("#workbench-root").
				Count(); err != nil ||
				count != 1 {
				t.Fatalf("new tab lacks v2 workbench: %d %v", count, err)
			}
		},
	)
}

func assertDocumentActionsOwnDocumentMenuItems() spec.Step {
	return spec.Custom(
		"document actions own copy and comment controls",
		func(t testing.TB, ctx *duiruntime.Context) {
			actions, err := ctx.Page.Locator("[data-testid='workbench-overflow-actions']").
				First().
				TextContent()
			if err != nil || !strings.Contains(actions, "Copy document") ||
				!strings.Contains(actions, "Comment") {
				t.Fatalf("document actions = %q %v", actions, err)
			}
			avatar, err := ctx.Page.Locator("#user_profile-content").TextContent()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(avatar, "Copy document") ||
				strings.Contains(avatar, "Discussions") {
				t.Fatalf("avatar menu retains document actions: %q", avatar)
			}
		},
	)
}

func assertCrossDocumentViewTransitionEvidence() spec.Step {
	return spec.Custom(
		"cross-document View Transition evidence is present",
		func(t testing.TB, ctx *duiruntime.Context) {
			value, err := ctx.Page.Evaluate(
				`() => ({meta: Boolean(document.querySelector('meta[name="view-transition"][content="same-origin"]')), navigation: performance.getEntriesByType('navigation').some(e => e.type === 'navigate')})`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			data := value.(map[string]any)
			if data["meta"] != true || data["navigation"] != true {
				t.Fatalf("missing cross-document transition evidence: %#v", data)
			}
		},
	)
}

func browserBack() spec.Step {
	return spec.Custom("browser back", func(t testing.TB, ctx *duiruntime.Context) {
		if _, err := ctx.Page.GoBack(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			count, err := ctx.Page.Locator("#workbench-root").Count()
			if err == nil && count == 1 {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("workbench did not finish history restoration")
	})
}

func browserForward() spec.Step {
	return spec.Custom("browser forward", func(t testing.TB, ctx *duiruntime.Context) {
		if _, err := ctx.Page.GoForward(); err != nil {
			t.Fatal(err)
		}
	})
}

func assertThreadDocumentLinksStayFullscreen() spec.Step {
	return spec.Custom(
		"rendered document links keep fullscreen Thoughts navigation",
		func(t testing.TB, ctx *duiruntime.Context) {
			link := ctx.Page.Locator(
				"#thread-artifact-document a[href='/thoughts/workbench-v2/root.md']",
			).First()
			count, err := link.Count()
			if err != nil || count != 1 {
				t.Fatalf("fullscreen document link count = %d %v", count, err)
			}
			action, err := link.GetAttribute("data-on:click")
			if err != nil || action != "" {
				t.Fatalf("document link was intercepted: %q %v", action, err)
			}
		},
	)
}

func toggleThreadArtifactDirectory(directory, child string) spec.Step {
	return spec.Custom(
		"thread artifact directory expands without navigation",
		func(t testing.TB, ctx *duiruntime.Context) {
			before := ctx.Page.URL()
			beforeCWD, err := ctx.Page.Locator("[data-thread-artifact-cwd]").
				GetAttribute("data-thread-artifact-cwd")
			if err != nil {
				t.Fatal(err)
			}
			details := ctx.Page.Locator("[data-thread-artifact-browser] details").
				Filter(playwright.LocatorFilterOptions{HasText: directory + "/"}).
				First()
			summary := details.Locator("summary")
			if err := summary.Focus(); err != nil {
				t.Fatal(err)
			}
			response, err := ctx.Page.ExpectResponse(
				"**/threads/wb2_alpha/artifact-directory?*",
				func() error { return summary.Press("Enter") },
			)
			if err != nil {
				t.Fatalf("directory load response not observed: %v", err)
			}
			if response.Status() != 200 {
				t.Fatalf("directory load status = %d", response.Status())
			}
			if ctx.Page.URL() != before {
				t.Fatalf("directory toggle navigated: %s", ctx.Page.URL())
			}
			afterCWD, err := ctx.Page.Locator("[data-thread-artifact-cwd]").
				GetAttribute("data-thread-artifact-cwd")
			if err != nil || afterCWD != beforeCWD {
				t.Fatalf(
					"directory toggle changed browser CWD: %q -> %q (%v)",
					beforeCWD,
					afterCWD,
					err,
				)
			}
			childLink := details.Locator("a[data-thread-artifact-file]").
				Filter(playwright.LocatorFilterOptions{HasText: child}).
				First()
			if err := childLink.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(30_000),
			}); err != nil {
				t.Fatalf("directory child did not appear: %v", err)
			}
		},
	)
}

func assertThreadArtifactCWD(want string) spec.Step {
	return spec.Custom(
		"thread artifact browser keeps expected CWD",
		func(t testing.TB, ctx *duiruntime.Context) {
			cwd, err := ctx.Page.Locator("[data-thread-artifact-cwd]").
				GetAttribute("data-thread-artifact-cwd")
			if err != nil || cwd != want {
				t.Fatalf("artifact browser CWD = %q, want %q (%v)", cwd, want, err)
			}
		},
	)
}

func assertThreadArtifactListingContains(name string) spec.Step {
	return spec.Custom(
		"thread artifact browser retains parent listing",
		func(t testing.TB, ctx *duiruntime.Context) {
			link := ctx.Page.Locator("[data-thread-artifact-browser] a[data-thread-artifact-file]").
				Filter(playwright.LocatorFilterOptions{HasText: name}).
				First()
			visible, err := link.IsVisible()
			if err != nil || !visible {
				t.Fatalf("artifact browser listing missing %q: %v %v", name, visible, err)
			}
		},
	)
}

func enterThreadArtifactDirectory(directory, wantCWD string) spec.Step {
	return navigateThreadArtifactBrowser(
		"enter thread artifact directory",
		"[data-thread-artifact-browser] li:has([data-thread-artifact-toggle]) a[data-thread-artifact-enter][aria-label='Open "+directory+" folder']",
		wantCWD,
	)
}

func upThreadArtifactDirectory(wantCWD string) spec.Step {
	return navigateThreadArtifactBrowser(
		"open parent thread artifact directory",
		"[data-thread-artifact-browser] a[data-thread-artifact-up]",
		wantCWD,
	)
}

func navigateThreadArtifactBrowser(name, selector, wantCWD string) spec.Step {
	return spec.Custom(name, func(t testing.TB, ctx *duiruntime.Context) {
		link := ctx.Page.Locator(selector).First()
		response, err := ctx.Page.ExpectResponse(
			"**/threads/wb2_alpha/artifact-browser?*",
			func() error { return link.Click() },
		)
		if err != nil {
			t.Fatalf("artifact browser response not observed: %v", err)
		}
		if response.Status() != http.StatusOK {
			t.Fatalf("artifact browser response status = %d", response.Status())
		}
		cwd := ctx.Page.Locator("[data-thread-artifact-cwd='" + wantCWD + "']")
		if err := cwd.WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(30_000),
		}); err != nil {
			t.Fatalf("artifact browser CWD did not become %q: %v", wantCWD, err)
		}
	})
}

func selectThreadArtifactFile(href, identity, text string) spec.Step {
	return spec.Custom(
		"thread artifact file patches in place",
		func(t testing.TB, ctx *duiruntime.Context) {
			link := ctx.Page.Locator(
				"[data-thread-artifact-browser] a[data-thread-artifact-file][href='" + href + "']",
			).First()
			response, err := ctx.Page.ExpectResponse(
				"**/threads/wb2_alpha/artifact?*",
				func() error { return link.Click() },
			)
			if err != nil {
				t.Fatalf("artifact response not observed: %v", err)
			}
			if response.Status() != 200 {
				t.Fatalf("artifact response status = %d", response.Status())
			}
			if err := ctx.Page.Locator("#thread-artifact-document").
				GetByText(text).
				First().
				WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(30_000),
				}); err != nil {
				t.Fatalf("patched artifact missing %q: %v", text, err)
			}
			if !strings.Contains(ctx.Page.URL(), href) {
				t.Fatalf("artifact URL = %s, want %s", ctx.Page.URL(), href)
			}
			comments, err := ctx.Page.Locator("#workbench-v2-comments-body").TextContent()
			if err != nil || !strings.Contains(comments, identity) {
				t.Fatalf("comments identity = %q, want %q: %v", comments, identity, err)
			}
			fields, err := ctx.Page.Evaluate(
				`identity => { const values = [...document.querySelectorAll('#thread-artifact-document input[name="doc_path"]')].map(el => el.value); return {count: values.length, correct: values.every(value => value === identity)} }`,
				identity,
			)
			if err != nil {
				t.Fatal(err)
			}
			state, ok := fields.(map[string]any)
			if !ok {
				t.Fatalf("artifact form identity has type %T", fields)
			}
			count, ok := state["count"].(int)
			if !ok || count == 0 || state["correct"] != true {
				t.Fatalf("artifact form identity = %#v, want %q", state, identity)
			}
		},
	)
}

func assertURLContains(want string) spec.Step {
	return spec.Custom(
		"URL matches expected resource",
		func(t testing.TB, ctx *duiruntime.Context) {
			if !strings.Contains(ctx.Page.URL(), want) {
				t.Fatalf("URL = %s, want %s", ctx.Page.URL(), want)
			}
		},
	)
}

func assertThreadIdentity(threadID string) spec.Step {
	return spec.Custom(
		"thread identity remains selected",
		func(t testing.TB, ctx *duiruntime.Context) {
			if !strings.Contains(ctx.Page.URL(), "/threads/"+threadID) {
				t.Fatalf("selected thread changed: %s", ctx.Page.URL())
			}
		},
	)
}

func selectV2MarkdownText(text string) spec.Step {
	return spec.Custom(
		"select v2 markdown text",
		func(t testing.TB, ctx *duiruntime.Context) {
			selector := "#workbench-v2-artifact-body p"
			_, err := ctx.Page.Evaluate(`([selector, text]) => {
			const element = [...document.querySelectorAll(selector)].find(el => el.textContent.includes(text))
			if (!element) throw new Error('markdown selection target missing')
			const range = document.createRange(); range.selectNodeContents(element)
			const selection = window.getSelection(); selection.removeAllRanges(); selection.addRange(range)
			element.dispatchEvent(new MouseEvent('mouseup', {bubbles: true}))
		}`, []string{selector, text})
			if err != nil {
				t.Fatal(err)
			}
		},
	)
}

func submitV2SelectedComment(text string) spec.Step {
	return spec.Custom(
		"submit v2 selected-text comment",
		func(t testing.TB, ctx *duiruntime.Context) {
			trigger := ctx.Page.Locator("#workbench-v2-artifact-body form.commentui-selection-trigger").
				First()
			if err := trigger.GetByRole(*playwright.AriaRoleButton, playwright.LocatorGetByRoleOptions{Name: "Comment on selection"}).
				Click(); err != nil {
				t.Fatal(err)
			}
			form := ctx.Page.Locator("#workbench-v2-artifact-body form:has(textarea[name='comment_text'])").
				First()
			if err := form.Locator("textarea[name='comment_text']").
				Fill(text); err != nil {
				t.Fatal(err)
			}
			if err := form.GetByRole(*playwright.AriaRoleButton, playwright.LocatorGetByRoleOptions{Name: "Comment"}).
				First().
				Click(); err != nil {
				t.Fatal(err)
			}
		},
	)
}

func expectV2InlineCommentContext(text string) spec.Step {
	return spec.Custom(
		"v2 inline context marker and comment card",
		func(t testing.TB, ctx *duiruntime.Context) {
			marker := ctx.Page.Locator("#workbench-v2-artifact-body [aria-label^='Open'][aria-label*='comments in context pane']").
				First()
			if err := marker.WaitFor(
				playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(30_000),
				},
			); err != nil {
				t.Fatal(err)
			}
			card := ctx.Page.Locator("#workbench-v2-comments-body article").
				GetByText(text, playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).
				First()
			if err := card.WaitFor(
				playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(30_000),
				},
			); err != nil {
				t.Fatal(err)
			}
		},
	)
}

func openV2CommentInDocument(comment string) spec.Step {
	return spec.Custom(
		"open v2 comment in document",
		func(t testing.TB, ctx *duiruntime.Context) {
			card := ctx.Page.Locator("#workbench-v2-comments-body article").
				Filter(playwright.LocatorFilterOptions{HasText: comment}).
				First()
			if err := card.GetByRole(*playwright.AriaRoleButton, playwright.LocatorGetByRoleOptions{Name: "Open comment in document"}).
				Click(); err != nil {
				t.Fatal(err)
			}
		},
	)
}

func expandV2InlineComment() spec.Step {
	return spec.Custom(
		"open v2 inline comment context",
		func(t testing.TB, ctx *duiruntime.Context) {
			button := ctx.Page.Locator("#workbench-v2-artifact-body [aria-label^='Open'][aria-label*='comments in context pane']").
				First()
			if err := button.Click(); err != nil {
				t.Fatal(err)
			}
		},
	)
}

func replyToV2Comment(comment, reply string) spec.Step {
	return spec.Custom(
		"reply to v2 comment",
		func(t testing.TB, ctx *duiruntime.Context) {
			card := ctx.Page.Locator("#workbench-v2-comments-body article").
				Filter(playwright.LocatorFilterOptions{HasText: comment}).
				First()
			form := card.Locator("form:has(textarea[name='reply_text'])").First()
			if err := form.Locator("textarea[name='reply_text']").
				Fill(reply); err != nil {
				t.Fatal(err)
			}
			if err := form.GetByRole(*playwright.AriaRoleButton, playwright.LocatorGetByRoleOptions{Name: "Reply"}).
				Click(); err != nil {
				t.Fatal(err)
			}
		},
	)
}

func resolveV2Comment(comment string) spec.Step {
	return spec.Custom("resolve v2 comment", func(t testing.TB, ctx *duiruntime.Context) {
		card := ctx.Page.Locator("#workbench-v2-comments-body article").
			Filter(playwright.LocatorFilterOptions{HasText: comment}).
			First()
		if err := card.GetByRole(*playwright.AriaRoleButton, playwright.LocatorGetByRoleOptions{Name: "Resolve"}).
			Click(); err != nil {
			t.Fatal(err)
		}
	})
}

func expectV2ResolvedMutationStaysInV2(comment string) spec.Step {
	return spec.Custom(
		"resolved comment leaves only v2 controls",
		func(t testing.TB, ctx *duiruntime.Context) {
			card := ctx.Page.Locator("#workbench-v2-comments-body article").
				Filter(playwright.LocatorFilterOptions{HasText: comment}).
				First()
			if err := card.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateDetached,
				Timeout: playwright.Float(30_000),
			}); err != nil {
				t.Fatalf("resolved comment remains active: %v", err)
			}
			legacy, err := ctx.Page.Locator("[id^='doc-right-']").Count()
			if err != nil || legacy != 0 {
				t.Fatalf("resolved mutation restored legacy controls: %d %v", legacy, err)
			}
			v2Fields, err := ctx.Page.Locator("#workbench-root input[name='workbench_v2'][value='1']").
				Count()
			if err != nil || v2Fields == 0 {
				t.Fatalf("resolved mutation lost v2 form identity: %d %v", v2Fields, err)
			}
		},
	)
}

func openAndCancelV2Selection() spec.Step {
	return spec.Custom(
		"open and cancel v2 selection comment",
		func(t testing.TB, ctx *duiruntime.Context) {
			trigger := ctx.Page.Locator("#workbench-v2-artifact-body form.commentui-selection-trigger").
				First()
			if err := trigger.GetByRole(*playwright.AriaRoleButton, playwright.LocatorGetByRoleOptions{Name: "Comment on selection"}).
				Click(); err != nil {
				t.Fatal(err)
			}
			form := ctx.Page.Locator("#workbench-v2-artifact-body form:has(textarea[name='comment_text'])").
				First()
			if err := form.GetByRole(*playwright.AriaRoleButton, playwright.LocatorGetByRoleOptions{Name: "Cancel"}).
				Click(); err != nil {
				t.Fatal(err)
			}
		},
	)
}

func expectArtifactIdentity(want string) spec.Step {
	return spec.Custom(
		"displayed artifact identity remains unchanged",
		func(t testing.TB, ctx *duiruntime.Context) {
			value, err := ctx.Page.Evaluate(
				`() => document.querySelector('#workbench-v2-artifact-body input[name="doc_path"]')?.value || ''`,
				nil,
			)
			if err != nil || value != want {
				t.Fatalf(
					"displayed artifact identity = %q, want %q (%v)",
					value,
					want,
					err,
				)
			}
		},
	)
}

func rememberWorkbenchRoot() spec.Step {
	return spec.Custom(
		"remember workbench root",
		func(t testing.TB, ctx *duiruntime.Context) {
			value, err := ctx.Page.Evaluate(
				`() => { const root = document.getElementById('workbench-root'); if (!root) return false; root.dataset.e2eRootIdentity = crypto.randomUUID(); return root.dataset.e2eRootIdentity }`,
				nil,
			)
			if err != nil || value == false {
				t.Fatalf("workbench root missing: %v", err)
			}
			ctx.Memory["workbench-v2.root"] = value.(string)
		},
	)
}

func expectWorkbenchRootUnchanged() spec.Step {
	return spec.Custom(
		"workbench root survives comment morph",
		func(t testing.TB, ctx *duiruntime.Context) {
			value, err := ctx.Page.Locator("#workbench-root").
				GetAttribute("data-e2e-root-identity")
			if err != nil || value != ctx.Memory["workbench-v2.root"] {
				t.Fatalf("comment replaced workbench root: %q %v", value, err)
			}
		},
	)
}

func submitV2WholeDocumentComment(text string) spec.Step {
	return spec.Custom(
		"submit v2 whole-document comment",
		func(t testing.TB, ctx *duiruntime.Context) {
			menu := ctx.Page.Locator("#workbench-v2-artifact-body [data-testid='workbench-overflow-actions']").
				First()
			if err := menu.Locator("summary").Click(); err != nil {
				t.Fatal(err)
			}
			button := menu.GetByRole(*playwright.AriaRoleButton, playwright.LocatorGetByRoleOptions{Name: "Comment"}).
				First()
			if err := button.Click(); err != nil {
				t.Fatal(err)
			}
			form := ctx.Page.Locator("#workbench-v2-artifact-body form:has(textarea[name='comment_text'])").
				First()
			if err := form.Locator("textarea[name='comment_text']").
				Fill(text); err != nil {
				t.Fatal(err)
			}
			if err := form.GetByRole(*playwright.AriaRoleButton, playwright.LocatorGetByRoleOptions{Name: "Comment"}).
				First().
				Click(); err != nil {
				t.Fatal(err)
			}
		},
	)
}

func expectV2CommentsContain(text string) spec.Step {
	return spec.Custom(
		"v2 comments region contains comment",
		func(t testing.TB, ctx *duiruntime.Context) {
			comment := ctx.Page.Locator("#workbench-v2-comments-body").
				GetByText(text, playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).
				First()
			if err := comment.WaitFor(
				playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(30_000),
				},
			); err != nil {
				t.Fatalf("v2 comments missing %q: %v", text, err)
			}
		},
	)
}

func expectV2CommentsAttached(text string) spec.Step {
	return spec.Custom(
		"v2 comments region contains persisted comment",
		func(t testing.TB, ctx *duiruntime.Context) {
			comment := ctx.Page.Locator("#workbench-v2-comments-body").
				GetByText(text, playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).
				First()
			if err := comment.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateAttached,
				Timeout: playwright.Float(30_000),
			}); err != nil {
				t.Fatalf("persisted v2 comment missing %q: %v", text, err)
			}
		},
	)
}

func expectV2CommentsAbsent(text string) spec.Step {
	return spec.Custom(
		"v2 comments region excludes other artifact comment",
		func(t testing.TB, ctx *duiruntime.Context) {
			count, err := ctx.Page.Locator("#workbench-v2-comments-body").
				GetByText(text, playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).
				Count()
			if err != nil || count != 0 {
				t.Fatalf(
					"unexpected comment %q on current artifact: %d %v",
					text,
					count,
					err,
				)
			}
		},
	)
}

func rememberWorkbenchV2StaticHTML() spec.Step {
	return spec.Custom(
		"remember workbench v2 static HTML",
		func(t testing.TB, ctx *duiruntime.Context) {
			root := os.Getenv("VAMOS_E2E_THOUGHTS_ROOT")
			if root == "" {
				root = filepath.Join(ctx.Config.RepoRoot, "thoughts")
			}
			contents, err := os.ReadFile(
				filepath.Join(root, "workbench-v2", "static.html"),
			)
			if err != nil {
				t.Fatal(err)
			}
			ctx.Memory["workbench-v2.static-html"] = string(contents)
		},
	)
}

func expectWorkbenchV2StaticHTMLUnchanged() spec.Step {
	return spec.Custom(
		"workbench v2 static HTML remains unchanged",
		func(t testing.TB, ctx *duiruntime.Context) {
			root := os.Getenv("VAMOS_E2E_THOUGHTS_ROOT")
			if root == "" {
				root = filepath.Join(ctx.Config.RepoRoot, "thoughts")
			}
			contents, err := os.ReadFile(
				filepath.Join(root, "workbench-v2", "static.html"),
			)
			if err != nil {
				t.Fatal(err)
			}
			if string(contents) != ctx.Memory["workbench-v2.static-html"] {
				t.Fatal("static HTML changed after comment")
			}
		},
	)
}

func assertV2OpaqueFrame() spec.Step {
	return spec.Custom(
		"v2 static HTML frame remains opaque",
		func(t testing.TB, ctx *duiruntime.Context) {
			frame := ctx.Page.Locator("#workbench-v2-artifact-body iframe[data-vamos-html-applet]").
				First()
			sandbox, err := frame.GetAttribute("sandbox")
			if err != nil || strings.Contains(sandbox, "allow-same-origin") {
				t.Fatalf("static HTML frame is not opaque: %q %v", sandbox, err)
			}
		},
	)
}

func expectRouteStatus(path string, want int) spec.Step {
	return spec.Custom(
		"route returns expected status",
		func(t testing.TB, ctx *duiruntime.Context) {
			response, err := ctx.Page.Goto(
				ctx.Config.BaseURL+path,
				playwright.PageGotoOptions{
					WaitUntil: playwright.WaitUntilStateDomcontentloaded,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if response == nil || response.Status() != want {
				t.Fatalf("%s status = %v, want %d", path, response, want)
			}
			if want == 200 {
				for _, id := range []string{"workbench-root", "workbench-v2-threads-body", "workbench-v2-chat-body", "workbench-v2-artifact-body", "workbench-v2-comments-body"} {
					count, countErr := ctx.Page.Locator("#" + id).Count()
					if countErr != nil || count != 1 {
						t.Fatalf("%s did not render strict v2 target %s", path, id)
					}
				}
			}
		},
	)
}

func expectEncodedThoughtsRejection(path string) spec.Step {
	return spec.Custom(
		"encoded unsafe Thoughts request is rejected",
		func(t testing.TB, ctx *duiruntime.Context) {
			response, err := ctx.Page.Request().Get(ctx.Config.BaseURL + path)
			if err != nil {
				t.Fatal(err)
			}
			body, err := response.Text()
			if err != nil {
				t.Fatal(err)
			}
			if response.Status() != 400 && response.Status() != 404 {
				t.Fatalf("unsafe Thoughts status = %d", response.Status())
			}
			if strings.Contains(body, "outside thoughts root") ||
				strings.Contains(body, "secret.md") {
				t.Fatalf("unsafe Thoughts response leaked artifact: %q", body)
			}
			if !strings.Contains(strings.ToLower(body), "error") &&
				!strings.Contains(strings.ToLower(body), "invalid") &&
				!strings.Contains(strings.ToLower(body), "not found") {
				t.Fatalf("unsafe Thoughts response was not explicit: %q", body)
			}
		},
	)
}

func expectSafeArtifactFailure(path string) spec.Step {
	return spec.Custom(
		"unsafe artifact has explicit safe outcome",
		func(t testing.TB, ctx *duiruntime.Context) {
			response, err := ctx.Page.Goto(
				ctx.Config.BaseURL+path,
				playwright.PageGotoOptions{
					WaitUntil: playwright.WaitUntilStateDomcontentloaded,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if response == nil || response.Status() < 400 {
				t.Fatalf("unsafe artifact succeeded: %s", path)
			}
		},
	)
}

func assertRouteHydratedVisibility() spec.Step {
	return spec.Custom(
		"route hydrates visibility without persisted state",
		func(t testing.TB, ctx *duiruntime.Context) {
			value, err := ctx.Page.Locator("#workbench-root").
				GetAttribute("data-workbench-mobile-active")
			if err != nil || strings.TrimSpace(value) == "" {
				t.Fatalf("route did not hydrate workbench visibility: %q %v", value, err)
			}
		},
	)
}

func assertNoBrowserDraftStorage() spec.Step {
	return spec.Custom(
		"draft is not browser storage",
		func(t testing.TB, ctx *duiruntime.Context) {
			value, err := ctx.Page.Evaluate(
				`() => ({local: Object.keys(localStorage), session: Object.keys(sessionStorage)})`,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			stores := value.(map[string]any)
			for _, key := range append(stores["local"].([]any), stores["session"].([]any)...) {
				if strings.Contains(strings.ToLower(key.(string)), "draft") {
					t.Fatalf("draft leaked to browser storage: %s", key)
				}
			}
		},
	)
}
