package goldens

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/CoreyCole/vamos/pkg/e2e/artifacts"
)

func TestAcceptRequiresHumanApproval(t *testing.T) {
	err := Accept(
		context.Background(),
		artifacts.RunManifest{},
		AcceptOptions{GoldenRoot: t.TempDir()},
	)
	if err == nil {
		t.Fatal("Accept() error=nil want approval error")
	}
}

func TestCaptureCopiesScreenshotsWithRunHierarchy(t *testing.T) {
	tmp := t.TempDir()
	run := artifacts.RunManifest{ID: "run-1"}
	src := filepath.Join(
		tmp,
		"runs",
		run.ID,
		"story",
		"scenario",
		"desktop-full",
		"page.png",
	)
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	run.Screenshots = []string{src}
	root := filepath.Join(tmp, "goldens")
	if err := Capture(
		context.Background(),
		run,
		CaptureOptions{GoldenRoot: root},
	); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "story", "scenario.desktop-full.png"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "png" {
		t.Fatalf("golden content=%q", string(got))
	}
}

func TestLoadManifestReadsManifestJSON(t *testing.T) {
	dir := t.TempDir()
	want := artifacts.RunManifest{ID: "run-1"}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID {
		t.Fatalf("ID=%q want %q", got.ID, want.ID)
	}
}
