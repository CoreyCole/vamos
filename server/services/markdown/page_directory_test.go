//go:build !integration || unit
// +build !integration unit

package markdown

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetDirectoryListingIncludesRenderableFormats(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "note.md"), []byte("# Note"))
	mustWriteFile(t, filepath.Join(root, "app.html"), []byte("<h1>App</h1>"))
	mustWriteFile(t, filepath.Join(root, "legacy.htm"), []byte("<h1>Legacy</h1>"))
	mustWriteFile(t, filepath.Join(root, "data.csv"), []byte("a,b\n1,2"))
	mustWriteFile(t, filepath.Join(root, "image.png"), []byte("skip"))
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	listing, err := service.GetDirectoryListing("")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, item := range listing.Items {
		names[item.Name] = true
	}
	for _, want := range []string{"note", "app.html", "legacy.htm", "data.csv"} {
		if !names[want] {
			t.Fatalf("missing %q in %#v", want, listing.Items)
		}
	}
	if names["image.png"] {
		t.Fatalf("image should be skipped: %#v", listing.Items)
	}
}

func TestGetDirectoryListingRejectsEscapesAndBuildsNavigation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "owner", "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(root, "owner", "docs", "plan.md"), []byte("# Plan"))
	outside := t.TempDir()
	mustWriteFile(t, filepath.Join(outside, "secret.md"), []byte("# Secret"))
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	listing, err := service.GetDirectoryListing("owner/docs")
	if err != nil {
		t.Fatal(err)
	}
	if listing.Parent != "owner" || len(listing.Breadcrumbs) != 2 {
		t.Fatalf("unexpected navigation: %#v", listing)
	}
	for _, escaped := range []string{"../", "escape"} {
		if _, err := service.GetDirectoryListing(escaped); err == nil {
			t.Fatalf("GetDirectoryListing(%q) succeeded", escaped)
		}
	}
}

func TestDirectoryPrimaryPanelRendersAnchors(t *testing.T) {
	t.Parallel()

	args := &DirectoryArgs{Items: []DirectoryItem{
		{Name: "docs", Path: "owner/docs", IsDir: true},
		{Name: "plan.md", Path: "owner/plan.md"},
	}}
	var buf bytes.Buffer
	if err := DirectoryPrimaryPanel(args).Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{`href="/thoughts/owner/docs"`, `href="/thoughts/owner/plan.md"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %s", want, html)
		}
	}
	for _, notWant := range []string{"/thoughts/actions/select-directory", "/thoughts/actions/select-document", "data-on:submit"} {
		if strings.Contains(html, notWant) {
			t.Fatalf("unexpected %q in %s", notWant, html)
		}
	}
}

func TestDirectoryPrimaryPanelOmitsLegacyChatWorkspaceQuery(t *testing.T) {
	t.Parallel()

	args := &DirectoryArgs{
		WorkbenchLinkState: ThoughtsWorkbenchLinkState{
			Context:         "chat",
			ChatWorkspaceID: "ws 1",
			ChatRunID:       "run+1",
		},
		Items: []DirectoryItem{
			{Name: "docs", Path: "owner/docs", IsDir: true},
			{Name: "plan.md", Path: "owner/plan.md"},
		},
	}
	var buf bytes.Buffer
	if err := DirectoryPrimaryPanel(args).Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, notWant := range []string{"context=chat", "chat_workspace=ws+1", "run=run%2B1"} {
		if strings.Contains(html, notWant) {
			t.Fatalf("legacy query leaked into directory link %q in %s", notWant, html)
		}
	}
}

func TestDirectoryPrimaryPanelUsesLocalSearchAndBreadcrumbs(t *testing.T) {
	t.Parallel()

	args := &DirectoryArgs{
		Path:   "owner/docs",
		Parent: "owner",
		Breadcrumbs: []DirectoryBreadcrumb{
			{Name: "owner", Path: "owner"},
			{Name: "docs", Path: "owner/docs"},
		},
		Items: []DirectoryItem{{Name: "Plan", Path: "owner/docs/plan.md"}},
	}
	var buf bytes.Buffer
	if err := DirectoryPrimaryPanel(args).Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{`data-signals="{dirSearch: ''}"`, `data-bind="dirSearch"`, `data-show="String(&#34;Plan&#34;).toLowerCase().includes(String($dirSearch || &#39;&#39;).toLowerCase())"`, `href="/thoughts/owner"`, `href="/thoughts/owner/docs"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %s", want, html)
		}
	}
}
