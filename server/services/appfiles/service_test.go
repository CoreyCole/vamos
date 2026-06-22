package appfiles

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListExcludesHiddenAndSortsDirectoriesFirst(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "apps", "current"))
	mustMkdir(t, filepath.Join(root, "apps", "iterations", "hidden"))
	mustMkdir(t, filepath.Join(root, "zdir"))
	mustWrite(t, filepath.Join(root, "players.csv"), "name\nAda\n")
	mustWrite(t, filepath.Join(root, "notes.txt"), "hello")
	mustWrite(t, filepath.Join(root, "apps", "iterations", "hidden", "debug.log"), "hidden")

	vm, err := NewService().List(t.Context(), BrowserConfig{Root: root, HiddenPaths: []string{"apps/iterations"}, RoutePrefix: "/files"}, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := nodeNames(vm.Nodes)
	want := []string{"apps", "zdir", "notes.txt", "players.csv"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("nodes = %v, want %v", got, want)
	}
	for _, node := range vm.Nodes {
		if strings.Contains(node.Path, "iterations") {
			t.Fatalf("hidden node leaked: %#v", node)
		}
	}
}

func TestRenderSupportsHTMLCSVTextAndUnsupported(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "page.html"), "<strong>Tournament</strong>")
	mustWrite(t, filepath.Join(root, "players.csv"), "name,level\nAda,4.0\n")
	mustWrite(t, filepath.Join(root, "notes.md"), "# Notes\nFriendly")
	mustWrite(t, filepath.Join(root, "image.png"), "png")

	svc := NewService()
	cases := []struct {
		path string
		want string
	}{
		{path: "page.html", want: "Tournament"},
		{path: "players.csv", want: "Ada"},
		{path: "notes.md", want: "Friendly"},
		{path: "image.png", want: "This file type can be downloaded."},
	}
	for _, tc := range cases {
		component, err := svc.Render(t.Context(), RenderRequest{Root: root, Path: tc.path})
		if err != nil {
			t.Fatalf("render %s: %v", tc.path, err)
		}
		var body bytes.Buffer
		if err := component.Render(t.Context(), &body); err != nil {
			t.Fatalf("render component %s: %v", tc.path, err)
		}
		if !strings.Contains(body.String(), tc.want) {
			t.Fatalf("rendered %s = %q, want %q", tc.path, body.String(), tc.want)
		}
	}
}

func TestFilesBrowserRendersFriendlyLabels(t *testing.T) {
	vm := FilesViewModel{
		Title: "Files",
		Nodes: []FileNode{{Path: "players.csv", Name: "players.csv", Renderable: true, DownloadURL: "/files/players.csv"}},
	}
	var body bytes.Buffer
	if err := FilesBrowser(vm).Render(t.Context(), &body); err != nil {
		t.Fatalf("render browser: %v", err)
	}
	html := body.String()
	for _, want := range []string{"Files", "players.csv", "Download"} {
		if !strings.Contains(html, want) {
			t.Fatalf("browser = %q, missing %q", html, want)
		}
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func nodeNames(nodes []FileNode) []string {
	names := make([]string, len(nodes))
	for i, node := range nodes {
		names[i] = node.Name
	}
	return names
}
