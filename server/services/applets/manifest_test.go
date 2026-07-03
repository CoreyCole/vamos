package applets

import (
	"strings"
	"testing"
)

func TestParseAppletManifestDatastarWithAliases(t *testing.T) {
	body := []byte(`---
vamos_artifact: applet
applet:
  id: wordle
  kind: datastar
  title: Wordle
  files_root: files
  source_dir: apps/wordle
  start_command: ["wordle"]
  health_path: /healthz
  root_aliases:
    - pattern: /events
      methods: [GET]
    - pattern: /guesses
      methods: [POST]
---
# Wordle
`)
	manifest, err := ParseAppletManifest(body)
	if err != nil {
		t.Fatalf("ParseAppletManifest() error = %v", err)
	}
	if manifest.ID != "wordle" || manifest.Kind != AppletKindDatastar || manifest.Title != "Wordle" {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if got := manifest.RootAliases[1].Methods; len(got) != 1 || got[0] != "POST" {
		t.Fatalf("alias methods = %#v", got)
	}
	if err := ValidateAppletManifest(manifest); err != nil {
		t.Fatalf("ValidateAppletManifest() error = %v", err)
	}
}

func TestParseAppletManifestStreamlitDefaults(t *testing.T) {
	body := []byte(`---
vamos_artifact: applet
applet:
  id: sales_dashboard
  kind: streamlit
  source_dir: dashboards/sales
  start_command: ["streamlit", "run", "app.py"]
---
`)
	manifest, err := ParseAppletManifest(body)
	if err != nil {
		t.Fatalf("ParseAppletManifest() error = %v", err)
	}
	patterns := aliasPatterns(manifest.RootAliases)
	if !contains(patterns, "/_stcore/*") || !contains(patterns, "/vendor/*") {
		t.Fatalf("streamlit aliases = %#v", patterns)
	}
	if contains(patterns, "/static/*") {
		t.Fatalf("streamlit aliases should not default /static/*: %#v", patterns)
	}
	if manifest.HealthPath != "/healthz" {
		t.Fatalf("health path = %q", manifest.HealthPath)
	}
}

func TestParseAppletManifestRejectsNonAppletFrontmatter(t *testing.T) {
	_, err := ParseAppletManifest([]byte("---\nvamos_artifact: note\n---\n"))
	if err == nil || !strings.Contains(err.Error(), "not a Vamos applet") {
		t.Fatalf("ParseAppletManifest() error = %v", err)
	}
}

func TestValidateAppletManifestRejectsUnsafeValues(t *testing.T) {
	base := AppletManifest{ID: "wordle", Kind: AppletKindHTTP, SourceDir: ".", StartCommand: []string{"app"}}
	for name, manifest := range map[string]AppletManifest{
		"unsafe id":       withID(base, "Wordle"),
		"relative alias":  withAliases(base, []RouteAlias{{Pattern: "events"}}),
		"broad alias":     withAliases(base, []RouteAlias{{Pattern: "/*"}}),
		"traversal alias": withAliases(base, []RouteAlias{{Pattern: "/../events"}}),
		"reserved alias":  withAliases(base, []RouteAlias{{Pattern: "/api/events"}}),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateAppletManifest(manifest); err == nil {
				t.Fatal("ValidateAppletManifest() unexpectedly succeeded")
			}
		})
	}
}

func TestDefaultAppletManifestMapsLegacyAppDir(t *testing.T) {
	manifest := DefaultAppletManifest("legacy", AppletManifest{AppDir: "examples/legacy", StartCommand: []string{"app"}})
	if manifest.ID != "legacy" || manifest.SourceDir != "examples/legacy" || manifest.FilesRoot != "files" || manifest.Kind != AppletKindHTTP {
		t.Fatalf("unexpected default manifest: %+v", manifest)
	}
}

func aliasPatterns(aliases []RouteAlias) []string {
	out := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		out = append(out, alias.Pattern)
	}
	return out
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func withID(manifest AppletManifest, id string) AppletManifest {
	manifest.ID = id
	return manifest
}

func withAliases(manifest AppletManifest, aliases []RouteAlias) AppletManifest {
	manifest.RootAliases = aliases
	return manifest
}
