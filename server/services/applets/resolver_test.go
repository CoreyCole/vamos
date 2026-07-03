package applets

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestResolverResolveExampleApplet(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, filepath.Join(root, "wordle", "AGENTS.md"), `---
vamos_artifact: applet
applet:
  id: wordle
  title: Wordle
  app_dir: app
  files_root: files
  start_command: ["wordle"]
  env:
    WORDLE_MODE: test
---
`)
	ctx, err := (Resolver{ExamplesRoot: root}).ResolveExampleApplet(context.Background(), "wordle")
	if err != nil {
		t.Fatalf("ResolveExampleApplet() error = %v", err)
	}
	if ctx.SourceKind != AppletSourceExample {
		t.Fatalf("SourceKind = %q", ctx.SourceKind)
	}
	if ctx.IdentityPath != "examples/wordle/AGENTS.md" {
		t.Fatalf("IdentityPath = %q", ctx.IdentityPath)
	}
	if ctx.RuntimeKey != "wordle" {
		t.Fatalf("RuntimeKey = %q", ctx.RuntimeKey)
	}
	if ctx.RouteHref != "/examples/wordle" || ctx.IFrameSrc != "/examples/wordle/app/" || ctx.StatusURL != "/examples/wordle/status" {
		t.Fatalf("unexpected routes: %+v", ctx)
	}
	if ctx.Manifest.SourceDir != filepath.Join(root, "wordle", "app") {
		t.Fatalf("SourceDir = %q", ctx.Manifest.SourceDir)
	}
}

func TestResolverResolveThoughtsApplet(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, filepath.Join(root, "plans", "demo", "applet.md"), `---
vamos_artifact: applet
applet:
  id: demo
  title: Demo
  source_dir: apps/demo
  start_command: ["demo"]
---
`)
	ctx, err := (Resolver{ThoughtsRoot: root}).ResolveThoughtsApplet(context.Background(), "thoughts/plans/demo/applet.md")
	if err != nil {
		t.Fatalf("ResolveThoughtsApplet() error = %v", err)
	}
	wantToken := EncodeAppletIdentity("thoughts/plans/demo/applet.md")
	if ctx.SourceKind != AppletSourceThoughts {
		t.Fatalf("SourceKind = %q", ctx.SourceKind)
	}
	if ctx.IdentityPath != "thoughts/plans/demo/applet.md" {
		t.Fatalf("IdentityPath = %q", ctx.IdentityPath)
	}
	if ctx.RuntimeKey != wantToken {
		t.Fatalf("RuntimeKey = %q", ctx.RuntimeKey)
	}
	if ctx.RouteHref != "/thoughts/_render/app/"+wantToken || ctx.IFrameSrc != "/thoughts/_render/app/"+wantToken+"/app/" {
		t.Fatalf("unexpected routes: %+v", ctx)
	}
}

func TestResolverResolveThoughtsAppletDirectoryUsesAgentsManifest(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, filepath.Join(root, "plans", "demo", "AGENTS.md"), `---
vamos_artifact: applet
applet:
  id: demo
  title: Demo
  source_dir: app
  start_command: ["demo"]
---
`)
	ctx, err := (Resolver{ThoughtsRoot: root}).ResolveThoughtsApplet(context.Background(), "thoughts/plans/demo")
	if err != nil {
		t.Fatalf("ResolveThoughtsApplet() error = %v", err)
	}
	wantIdentity := "thoughts/plans/demo/AGENTS.md"
	wantToken := EncodeAppletIdentity(wantIdentity)
	if ctx.IdentityPath != wantIdentity {
		t.Fatalf("IdentityPath = %q", ctx.IdentityPath)
	}
	if ctx.RuntimeKey != wantToken {
		t.Fatalf("RuntimeKey = %q", ctx.RuntimeKey)
	}
	if ctx.RouteHref != "/thoughts/_render/app/"+wantToken || ctx.IFrameSrc != "/thoughts/_render/app/"+wantToken+"/app/" || ctx.StatusURL != "/thoughts/_render/app/"+wantToken+"/status" {
		t.Fatalf("unexpected routes: %+v", ctx)
	}
}

func TestResolverResolveThoughtsAppletToken(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, filepath.Join(root, "plans", "demo", "AGENTS.md"), `---
vamos_artifact: applet
applet:
  id: demo
  source_dir: app
  start_command: ["demo"]
---
`)
	identity := "thoughts/plans/demo/AGENTS.md"
	ctx, err := (Resolver{ThoughtsRoot: root}).ResolveThoughtsAppletToken(context.Background(), EncodeAppletIdentity(identity))
	if err != nil {
		t.Fatalf("ResolveThoughtsAppletToken() error = %v", err)
	}
	if ctx.IdentityPath != identity || ctx.RuntimeKey != EncodeAppletIdentity(identity) {
		t.Fatalf("unexpected token resolution: %+v", ctx)
	}
}

func TestResolverDuplicateThoughtsManifestIDsUseDistinctRuntimeKeys(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"one", "two"} {
		writeManifest(t, filepath.Join(root, "plans", dir, "AGENTS.md"), `---
vamos_artifact: applet
applet:
  id: demo
  source_dir: app
  start_command: ["demo"]
---
`)
	}
	one, err := (Resolver{ThoughtsRoot: root}).ResolveThoughtsApplet(context.Background(), "thoughts/plans/one")
	if err != nil {
		t.Fatalf("ResolveThoughtsApplet(one) error = %v", err)
	}
	two, err := (Resolver{ThoughtsRoot: root}).ResolveThoughtsApplet(context.Background(), "thoughts/plans/two")
	if err != nil {
		t.Fatalf("ResolveThoughtsApplet(two) error = %v", err)
	}
	if one.Manifest.ID != "demo" || two.Manifest.ID != "demo" {
		t.Fatalf("manifest IDs = %q %q", one.Manifest.ID, two.Manifest.ID)
	}
	if one.RuntimeKey == two.RuntimeKey || one.RuntimeKey == "demo" || two.RuntimeKey == "demo" {
		t.Fatalf("runtime keys are not durable path keys: %q %q", one.RuntimeKey, two.RuntimeKey)
	}
}

func TestResolverThoughtsRelativePathsResolveFromManifestDir(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "plans", "demo", "AGENTS.md")
	writeManifest(t, manifestPath, `---
vamos_artifact: applet
applet:
  id: demo
  source_dir: apps/demo
  files_root: files
  start_command: ["demo"]
---
`)
	ctx, err := (Resolver{ThoughtsRoot: root}).ResolveThoughtsApplet(context.Background(), "thoughts/plans/demo")
	if err != nil {
		t.Fatalf("ResolveThoughtsApplet() error = %v", err)
	}
	base := filepath.Dir(manifestPath)
	if ctx.Manifest.SourceDir != filepath.Join(base, "apps", "demo") {
		t.Fatalf("SourceDir = %q", ctx.Manifest.SourceDir)
	}
	if ctx.Manifest.FilesRoot != filepath.Join(base, "files") {
		t.Fatalf("FilesRoot = %q", ctx.Manifest.FilesRoot)
	}
}

func TestResolverRejectsThoughtsPathEscape(t *testing.T) {
	resolver := Resolver{ThoughtsRoot: t.TempDir()}
	for _, path := range []string{
		"thoughts/../secrets/applet.md",
		"thoughts/demo/../other/AGENTS.md",
		"thoughts/demo/..",
	} {
		t.Run(path, func(t *testing.T) {
			_, err := resolver.ResolveThoughtsApplet(context.Background(), path)
			if err == nil {
				t.Fatal("ResolveThoughtsApplet() unexpectedly succeeded")
			}
		})
	}
}

func TestDecodeAppletIdentityRejectsUnsafePaths(t *testing.T) {
	for _, identity := range []string{
		"",
		"/thoughts/demo/AGENTS.md",
		"thoughts/demo/../other/AGENTS.md",
		"thoughts/demo/..",
		"../thoughts/demo/AGENTS.md",
	} {
		t.Run(identity, func(t *testing.T) {
			token := base64.RawURLEncoding.EncodeToString([]byte(identity))
			_, err := DecodeAppletIdentity(token)
			if err == nil {
				t.Fatal("DecodeAppletIdentity() unexpectedly succeeded")
			}
		})
	}
}

func TestRuntimeConfigFromManifest(t *testing.T) {
	ctx := AppletContext{RuntimeKey: "thoughts-token", Manifest: AppletManifest{
		ID:           "demo",
		FilesRoot:    "/files",
		SourceDir:    "/files/apps/demo",
		BuildCommand: []string{"go", "build"},
		StartCommand: []string{"demo"},
		HealthPath:   "/healthz",
		IdleTimeout:  5,
		Env:          map[string]string{"A": "B"},
	}}
	cfg := RuntimeConfigFromManifest(ctx)
	if cfg.AppID != "thoughts-token" || cfg.FilesRoot != "/files" || cfg.SourceDir != "/files/apps/demo" || cfg.HealthPath != "/healthz" || cfg.IdleTimeout != 5 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if len(cfg.BuildCommand) != 2 || cfg.BuildCommand[0] != "go" || len(cfg.StartCommand) != 1 || cfg.StartCommand[0] != "demo" {
		t.Fatalf("commands not preserved: %+v", cfg)
	}
	if cfg.Env["A"] != "B" {
		t.Fatalf("env not preserved: %#v", cfg.Env)
	}
	ctx.Manifest.Env["A"] = "changed"
	if cfg.Env["A"] != "B" {
		t.Fatal("RuntimeConfigFromManifest did not clone env")
	}
}

func TestRuntimeConfigFromManifestFallsBackToManifestID(t *testing.T) {
	cfg := RuntimeConfigFromManifest(AppletContext{Manifest: AppletManifest{ID: "wordle"}})
	if cfg.AppID != "wordle" {
		t.Fatalf("AppID = %q", cfg.AppID)
	}
}

func writeManifest(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
