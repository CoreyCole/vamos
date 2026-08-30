package vamos

import (
	"testing"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"
)

var WorkbenchV2 workbenchV2Feature

type workbenchV2Feature struct{}

func (workbenchV2Feature) Root() spec.Locator {
	return spec.CSS("#workbench-root[data-workbench-page='threads']")
}

func (workbenchV2Feature) Threads() spec.Locator { return spec.CSS("#workbench-v2-threads-body") }

func (workbenchV2Feature) Chat() spec.Locator { return spec.CSS("#workbench-v2-chat-body") }

func (workbenchV2Feature) Artifact() spec.Locator { return spec.CSS("#workbench-v2-artifact-body") }

func (workbenchV2Feature) Comments() spec.Locator { return spec.CSS("#workbench-v2-comments-body") }
func (workbenchV2Feature) Composer() spec.Locator {
	return spec.CSS("#workbench-v2-chat-body #agent-chat-composer-input")
}

func workbenchOrderValue(value any) int {
	switch value := value.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return -1
	}
}

func (workbenchV2Feature) Ready() expectation {
	return expectation{
		customStep(
			"strict workbench v2 chrome is present",
			func(t testing.TB, ctx *duiruntime.Context) {
				root := resolveLocator(t, ctx, WorkbenchV2.Root())
				if count, err := root.Count(); err != nil || count != 1 {
					t.Fatalf("expected one v2 workbench root, got %d: %v", count, err)
				}
				for _, region := range []spec.Locator{WorkbenchV2.Threads(), WorkbenchV2.Chat(), WorkbenchV2.Artifact(), WorkbenchV2.Comments()} {
					if count, err := resolveLocator(
						t,
						ctx,
						region,
					).Count(); err != nil ||
						count != 1 {
						t.Fatalf("expected one v2 region, got %d: %v", count, err)
					}
				}
				data, err := ctx.Page.Evaluate(
					`() => { const ids = ['workbench-v2-threads-body', 'workbench-v2-chat-body', 'workbench-v2-artifact-body', 'workbench-v2-comments-body']; const bodies = ids.map(id => document.getElementById(id)); const order = bodies.map(el => [...document.querySelectorAll('#workbench-regions *')].indexOf(el)); const html = document.documentElement.innerHTML; const anchors = [...document.querySelectorAll('#workbench-v2-threads-body a[href], #workbench-v2-artifact-body a[href]')]; const rendered = [...anchors.map(a => a.getAttribute('href') || ''), ...[...document.querySelectorAll('form[action], input[type="hidden"]')].map(el => el.getAttribute('action') || el.value || '')]; return { badRendered: rendered.filter(v => /(?:[?&]|^)(context|chat_workspace|thread|run|hermes_thread)=/.test(v)), legacy: [...document.querySelectorAll("[id^='doc-workbench-'], [id^='doc-right-']")].map(el => el.id), ignored: bodies.filter(el => el && el.closest('[data-ignore-morph]')).map(el => el.id), order, badOutput: /rightRailActiveTab|docWorkbenchRight|data-replace-url|pushState|replaceState/.test(html), badQuery: /(?:^|[?&])(context|chat_workspace|thread|run|hermes_thread)=/.test(location.search), intercepted: anchors.filter(a => !a.closest('[data-thread-artifact-browser]') && (a.getAttribute('data-on:click') || a.getAttribute('data-on-click'))).map(a => a.href) } }`,
					nil,
				)
				if err != nil {
					t.Fatal(err)
				}
				value := data.(map[string]any)
				order := value["order"].([]any)
				ordered := len(order) == 4 &&
					workbenchOrderValue(order[0]) < workbenchOrderValue(order[1]) &&
					workbenchOrderValue(order[1]) < workbenchOrderValue(order[2]) &&
					workbenchOrderValue(order[2]) < workbenchOrderValue(order[3])
				if len(value["legacy"].([]any)) != 0 ||
					len(value["ignored"].([]any)) != 0 ||
					!ordered ||
					value["badOutput"].(bool) ||
					value["badQuery"].(bool) ||
					len(
						value["intercepted"].([]any),
					) != 0 || len(value["badRendered"].([]any)) != 0 {
					t.Fatalf("invalid v2 shell: %#v", value)
				}
			},
		),
	}
}
