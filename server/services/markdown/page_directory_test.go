//go:build !integration || unit
// +build !integration unit

package markdown

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDirectoryListingIncludesRenderableFormats(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "note.md"), []byte("# Note"))
	mustWriteFile(t, filepath.Join(root, "app.html"), []byte("<h1>App</h1>"))
	mustWriteFile(t, filepath.Join(root, "legacy.htm"), []byte("<h1>Legacy</h1>"))
	mustWriteFile(t, filepath.Join(root, "data.csv"), []byte("a,b\n1,2"))
	mustWriteFile(t, filepath.Join(root, "image.png"), []byte("png"))
	mustWriteFile(t, filepath.Join(root, "photo.jpg"), []byte("jpg"))
	mustWriteFile(t, filepath.Join(root, "anim.gif"), []byte("gif"))
	mustWriteFile(t, filepath.Join(root, "shot.webp"), []byte("webp"))
	mustWriteFile(t, filepath.Join(root, "vector.svg"), []byte("<svg></svg>"))
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
	for _, want := range []string{"note", "app.html", "legacy.htm", "data.csv", "image.png", "photo.jpg", "anim.gif", "shot.webp"} {
		if !names[want] {
			t.Fatalf("missing %q in %#v", want, listing.Items)
		}
	}
	if names["vector.svg"] {
		t.Fatalf("svg should be skipped: %#v", listing.Items)
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
