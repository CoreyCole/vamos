package applets

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CoreyCole/vamos/server/services/appletruntime"
)

type AppletSourceKind string

const (
	AppletSourceThoughts AppletSourceKind = "thoughts"
	AppletSourceExample  AppletSourceKind = "example"
)

type AppletContext struct {
	Manifest     AppletManifest
	SourceKind   AppletSourceKind
	IdentityPath string
	RuntimeKey   string
	RouteHref    string
	IFrameSrc    string
	StatusURL    string
}

type Resolver struct {
	ThoughtsRoot string
	ExamplesRoot string
}

func EncodeAppletIdentity(identityPath string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cleanSlashPath(identityPath)))
}

func DecodeAppletIdentity(token string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil {
		return "", fmt.Errorf("decode applet identity: %w", err)
	}
	raw := filepath.ToSlash(strings.TrimSpace(string(decoded)))
	if unsafeAppletIdentity(raw) {
		return "", fmt.Errorf("invalid applet identity %q", raw)
	}
	return cleanSlashPath(raw), nil
}

func (r Resolver) ResolveThoughtsApplet(_ context.Context, docPath string) (AppletContext, error) {
	identity, absPath, err := canonicalThoughtsManifestPath(r.ThoughtsRoot, docPath)
	if err != nil {
		return AppletContext{}, err
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
	manifest = resolveManifestRelativePaths(manifest, absPath)
	if err := ValidateAppletManifest(manifest); err != nil {
		return AppletContext{}, err
	}
	token := EncodeAppletIdentity(identity)
	return appletContext(AppletSourceThoughts, identity, token, manifest, "/thoughts/_render/app/"+token), nil
}

func (r Resolver) ResolveThoughtsAppletToken(ctx context.Context, token string) (AppletContext, error) {
	identity, err := DecodeAppletIdentity(token)
	if err != nil {
		return AppletContext{}, err
	}
	if !strings.HasPrefix(identity, "thoughts/") {
		return AppletContext{}, fmt.Errorf("applet identity %q is not a thoughts path", identity)
	}
	return r.ResolveThoughtsApplet(ctx, identity)
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
	return appletContext(AppletSourceExample, "examples/"+id+"/AGENTS.md", id, manifest, "/examples/"+id), nil
}

func RuntimeConfigFromManifest(applet AppletContext) appletruntime.RuntimeConfig {
	manifest := applet.Manifest
	appID := strings.TrimSpace(applet.RuntimeKey)
	if appID == "" {
		appID = manifest.ID
	}
	return appletruntime.RuntimeConfig{
		AppID:        appID,
		FilesRoot:    manifest.FilesRoot,
		SourceDir:    manifest.SourceDir,
		BuildCommand: append([]string(nil), manifest.BuildCommand...),
		StartCommand: append([]string(nil), manifest.StartCommand...),
		HealthPath:   manifest.HealthPath,
		IdleTimeout:  manifest.IdleTimeout,
		Env:          cloneStringMap(manifest.Env),
	}
}

func appletContext(sourceKind AppletSourceKind, identity, runtimeKey string, manifest AppletManifest, routeHref string) AppletContext {
	routeHref = strings.TrimRight(routeHref, "/")
	if manifest.AppRoute == "" {
		manifest.AppRoute = routeHref + "/app/"
	}
	return AppletContext{
		Manifest:     manifest,
		SourceKind:   sourceKind,
		IdentityPath: identity,
		RuntimeKey:   runtimeKey,
		RouteHref:    routeHref,
		IFrameSrc:    strings.TrimRight(manifest.AppRoute, "/") + "/",
		StatusURL:    routeHref + "/status",
	}
}

func canonicalThoughtsManifestPath(thoughtsRoot, requested string) (string, string, error) {
	raw := filepath.ToSlash(strings.TrimSpace(requested))
	if unsafeAppletIdentity(raw) {
		return "", "", fmt.Errorf("invalid thoughts applet path %q", requested)
	}
	identity := cleanSlashPath(raw)
	if !strings.HasPrefix(identity, "thoughts/") {
		identity = "thoughts/" + strings.TrimPrefix(identity, "/")
	}
	root, err := filepath.Abs(strings.TrimSpace(thoughtsRoot))
	if err != nil || root == "" {
		return "", "", fmt.Errorf("resolve thoughts root: %w", err)
	}
	rel := strings.TrimPrefix(identity, "thoughts/")
	candidate := filepath.Join(root, filepath.FromSlash(rel))
	if !pathWithinRoot(root, candidate) {
		return "", "", fmt.Errorf("thoughts applet path escapes root")
	}
	if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
		identity = strings.TrimRight(identity, "/") + "/AGENTS.md"
		candidate = filepath.Join(candidate, "AGENTS.md")
	}
	if !pathWithinRoot(root, candidate) {
		return "", "", fmt.Errorf("thoughts applet path escapes root")
	}
	return identity, candidate, nil
}

func resolveManifestRelativePaths(manifest AppletManifest, manifestAbsPath string) AppletManifest {
	base := filepath.Dir(manifestAbsPath)
	if manifest.SourceDir != "" && !filepath.IsAbs(manifest.SourceDir) {
		manifest.SourceDir = filepath.Join(base, filepath.FromSlash(manifest.SourceDir))
	}
	if manifest.FilesRoot != "" && !filepath.IsAbs(manifest.FilesRoot) {
		manifest.FilesRoot = filepath.Join(base, filepath.FromSlash(manifest.FilesRoot))
	}
	return manifest
}

func unsafeAppletIdentity(path string) bool {
	return path == "" || path == "." || filepath.IsAbs(path) || strings.HasPrefix(path, "../") || strings.Contains(path, "/../") || strings.HasSuffix(path, "/..")
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
