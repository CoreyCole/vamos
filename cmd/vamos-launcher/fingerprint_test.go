package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeRuntimeFingerprintChangesForRuntimeInputs(t *testing.T) {
	root := fakeRuntimeFingerprintRoot(t)
	source := RuntimeSource{Root: root, SourceKey: sourceRootKey(root), SourceFrom: "test"}
	base := mustFingerprint(t, source)

	writeFile(t, filepath.Join(root, "cmd", "vamos-runtime", "main.go"), "package main\nfunc main() { println(\"changed\") }\n")
	changedRuntime := mustFingerprint(t, source)
	if changedRuntime.Value == base.Value {
		t.Fatalf("runtime .go edit did not change fingerprint")
	}

	root = fakeRuntimeFingerprintRoot(t)
	source = RuntimeSource{Root: root, SourceKey: sourceRootKey(root), SourceFrom: "test"}
	base = mustFingerprint(t, source)
	writeFile(t, filepath.Join(root, "pkg", "example", "example.go"), "package example\nconst Value = 2\n")
	if mustFingerprint(t, source).Value == base.Value {
		t.Fatalf("pkg .go edit did not change fingerprint")
	}

	root = fakeRuntimeFingerprintRoot(t)
	source = RuntimeSource{Root: root, SourceKey: sourceRootKey(root), SourceFrom: "test"}
	base = mustFingerprint(t, source)
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/test\nrequire example.com/dep v1.2.3\n")
	if mustFingerprint(t, source).Value == base.Value {
		t.Fatalf("go.mod edit did not change fingerprint")
	}

	root = fakeRuntimeFingerprintRoot(t)
	source = RuntimeSource{Root: root, SourceKey: sourceRootKey(root), SourceFrom: "test"}
	base = mustFingerprint(t, source)
	asset := filepath.Join(root, "cmd", "vamos-runtime", "internal", "qrspicmd", "assets", "q_manager_child_extension.js")
	writeFile(t, asset, "console.log('changed')\n")
	if mustFingerprint(t, source).Value == base.Value {
		t.Fatalf("embedded q-manager JS edit did not change fingerprint")
	}
}

func TestComputeRuntimeFingerprintIgnoresExcludedInputs(t *testing.T) {
	root := fakeRuntimeFingerprintRoot(t)
	source := RuntimeSource{Root: root, SourceKey: sourceRootKey(root), SourceFrom: "test"}
	base := mustFingerprint(t, source)

	excluded := map[string]string{
		filepath.Join("cmd", "vamos-runtime", "foo_test.go"): "package main\nfunc TestX(){}\n",
		filepath.Join("docs", "notes.md"):                    "docs\n",
		filepath.Join("thoughts", "x.md"):                    "thoughts\n",
		filepath.Join(".vamos", "status.json"):               "{}\n",
		filepath.Join(".build-agents", "cache"):              "cache\n",
		filepath.Join("node_modules", "x.js"):                "ignored\n",
		filepath.Join("pkg", "ui", "node_modules", "x.js"):   "ignored\n",
		filepath.Join("dist", "x.js"):                        "ignored\n",
	}
	for rel, contents := range excluded {
		writeFile(t, filepath.Join(root, rel), contents)
	}

	if got := mustFingerprint(t, source); got.Value != base.Value {
		t.Fatalf("excluded inputs changed fingerprint: got %s want %s", got.Value, base.Value)
	}
}

func TestManagedRuntimePathUsesSourceKeyAndFingerprint(t *testing.T) {
	cacheDir := t.TempDir()
	rootA := fakeRuntimeFingerprintRoot(t)
	rootB := fakeRuntimeFingerprintRoot(t)
	fpA := mustFingerprint(t, RuntimeSource{Root: rootA, SourceKey: sourceRootKey(rootA), SourceFrom: "test"})
	fpB := mustFingerprint(t, RuntimeSource{Root: rootB, SourceKey: sourceRootKey(rootB), SourceFrom: "test"})

	targetA := managedRuntimePath(cacheDir, RuntimeSource{Root: rootA, SourceKey: sourceRootKey(rootA), SourceFrom: "test"}, fpA)
	targetB := managedRuntimePath(cacheDir, RuntimeSource{Root: rootB, SourceKey: sourceRootKey(rootB), SourceFrom: "test"}, fpB)
	if targetA.BinaryPath == targetB.BinaryPath {
		t.Fatalf("different source roots got same binary path %q", targetA.BinaryPath)
	}
	for _, path := range []string{targetA.BinaryPath, targetA.LockPath, targetA.MetadataPath, targetA.TempDir} {
		if !strings.HasPrefix(path, cacheDir) {
			t.Fatalf("path %q is outside cache dir %q", path, cacheDir)
		}
	}
	if !strings.Contains(targetA.BinaryPath, sourceRootKey(rootA)) || !strings.Contains(targetA.BinaryPath, fpA.Value) {
		t.Fatalf("binary path %q missing source key or fingerprint", targetA.BinaryPath)
	}
}

func TestDefaultLauncherCacheDirRespectsEnvAndXDG(t *testing.T) {
	override := filepath.Join(t.TempDir(), "override-cache")
	t.Setenv("VAMOS_LAUNCHER_CACHE", override)
	got, err := defaultLauncherCacheDir()
	if err != nil {
		t.Fatalf("defaultLauncherCacheDir override: %v", err)
	}
	if got != override {
		t.Fatalf("cache dir = %q, want %q", got, override)
	}

	t.Setenv("VAMOS_LAUNCHER_CACHE", "")
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	got, err = defaultLauncherCacheDir()
	if err != nil {
		t.Fatalf("defaultLauncherCacheDir xdg: %v", err)
	}
	want := filepath.Join(xdg, "vamos", "launcher")
	if got != want {
		t.Fatalf("cache dir = %q, want %q", got, want)
	}
}

func mustFingerprint(t *testing.T, source RuntimeSource) Fingerprint {
	t.Helper()
	fp, err := computeRuntimeFingerprint(context.Background(), source)
	if err != nil {
		t.Fatalf("computeRuntimeFingerprint: %v", err)
	}
	if fp.Value == "" {
		t.Fatalf("empty fingerprint")
	}
	return fp
}

func fakeRuntimeFingerprintRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/test\n")
	writeFile(t, filepath.Join(root, "go.sum"), "")
	writeFile(t, filepath.Join(root, "cmd", "vamos-runtime", "main.go"), "package main\nfunc main() {}\n")
	writeFile(t, filepath.Join(root, "pkg", "example", "example.go"), "package example\nconst Value = 1\n")
	writeFile(t, filepath.Join(root, "cmd", "vamos-runtime", "internal", "qrspicmd", "assets", "q_manager_child_extension.js"), "console.log('base')\n")
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
