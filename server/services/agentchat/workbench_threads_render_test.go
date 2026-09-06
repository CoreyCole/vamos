package agentchat

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"github.com/CoreyCole/vamos/server/testhelpers"
)

func TestWorkbenchThreadListRendersToggleableProjectFolders(t *testing.T) {
	groups := []WorkbenchThreadGroup{
		{
			Label: "alpha",
			Threads: []WorkbenchThread{
				{ID: "alpha-selected", Title: "Selected Alpha"},
				{ID: "alpha-other", Title: "Other Alpha"},
			},
		},
		{
			Label: "beta",
			Threads: []WorkbenchThread{
				{ID: "beta", Title: "Beta Thread"},
			},
		},
	}
	doc := testhelpers.RenderToDocument(t, WorkbenchThreadList(
		groups,
		"alpha-selected",
		"thoughts/owner/plans/beta/design.md",
	)).Doc

	folders := doc.Find("details[data-thread-project-folder]")
	if folders.Length() != len(groups) {
		t.Fatalf("project folder count = %d, want %d", folders.Length(), len(groups))
	}
	folders.Each(func(_ int, folder *goquery.Selection) {
		if _, open := folder.Attr("open"); !open {
			t.Error("project folder is not initially open")
		}
		summary := folder.ChildrenFiltered("summary")
		if summary.Length() != 1 {
			t.Errorf("direct summary count = %d, want 1", summary.Length())
		}
		if summary.Find("a").Length() != 0 {
			t.Error("project summary contains a navigation link")
		}
		if _, customToggle := summary.Attr("data-on:click"); customToggle {
			t.Error("project summary replaces native disclosure behavior")
		}
		effect, ok := folder.Attr("data-effect")
		if !ok || !strings.Contains(effect, "el.open = true") {
			t.Errorf("search does not reveal collapsed project folder: %q", effect)
		}
	})

	alpha := folders.FilterFunction(func(_ int, folder *goquery.Selection) bool {
		return strings.TrimSpace(
			folder.Find("[data-thread-project-label]").Text(),
		) == "alpha"
	})
	beta := folders.FilterFunction(func(_ int, folder *goquery.Selection) bool {
		return strings.TrimSpace(
			folder.Find("[data-thread-project-label]").Text(),
		) == "beta"
	})
	if alpha.Find("a[href]").Length() != 2 || beta.Find("a[href]").Length() != 1 {
		t.Fatalf(
			"thread nesting: alpha=%d beta=%d",
			alpha.Find("a[href]").Length(),
			beta.Find("a[href]").Length(),
		)
	}
	selected := alpha.Find("a[href^='/threads/alpha-selected']")
	className, _ := selected.Attr("class")
	if selected.Length() != 1 || !strings.Contains(className, "font-medium") {
		t.Fatalf("selected thread styling = %q", className)
	}
	betaHref, _ := beta.Find("a[href]").Attr("href")
	if betaHref != "/threads/beta?artifact=thoughts%2Fowner%2Fplans%2Fbeta%2Fdesign.md" {
		t.Fatalf("beta thread href = %q", betaHref)
	}
	if doc.Find("label[for='workbench-thread-search']").Length() != 1 {
		t.Fatal("thread search lacks an accessible label")
	}
	toggle := doc.Find("button[data-workbench-threads-toggle]")
	action, _ := toggle.Attr("data-on:click")
	if toggle.Length() != 1 ||
		!strings.Contains(action, "workbenchV2Threads.visible = false") ||
		!strings.Contains(action, "wb2_threads_open=0") ||
		toggle.NextAllFiltered("input#workbench-thread-search").Length() != 1 {
		t.Fatalf("threads toggle is not placed before search: %q", action)
	}
	if _, persists := toggle.Attr("data-on:workbench-layout-save"); persists {
		t.Fatal("threads toggle persists temporary visibility")
	}
}
