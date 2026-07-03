package applets

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type AppletKind string

const (
	AppletKindDatastar  AppletKind = "datastar"
	AppletKindStreamlit AppletKind = "streamlit"
	AppletKindHTTP      AppletKind = "http"
)

type AppletManifest struct {
	ID           string            `yaml:"id"`
	Kind         AppletKind        `yaml:"kind"`
	Title        string            `yaml:"title"`
	FilesRoot    string            `yaml:"files_root"`
	SourceDir    string            `yaml:"source_dir"`
	AppDir       string            `yaml:"app_dir"`
	BuildCommand []string          `yaml:"build_command"`
	StartCommand []string          `yaml:"start_command"`
	HealthPath   string            `yaml:"health_path"`
	AppRoute     string            `yaml:"app_route"`
	RootAliases  []RouteAlias      `yaml:"root_aliases"`
	IdleTimeout  time.Duration     `yaml:"idle_timeout"`
	Env          map[string]string `yaml:"env"`
}

type RouteAlias struct {
	Pattern string   `yaml:"pattern"`
	Methods []string `yaml:"methods"`
}

type appletFrontmatter struct {
	VamosArtifact string         `yaml:"vamos_artifact"`
	Applet        AppletManifest `yaml:"applet"`
}

var safeAppletID = regexp.MustCompile(`^[a-z0-9_-]+$`)

func ParseAppletManifest(body []byte) (AppletManifest, error) {
	frontmatter, err := parseAppletFrontmatter(body)
	if err != nil {
		return AppletManifest{}, err
	}
	if strings.TrimSpace(frontmatter.VamosArtifact) != "applet" {
		return AppletManifest{}, errors.New("frontmatter is not a Vamos applet")
	}
	manifest := frontmatter.Applet
	if manifest.SourceDir == "" {
		manifest.SourceDir = manifest.AppDir
	}
	manifest = ApplyKindDefaults(manifest)
	return manifest, nil
}

func ApplyKindDefaults(manifest AppletManifest) AppletManifest {
	if manifest.Kind == "" {
		manifest.Kind = AppletKindHTTP
	}
	if manifest.HealthPath == "" {
		manifest.HealthPath = "/healthz"
	}
	if manifest.Kind == AppletKindStreamlit && len(manifest.RootAliases) == 0 {
		manifest.RootAliases = StreamlitDefaultAliases()
	}
	if manifest.Kind == AppletKindDatastar && len(manifest.RootAliases) == 0 {
		manifest.RootAliases = DatastarDefaultAliases()
	}
	return manifest
}

func DefaultAppletManifest(id string, manifest AppletManifest) AppletManifest {
	id = strings.TrimSpace(id)
	manifest = ApplyKindDefaults(manifest)
	if strings.TrimSpace(manifest.ID) == "" {
		manifest.ID = id
	}
	if strings.TrimSpace(manifest.Title) == "" {
		manifest.Title = manifest.ID
	}
	if manifest.SourceDir == "" {
		manifest.SourceDir = manifest.AppDir
	}
	if manifest.SourceDir == "" {
		manifest.SourceDir = "."
	}
	if manifest.FilesRoot == "" {
		manifest.FilesRoot = "files"
	}
	if manifest.Env == nil {
		manifest.Env = map[string]string{}
	}
	return manifest
}

func ValidateAppletManifest(manifest AppletManifest) error {
	if !safeAppletID.MatchString(strings.TrimSpace(manifest.ID)) {
		return fmt.Errorf("unsafe applet id %q", manifest.ID)
	}
	if manifest.Kind != AppletKindHTTP && manifest.Kind != AppletKindDatastar && manifest.Kind != AppletKindStreamlit {
		return fmt.Errorf("unsupported applet kind %q", manifest.Kind)
	}
	if strings.TrimSpace(manifest.SourceDir) == "" {
		return errors.New("source_dir is required")
	}
	if len(manifest.StartCommand) == 0 || strings.TrimSpace(manifest.StartCommand[0]) == "" {
		return errors.New("start_command is required")
	}
	for _, alias := range manifest.RootAliases {
		if err := validateRouteAlias(alias); err != nil {
			return err
		}
	}
	return nil
}

func DatastarDefaultAliases() []RouteAlias {
	return nil
}

func StreamlitDefaultAliases() []RouteAlias {
	return []RouteAlias{
		{Pattern: "/_stcore/*"},
		{Pattern: "/vendor/*"},
	}
}

func parseAppletFrontmatter(body []byte) (appletFrontmatter, error) {
	trimmed := bytes.TrimSpace(body)
	if !bytes.HasPrefix(trimmed, []byte("---\n")) {
		return appletFrontmatter{}, errors.New("applet manifest missing frontmatter")
	}
	rest := trimmed[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return appletFrontmatter{}, errors.New("applet manifest frontmatter is not closed")
	}
	var frontmatter appletFrontmatter
	if err := yaml.Unmarshal(rest[:end], &frontmatter); err != nil {
		return appletFrontmatter{}, err
	}
	return frontmatter, nil
}

func validateRouteAlias(alias RouteAlias) error {
	pattern := strings.TrimSpace(alias.Pattern)
	if pattern == "" {
		return errors.New("route alias pattern is required")
	}
	if !strings.HasPrefix(pattern, "/") {
		return fmt.Errorf("route alias %q must be absolute", pattern)
	}
	if pattern == "/" || pattern == "/*" {
		return fmt.Errorf("route alias %q is too broad", pattern)
	}
	if strings.Contains(pattern, "..") {
		return fmt.Errorf("route alias %q contains unsafe traversal", pattern)
	}
	for _, reserved := range []string{"/api", "/forms", "/thoughts", "/agent-chat"} {
		if pattern == reserved || strings.HasPrefix(pattern, reserved+"/") || strings.HasPrefix(pattern, reserved+"/*") {
			return fmt.Errorf("route alias %q conflicts with reserved Vamos prefix %q", pattern, reserved)
		}
	}
	if strings.Contains(pattern[:len(pattern)-1], "*") || strings.Count(pattern, "*") > 1 {
		return fmt.Errorf("route alias %q has unsupported wildcard placement", pattern)
	}
	for _, method := range alias.Methods {
		if strings.TrimSpace(method) == "" {
			return fmt.Errorf("route alias %q has empty method", pattern)
		}
	}
	return nil
}
