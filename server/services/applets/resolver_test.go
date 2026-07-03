package applets

import (
	"context"
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
	if ctx.IdentityPath != "examples/wordle/AGENTS.md" {
		t.Fatalf("IdentityPath = %q", ctx.IdentityPath)
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
	if ctx.IdentityPath != "thoughts/plans/demo/applet.md" {
		t.Fatalf("IdentityPath = %q", ctx.IdentityPath)
	}
	if ctx.RouteHref != "/thoughts/_render/app/demo" || ctx.IFrameSrc != "/thoughts/_render/app/demo/app/" {
		t.Fatalf("unexpected routes: %+v", ctx)
	}
}

func TestResolverRejectsThoughtsPathEscape(t *testing.T) {
	_, err := (Resolver{ThoughtsRoot: t.TempDir()}).ResolveThoughtsApplet(context.Background(), "thoughts/../secrets/applet.md")
	if err == nil {
		t.Fatal("ResolveThoughtsApplet() unexpectedly succeeded")
	}
}

func TestRuntimeConfigFromManifest(t *testing.T) {
	ctx := AppletContext{Manifest: AppletManifest{
		ID:           "demo",
		FilesRoot:    "/files",
		SourceDir:    "/files/apps/demo",
		BuildCommand: []string{"go", "build"},
		StartCommand: []string{"demo"},
		HealthPath:   "/healthz",
		Env:          map[string]string{"A": "B"},
	}}
	cfg := RuntimeConfigFromManifest(ctx)
	if cfg.AppID != "demo" || cfg.FilesRoot != "/files" || cfg.SourceDir != "/files/apps/demo" || cfg.HealthPath != "/healthz" {
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

func writeManifest(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
