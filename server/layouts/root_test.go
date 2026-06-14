//go:build !integration || unit
// +build !integration unit

package layouts

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestRootLoadsDatastarPolyfillsWhenProUnavailable(t *testing.T) {
	SetDatastarProAvailable(false)

	body := renderLayoutComponent(t, Root(RootArgs{}))
	for _, want := range []string{
		"Datastar Pro asset unavailable; falling back to public Datastar bundle",
		"https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.1/bundles/datastar.js",
		"/js/vamos-datastar-polyfills.js",
		"polyfills.install(datastar)",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("Root() missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, `src="/js/datastar-pro-v1.js"`) {
		t.Fatalf("Root() loaded Pro asset when fallback expected:\n%s", body)
	}
}

func TestRootLoadsDatastarProDirectlyWhenAvailable(t *testing.T) {
	SetDatastarProAvailable(true)
	t.Cleanup(func() { SetDatastarProAvailable(false) })

	body := renderLayoutComponent(t, Root(RootArgs{}))
	for _, want := range []string{
		`src="/js/datastar-pro-v1.js"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("Root() missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, "falling back to public Datastar bundle") {
		t.Fatalf("Root() rendered fallback when Pro expected:\n%s", body)
	}
}

func TestRootSkipsDatastarInspectorWhenAssetUnavailable(t *testing.T) {
	SetDatastarInspectorAvailable(false)

	body := renderLayoutComponent(t, Root(RootArgs{}))
	if strings.Contains(body, `/js/datastar-inspector.js`) {
		t.Fatalf("Root() rendered missing inspector asset:\n%s", body)
	}
}

func TestRootLoadsDatastarInspectorWhenAssetAvailable(t *testing.T) {
	SetDatastarInspectorAvailable(true)
	t.Cleanup(func() { SetDatastarInspectorAvailable(false) })

	body := renderLayoutComponent(t, Root(RootArgs{}))
	if !strings.Contains(body, `/js/datastar-inspector.js`) {
		t.Fatalf("Root() missing inspector asset when available:\n%s", body)
	}
}

func TestVamosDatastarPolyfillAssetContainsSafetyContracts(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile("../../static/js/vamos-datastar-polyfills.js")
	if err != nil {
		t.Fatalf("ReadFile(vamos-datastar-polyfills.js) error = %v", err)
	}
	js := string(contents)
	for _, want := range []string{
		"export function install(datastar)",
		"installClipboard(datastar)",
		"installReplaceURL(datastar)",
		"export function resolveSameOriginURL",
		"url.origin !== window.location.origin",
		"history.replaceState",
		"MutationObserver",
		"@clipboard",
		"stopImmediatePropagation",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("polyfill missing %q in:\n%s", want, js)
		}
	}
}

func TestHeaderVisibleNavIsMinimal(t *testing.T) {
	t.Parallel()

	items := headerVisibleNavItems()
	if got, want := len(items), 1; got != want {
		t.Fatalf("headerVisibleNavItems() len = %d, want %d", got, want)
	}
	if got, want := items[0].Label, "Thoughts"; got != want {
		t.Fatalf("visible nav label = %q, want %q", got, want)
	}
	if got, want := items[0].Href, "/thoughts/"; got != want {
		t.Fatalf("visible nav href = %q, want %q", got, want)
	}

	body := renderLayoutComponent(t, Header(RootArgs{PageType: PageTypeAgentChat}))
	for _, want := range []string{"Vamos", `href="/"`, "Thoughts", `href="/thoughts/"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("Header() missing %q: %s", want, body)
		}
	}
	for _, removed := range []string{"Agent Chat / New chat", "header_mobile_product_nav", "$header_mobile_product_nav.open"} {
		if strings.Contains(body, removed) {
			t.Fatalf("Header() still contains removed mobile nav %q: %s", removed, body)
		}
	}
}

func TestBuildBreadcrumbsFromPathLinksParentsAndCurrentWithoutThoughtsRoot(t *testing.T) {
	t.Parallel()

	crumbs := buildBreadcrumbsFromPath("owner/plans/demo/outline.md", PageTypeMarkdown, BreadcrumbLinkState{})
	want := []struct {
		label string
		href  string
	}{
		{label: "Owner", href: "/thoughts/owner"},
		{label: "Plans", href: "/thoughts/owner/plans"},
		{label: "Demo", href: "/thoughts/owner/plans/demo"},
		{label: "Outline", href: ""},
	}
	if got, wantLen := len(crumbs), len(want); got != wantLen {
		t.Fatalf("crumbs len = %d, want %d: %#v", got, wantLen, crumbs)
	}
	for i, wantCrumb := range want {
		if crumbs[i].Label != wantCrumb.label || crumbs[i].Href != wantCrumb.href {
			t.Fatalf("crumb[%d] = %#v, want label %q href %q", i, crumbs[i], wantCrumb.label, wantCrumb.href)
		}
	}
}

func TestBuildBreadcrumbsFromPathPreservesChatQuery(t *testing.T) {
	t.Parallel()

	crumbs := buildBreadcrumbsFromPath("owner/plans/demo/outline.md", PageTypeMarkdown, BreadcrumbLinkState{
		Active:      true,
		WorkspaceID: "ws 1",
		ThreadID:    "th/1",
		RunID:       "run+1",
	})
	if len(crumbs) < 2 {
		t.Fatalf("crumbs len = %d, want at least 2", len(crumbs))
	}
	for _, crumb := range crumbs[:len(crumbs)-1] {
		if crumb.Label == "Thoughts" {
			t.Fatalf("breadcrumbs should not duplicate top-level Thoughts nav: %#v", crumbs)
		}
		for _, want := range []string{"context=chat", "thread=th%2F1", "run=run%2B1"} {
			if !strings.Contains(crumb.Href, want) {
				t.Fatalf("crumb href missing %q: %#v", want, crumb)
			}
		}
		if strings.Contains(crumb.Href, "chat_workspace=") {
			t.Fatalf("crumb href preserved chat_workspace with thread: %#v", crumb)
		}
	}
	if got := crumbs[len(crumbs)-1].Href; got != "" {
		t.Fatalf("current crumb href = %q, want empty", got)
	}
}

func TestHeaderBreadcrumbsAlignAfterThoughtsWithoutDuplicateRoot(t *testing.T) {
	t.Parallel()

	body := renderLayoutComponent(t, Header(RootArgs{
		PageType:    PageTypeMarkdown,
		CurrentPath: "CoreyCole/plans/example/outline.md",
	}))

	thoughtsIndex := strings.Index(body, `href="/thoughts/"`)
	breadcrumbIndex := strings.Index(body, `aria-label="breadcrumb"`)
	if thoughtsIndex < 0 || breadcrumbIndex < 0 || breadcrumbIndex < thoughtsIndex {
		t.Fatalf("breadcrumb should render after Thoughts nav link: %s", body)
	}
	breadcrumbSegment := body[breadcrumbIndex:]
	if strings.Contains(breadcrumbSegment, ">Thoughts</a>") || strings.Contains(breadcrumbSegment, ">Thoughts</span>") {
		t.Fatalf("breadcrumb should not duplicate top-level Thoughts nav: %s", breadcrumbSegment)
	}
	for _, notWant := range []string{"justify-end", "data-init=\"el.scrollLeft = el.scrollWidth\"", "min-w-full"} {
		if strings.Contains(breadcrumbSegment, notWant) {
			t.Fatalf(
				"breadcrumb should be left-aligned, found %q in: %s",
				notWant,
				breadcrumbSegment,
			)
		}
	}
}

func TestHeaderAvatarContainsSecondaryActions(t *testing.T) {
	SetGitHubLinkProvider(func(currentPath string) string {
		return "https://github.example.test/acme/project/blob/main/" + strings.TrimSuffix(
			currentPath,
			"/",
		)
	})
	t.Cleanup(func() { SetGitHubLinkProvider(nil) })

	body := renderLayoutComponent(t, Header(RootArgs{
		PageType:           PageTypeSystem,
		UserEmail:          "user@example.com",
		CurrentPath:        "thoughts/example.md",
		ClipboardContent:   "encoded-doc",
		CurrentSyntaxTheme: "github-dark",
	}))
	for _, want := range []string{
		"System", "Storybook", "Syntax:", "syntax_theme_select", "/api/syntax-theme", "Toggle theme", "Copy document", "View on GitHub",
		`href="/system"`, `href="/storybook"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("avatar menu missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "Pipelines") ||
		strings.Contains(body, `href="/pipe`+`lines"`) {
		t.Fatalf("avatar menu still contains pipelines nav: %s", body)
	}
	if strings.Contains(body, `title="Copy document to clipboard"`) {
		t.Fatalf("desktop copy control still rendered outside avatar: %s", body)
	}
	if strings.Contains(body, `<span class="sr-only">Toggle theme</span>`) {
		t.Fatalf("old icon-only ThemeToggle still rendered outside avatar: %s", body)
	}
}

func TestHeaderAvatarRendersPageExtra(t *testing.T) {
	t.Parallel()

	extra := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<button>Discussions</button>`)
		return err
	})
	body := renderLayoutComponent(t, Header(RootArgs{
		PageType:        PageTypeMarkdown,
		UserEmail:       "user@example.com",
		AvatarMenuExtra: extra,
	}))
	if !strings.Contains(body, "Discussions") {
		t.Fatalf("Header() missing avatar extra: %s", body)
	}
	if strings.Contains(body, "HeaderExtra") {
		t.Fatalf("HeaderExtra leaked into output: %s", body)
	}
}

func TestHeaderWorkspaceSwitcher(t *testing.T) {
	t.Parallel()

	body := renderLayoutComponent(t, Header(RootArgs{
		PageType:            PageTypeAgentChat,
		UserEmail:           "user@example.com",
		WorkspaceManagerURL: "https://main.cn-agents.test",
		Workspaces: []WorkspaceNavItem{
			{
				Slug:    "main",
				Label:   "main",
				URL:     "https://main.cn-agents.test",
				Status:  "running",
				Current: true,
			},
			{
				Slug:   "feature",
				Label:  "feature",
				URL:    "https://main.cn-agents.test/workspaces",
				Status: "stopped",
			},
		},
	}))
	for _, want := range []string{
		"Workspaces",
		`href="https://main.cn-agents.test"`,
		"current",
		`href="https://main.cn-agents.test/workspaces"`,
		"Manage workspaces",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("Header() missing %q: %s", want, body)
		}
	}
	for _, notWant := range []string{"feature", "stopped"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("Header() rendered stopped workspace %q: %s", notWant, body)
		}
	}
}

func TestAgentChatDoesNotActivateThoughts(t *testing.T) {
	t.Parallel()

	if isThoughtsPageType(PageTypeAgentChat) {
		t.Fatal("PageTypeAgentChat should not be classified as Thoughts")
	}
	if !isThoughtsPageType(PageTypeMarkdown) {
		t.Fatal("PageTypeMarkdown should be classified as Thoughts")
	}

	body := renderLayoutComponent(t, Header(RootArgs{PageType: PageTypeAgentChat}))
	thoughtsHref := strings.LastIndex(body, `href="/thoughts/"`)
	if thoughtsHref < 0 {
		t.Fatalf("Header() missing Thoughts link: %s", body)
	}
	segmentStart := max(0, thoughtsHref-240)
	thoughtsSegment := body[segmentStart:thoughtsHref]
	if strings.Contains(thoughtsSegment, "text-foreground font-medium") {
		t.Fatalf("Thoughts nav appears active for AgentChat: %s", thoughtsSegment)
	}
}

func renderLayoutComponent(t *testing.T, component templ.Component) string {
	t.Helper()

	var body bytes.Buffer
	if err := component.Render(t.Context(), &body); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return body.String()
}
