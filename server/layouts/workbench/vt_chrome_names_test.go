package workbench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkbenchV2ChromeNamesStableShape(t *testing.T) {
	t.Parallel()
	names := WorkbenchV2ChromeNames()
	if len(names) < 10 {
		t.Fatalf("chrome name map too small: %d", len(names))
	}
	seenProbe := map[string]string{}
	seenSel := map[string]int{}
	for _, e := range names {
		if e.Selector == "" || e.Name == "" || e.Media == "" {
			t.Fatalf("incomplete entry: %#v", e)
		}
		if e.MustBeNone && e.Name != "none" {
			t.Fatalf("MustBeNone entry must use name none: %#v", e)
		}
		if e.Freeze && e.MustBeNone {
			t.Fatalf("none parents should not freeze: %#v", e)
		}
		seenSel[e.Selector]++
		if e.ProbeKey == "" {
			continue
		}
		if prev, ok := seenProbe[e.ProbeKey]; ok {
			t.Fatalf("duplicate ProbeKey %q on %s and %s", e.ProbeKey, prev, e.Selector)
		}
		seenProbe[e.ProbeKey] = e.Selector
	}
	for sel, n := range seenSel {
		if n != 1 {
			t.Fatalf("selector %s appears %d times", sel, n)
		}
	}
	desktop := DesktopSiblingDocNameInventory()
	if desktop["chat"] != "workbench-v2-chat" || desktop["tabs"] != "none" || desktop["artifact"] != "none" {
		t.Fatalf("desktop inventory = %#v", desktop)
	}
	if desktop["threadsReopen"] != "workbench-v2-threads-reopen" {
		t.Fatalf("desktop missing threadsReopen: %#v", desktop)
	}
	mobile := MobileSiblingDocNameInventory()
	if mobile["tabs"] != "workbench-mobile-tabs" || mobile["chat"] != "workbench-v2-chat" {
		t.Fatalf("mobile inventory = %#v", mobile)
	}
	chat := VTChromeName{}
	for _, e := range names {
		if e.Selector == "#workbench-v2-chat" {
			chat = e
			break
		}
	}
	if chat.ExpectedComputedName(true, true) != "none" {
		t.Fatal("chat must unname under thread-switch")
	}
}

func TestWorkbenchV2ChromeNamesMatchCSS(t *testing.T) {
	t.Parallel()
	cssPath := filepath.Join("..", "..", "..", "static", "css", "index.css")
	contents, err := os.ReadFile(cssPath)
	if err != nil {
		t.Fatalf("ReadFile(index.css): %v", err)
	}
	css := string(contents)
	for _, want := range CSSPresenceSnippets() {
		if !strings.Contains(css, want) {
			t.Fatalf("index.css missing name-map snippet %q", want)
		}
	}
	if strings.Contains(css, "view-transition-name: workbench-v2-artifact;") {
		t.Fatal("parent #workbench-v2-artifact must stay view-transition-name: none")
	}
}

func TestWorkbenchV2ChromeNamesMatchDocs(t *testing.T) {
	t.Parallel()
	docsPath := filepath.Join("..", "..", "..", "docs", "workbench-view-transitions.md")
	contents, err := os.ReadFile(docsPath)
	if err != nil {
		t.Fatalf("ReadFile(docs): %v", err)
	}
	docs := string(contents)
	table := DocsNameMapMarkdownTable()
	if !strings.Contains(docs, table) {
		t.Fatalf("docs name map must match WorkbenchV2ChromeNames table exactly.\nwant:\n%s", table)
	}
	if !strings.Contains(docs, "server/layouts/workbench/vt_chrome_names.go") {
		t.Fatal("docs must point at vt_chrome_names.go as SoT")
	}
}
