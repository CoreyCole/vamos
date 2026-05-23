package build

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestBuildTailwindHashedAsset(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	cssDir := filepath.Join(repoRoot, "static", "css")
	writeFile(t, filepath.Join(cssDir, "out.deadbeef.css"), "stale")
	runner := &cssRunner{t: t, repoRoot: repoRoot, output: "body { color: red; }\n"}

	if err := BuildTailwindHashedAsset(t.Context(), repoRoot, runner); err != nil {
		t.Fatalf("BuildTailwindHashedAsset: %v", err)
	}
	if diff := cmp.Diff(
		[]string{
			"pnpm",
			"exec",
			"tailwindcss",
			"-i",
			"static/css/index.css",
			"-o",
			"static/css/out.css",
		},
		runner.calls[0].Args,
	); diff != "" {
		t.Fatalf("tailwind command mismatch (-want +got):\n%s", diff)
	}
	if pathExists(t, filepath.Join(cssDir, "out.deadbeef.css")) {
		t.Fatal("stale hashed CSS survived")
	}
	hash, err := fileHash8(filepath.Join(cssDir, "out.css"))
	if err != nil {
		t.Fatalf("fileHash8: %v", err)
	}
	hashedPath := filepath.Join(cssDir, "out."+hash+".css")
	data, err := os.ReadFile(hashedPath)
	if err != nil {
		t.Fatalf("read hashed CSS: %v", err)
	}
	if string(data) != runner.output {
		t.Fatalf("hashed CSS = %q, want %q", string(data), runner.output)
	}
}

func TestActiveCSSHashPath(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	oldPath := filepath.Join(repoRoot, "static", "css", "out.11111111.css")
	newPath := filepath.Join(repoRoot, "static", "css", "out.22222222.css")
	writeFile(t, oldPath, "old")
	writeFile(t, newPath, "new")
	oldTime := time.Now().Add(-time.Hour)
	newTime := time.Now()
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes old: %v", err)
	}
	if err := os.Chtimes(newPath, newTime, newTime); err != nil {
		t.Fatalf("chtimes new: %v", err)
	}

	got, err := ActiveCSSHashPath(repoRoot)
	if err != nil {
		t.Fatalf("ActiveCSSHashPath: %v", err)
	}
	if got != "static/css/out.22222222.css" {
		t.Fatalf("ActiveCSSHashPath = %q, want newest hashed CSS", got)
	}
}

func TestCompiledTestArtifactDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{path: "pkg/agents/dist/worker.js", want: false},
		{path: "pkg/agents/dist/worker.test.js", want: true},
		{path: "pkg/agents/dist/worker_test.js", want: true},
		{path: "pkg/agents/dist/worker_test.d.ts", want: true},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()

			got := isCompiledTestArtifact(test.path)
			if got != test.want {
				t.Fatalf(
					"isCompiledTestArtifact(%q) = %v, want %v",
					test.path,
					got,
					test.want,
				)
			}
		})
	}
}

type cssRunner struct {
	t        *testing.T
	repoRoot string
	output   string
	calls    []CommandSpec
}

func (r *cssRunner) Run(_ context.Context, spec CommandSpec) error {
	r.calls = append(r.calls, spec)
	writeFile(r.t, filepath.Join(r.repoRoot, "static", "css", "out.css"), r.output)
	return nil
}
