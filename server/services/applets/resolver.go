package applets

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CoreyCole/vamos/server/services/appletruntime"
)

type AppletContext struct {
	Manifest     AppletManifest
	IdentityPath string
	RouteHref    string
	IFrameSrc    string
	StatusURL    string
}

type Resolver struct {
	ThoughtsRoot string
	ExamplesRoot string
}

func (r Resolver) ResolveThoughtsApplet(_ context.Context, docPath string) (AppletContext, error) {
	identity := cleanSlashPath(docPath)
	if identity == "" || strings.HasPrefix(identity, "../") || strings.Contains(identity, "/../") || filepath.IsAbs(docPath) {
		return AppletContext{}, fmt.Errorf("invalid thoughts applet path %q", docPath)
	}
	root, err := filepath.Abs(strings.TrimSpace(r.ThoughtsRoot))
	if err != nil || root == "" {
		return AppletContext{}, fmt.Errorf("resolve thoughts root: %w", err)
	}
	rel := strings.TrimPrefix(identity, "thoughts/")
	absPath := filepath.Join(root, filepath.FromSlash(rel))
	if !pathWithinRoot(root, absPath) {
		return AppletContext{}, fmt.Errorf("thoughts applet path escapes root")
	}
	body, err := os.ReadFile(absPath)
	if err != nil {
		return AppletContext{}, err
	}
	manifest, err := ParseAppletManifest(body)
	if err != nil {
		return AppletContext{}, err
	}
	manifest = DefaultAppletManifest(manifest.ID, manifest)
	if err := ValidateAppletManifest(manifest); err != nil {
		return AppletContext{}, err
	}
	return appletContext(identity, manifest, "/thoughts/_render/app/"+manifest.ID), nil
}

func (r Resolver) ResolveExampleApplet(_ context.Context, id string) (AppletContext, error) {
	id = strings.TrimSpace(id)
	if !safeAppletID.MatchString(id) {
		return AppletContext{}, fmt.Errorf("invalid example applet id %q", id)
	}
	root, err := filepath.Abs(firstNonEmpty(strings.TrimSpace(r.ExamplesRoot), "examples"))
	if err != nil {
		return AppletContext{}, fmt.Errorf("resolve examples root: %w", err)
	}
	manifestPath := filepath.Join(root, id, "AGENTS.md")
	if !pathWithinRoot(root, manifestPath) {
		return AppletContext{}, fmt.Errorf("example applet path escapes root")
	}
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		return AppletContext{}, err
	}
	manifest, err := ParseAppletManifest(body)
	if err != nil {
		return AppletContext{}, err
	}
	manifest = DefaultAppletManifest(id, manifest)
	if manifest.ID != id {
		return AppletContext{}, fmt.Errorf("example id %q does not match applet id %q", id, manifest.ID)
	}
	if manifest.SourceDir != "" && !filepath.IsAbs(manifest.SourceDir) {
		manifest.SourceDir = filepath.Join(root, id, filepath.FromSlash(manifest.SourceDir))
	}
	if manifest.FilesRoot != "" && !filepath.IsAbs(manifest.FilesRoot) {
		manifest.FilesRoot = filepath.Join(root, id, filepath.FromSlash(manifest.FilesRoot))
	}
	if err := ValidateAppletManifest(manifest); err != nil {
		return AppletContext{}, err
	}
	return appletContext("examples/"+id+"/AGENTS.md", manifest, "/examples/"+id), nil
}

func RuntimeConfigFromManifest(applet AppletContext) appletruntime.RuntimeConfig {
	manifest := applet.Manifest
	return appletruntime.RuntimeConfig{
		AppID:        manifest.ID,
		FilesRoot:    manifest.FilesRoot,
		SourceDir:    manifest.SourceDir,
		BuildCommand: append([]string(nil), manifest.BuildCommand...),
		StartCommand: append([]string(nil), manifest.StartCommand...),
		HealthPath:   manifest.HealthPath,
		IdleTimeout:  manifest.IdleTimeout,
		Env:          cloneStringMap(manifest.Env),
	}
}

func appletContext(identity string, manifest AppletManifest, routeHref string) AppletContext {
	routeHref = strings.TrimRight(routeHref, "/")
	if manifest.AppRoute == "" {
		manifest.AppRoute = routeHref + "/app/"
	}
	return AppletContext{
		Manifest:     manifest,
		IdentityPath: identity,
		RouteHref:    routeHref,
		IFrameSrc:    strings.TrimRight(manifest.AppRoute, "/") + "/",
		StatusURL:    routeHref + "/status",
	}
}

func cleanSlashPath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	return filepath.ToSlash(filepath.Clean(path))
}

func pathWithinRoot(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
