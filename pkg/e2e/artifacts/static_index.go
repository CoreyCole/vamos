package artifacts

import (
	"html/template"
	"os"
	"path/filepath"
	"sort"
)

type StaticIndexOptions struct {
	CDNURL string
}

type FeatureGroup struct {
	Slug      string
	Scenarios []ScenarioGroup
}

type ScenarioGroup struct {
	Slug      string
	Viewports []ViewportGroup
}

type ViewportGroup struct {
	Name       string
	Screenshot *ArtifactEntry
	HTML       *ArtifactEntry
	Traces     []ArtifactEntry
	Entries    []ArtifactEntry
}

type staticIndexModel struct {
	Manifest        RunManifest
	CDNURL          string
	Features        []FeatureGroup
	ScreenshotCount int
	HTMLCount       int
	TraceCount      int
}

func DefaultDatastarCDNURL() string {
	return "https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.1/bundles/datastar.js"
}

func WriteStaticIndex(manifest RunManifest, runDir string, opts StaticIndexOptions) (string, error) {
	if opts.CDNURL == "" {
		opts.CDNURL = DefaultDatastarCDNURL()
	}
	model := staticIndexModel{
		Manifest:        manifest,
		CDNURL:          opts.CDNURL,
		Features:        groupArtifactsByScenario(manifest.Artifacts),
		ScreenshotCount: countKind(manifest.Artifacts, ArtifactKindScreenshot),
		HTMLCount:       countKind(manifest.Artifacts, ArtifactKindHTML),
		TraceCount:      countKind(manifest.Artifacts, ArtifactKindTrace),
	}
	path := filepath.Join(runDir, "index.html")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := staticIndexTemplate.Execute(f, model); err != nil {
		return "", err
	}
	return path, nil
}

func groupArtifactsByScenario(entries []ArtifactEntry) []FeatureGroup {
	entries = append([]ArtifactEntry{}, entries...)
	sort.Slice(entries, func(i, j int) bool {
		return artifactSortKey(entries[i]) < artifactSortKey(entries[j])
	})

	featureIndexes := map[string]int{}
	features := []FeatureGroup{}
	for _, entry := range entries {
		featureIndex, ok := featureIndexes[entry.FeatureSlug]
		if !ok {
			featureIndex = len(features)
			featureIndexes[entry.FeatureSlug] = featureIndex
			features = append(features, FeatureGroup{Slug: entry.FeatureSlug})
		}

		scenarios := &features[featureIndex].Scenarios
		scenarioIndex := indexScenario(*scenarios, entry.ScenarioSlug)
		if scenarioIndex == -1 {
			scenarioIndex = len(*scenarios)
			*scenarios = append(*scenarios, ScenarioGroup{Slug: entry.ScenarioSlug})
		}

		viewports := &(*scenarios)[scenarioIndex].Viewports
		viewportIndex := indexViewport(*viewports, entry.Viewport)
		if viewportIndex == -1 {
			viewportIndex = len(*viewports)
			*viewports = append(*viewports, ViewportGroup{Name: entry.Viewport})
		}

		viewport := &(*viewports)[viewportIndex]
		viewport.Entries = append(viewport.Entries, entry)
		switch entry.Kind {
		case ArtifactKindScreenshot:
			if viewport.Screenshot == nil {
				entryCopy := entry
				viewport.Screenshot = &entryCopy
			}
		case ArtifactKindHTML:
			if viewport.HTML == nil {
				entryCopy := entry
				viewport.HTML = &entryCopy
			}
		case ArtifactKindTrace:
			viewport.Traces = append(viewport.Traces, entry)
		}
	}
	return features
}

func artifactSortKey(entry ArtifactEntry) string {
	return entry.FeatureSlug + "\x00" + entry.ScenarioSlug + "\x00" + entry.Viewport + "\x00" + string(entry.Kind) + "\x00" + entry.Label + "\x00" + entry.Path
}

func indexScenario(scenarios []ScenarioGroup, slug string) int {
	for i, scenario := range scenarios {
		if scenario.Slug == slug {
			return i
		}
	}
	return -1
}

func indexViewport(viewports []ViewportGroup, name string) int {
	for i, viewport := range viewports {
		if viewport.Name == name {
			return i
		}
	}
	return -1
}

func countKind(entries []ArtifactEntry, kind ArtifactKind) int {
	count := 0
	for _, entry := range entries {
		if entry.Kind == kind {
			count++
		}
	}
	return count
}

var staticIndexTemplate = template.Must(template.New("static-index").Parse(`<!doctype html>
<html lang="en" data-signals='{"query":"","viewport":"all","expanded":true}'>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Vamos E2E run {{ .Manifest.ID }}</title>
  <style>
    :root { color-scheme: light dark; --background: #ffffff; --foreground: #0f172a; --muted: #64748b; --border: #dbe4ef; --card: #ffffff; --accent: #2563eb; --accent-foreground: #ffffff; }
    @media (prefers-color-scheme: dark) { :root { --background: #020617; --foreground: #e2e8f0; --muted: #94a3b8; --border: #1e293b; --card: #0f172a; --accent: #60a5fa; --accent-foreground: #020617; } }
    * { box-sizing: border-box; }
    body { margin: 0; background: var(--background); color: var(--foreground); font: 14px/1.5 ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    main { width: min(1180px, calc(100% - 32px)); margin: 0 auto; padding: 32px 0; }
    h1, h2, h3, p { margin-top: 0; }
    a { color: var(--accent); }
    img { display: block; width: 100%; max-height: 720px; object-fit: contain; border: 1px solid var(--border); border-radius: 12px; background: #fff; }
    [data-slot="card"] { background: var(--card); border: 1px solid var(--border); border-radius: 16px; box-shadow: 0 1px 2px rgb(15 23 42 / 0.06); margin: 16px 0; overflow: hidden; }
    [data-slot="card-header"] { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 16px; border-bottom: 1px solid var(--border); }
    [data-slot="card-content"] { padding: 16px; }
    [data-slot="badge"] { display: inline-flex; align-items: center; border: 1px solid var(--border); border-radius: 999px; padding: 2px 10px; color: var(--muted); font-size: 12px; font-weight: 600; }
    [data-slot="button"] { appearance: none; border: 1px solid var(--border); border-radius: 10px; background: var(--card); color: var(--foreground); cursor: pointer; padding: 8px 12px; font-weight: 600; }
    [data-slot="button"][data-active="true"] { background: var(--accent); color: var(--accent-foreground); border-color: var(--accent); }
    [data-slot="input"] { width: min(420px, 100%); border: 1px solid var(--border); border-radius: 10px; background: var(--card); color: var(--foreground); padding: 9px 12px; }
    .summary-grid, .artifact-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px; }
    .summary-item { border: 1px solid var(--border); border-radius: 12px; padding: 12px; }
    .muted { color: var(--muted); }
    .controls { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin: 16px 0; }
    .links { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 12px; }
    summary { cursor: pointer; font-weight: 700; }
    details > summary { padding: 16px; }
    details > [data-slot="card-content"] { border-top: 1px solid var(--border); }
    code { overflow-wrap: anywhere; }
  </style>
  <script type="module" src="{{ .CDNURL }}"></script>
</head>
<body>
<main>
  <header>
    <p class="muted">Vamos E2E verification artifact</p>
    <h1>Run {{ .Manifest.ID }}</h1>
    <section class="summary-grid" aria-label="Run summary">
      <div class="summary-item"><strong>Command</strong><br><code>{{ .Manifest.Command }}</code></div>
      <div class="summary-item"><strong>Base URL</strong><br><code>{{ .Manifest.BaseURL }}</code></div>
      <div class="summary-item"><strong>Viewports</strong><br><code>{{ .Manifest.ViewportFilter }}</code></div>
      <div class="summary-item"><strong>Commit</strong><br><code>{{ .Manifest.RepoCommit }}</code></div>
      <div class="summary-item"><strong>Artifacts</strong><br>{{ .ScreenshotCount }} screenshots · {{ .HTMLCount }} HTML · {{ .TraceCount }} traces</div>
    </section>
  </header>

  <nav class="controls" aria-label="Artifact filters">
    <input data-slot="input" type="search" placeholder="Filter text" data-bind="query" data-on:input="$query = evt.target.value">
    <button data-slot="button" data-on:click="$viewport = 'all'">All viewports</button>
    <button data-slot="button" data-on:click="$viewport = 'mobile'">Mobile</button>
    <button data-slot="button" data-on:click="$viewport = 'desktop-half'">Desktop half</button>
    <button data-slot="button" data-on:click="$viewport = 'desktop-full'">Desktop full</button>
  </nav>

  {{ range .Features }}
  <details open data-slot="card" data-feature="{{ .Slug }}">
    <summary>{{ .Slug }}</summary>
    <section data-slot="card-content">
      {{ range .Scenarios }}
      <details open data-slot="card" data-scenario="{{ .Slug }}">
        <summary>{{ .Slug }}</summary>
        <section data-slot="card-content" class="artifact-grid">
          {{ range .Viewports }}
          <article data-slot="card" data-viewport="{{ .Name }}" data-show="$viewport == 'all' || $viewport == '{{ .Name }}'">
            <header data-slot="card-header">
              <div><span data-slot="badge">{{ .Name }}</span></div>
              <button data-slot="button" data-on:click="$expanded = !$expanded">Toggle</button>
            </header>
            <section data-slot="card-content" data-show="$expanded">
              {{ if .Screenshot }}<a href="{{ .Screenshot.Path }}"><img src="{{ .Screenshot.Path }}" alt="{{ .Name }} screenshot"></a>{{ else }}<p class="muted">No screenshot artifact.</p>{{ end }}
              <div class="links">
                {{ if .HTML }}<a href="{{ .HTML.Path }}">HTML snapshot</a>{{ end }}
                {{ range .Traces }}<a href="{{ .Path }}">Trace: {{ .Label }}</a>{{ end }}
              </div>
            </section>
          </article>
          {{ end }}
        </section>
      </details>
      {{ end }}
    </section>
  </details>
  {{ end }}
</main>
</body>
</html>
`))
