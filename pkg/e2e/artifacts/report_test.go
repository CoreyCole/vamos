package artifacts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteManifestFailuresAndReport(t *testing.T) {
	root := t.TempDir()
	manifest, err := NewRun(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Stories = []string{"thoughts-workbench"}
	failures := []Failure{{Story: "thoughts-workbench", Scenario: "root", Error: "boom"}}
	paths := []string{}
	for _, write := range []func(string, RunManifest) (string, error){WriteManifest, func(root string, manifest RunManifest) (string, error) {
		return WriteMarkdownReport(root, manifest, failures)
	}} {
		path, err := write(root, manifest)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	failuresPath, err := WriteFailures(root, manifest, failures)
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, failuresPath)
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s: %v", path, err)
		}
		if filepath.Dir(path) != RunDir(root, manifest) {
			t.Fatalf("path %s not under run dir %s", path, RunDir(root, manifest))
		}
	}
}
