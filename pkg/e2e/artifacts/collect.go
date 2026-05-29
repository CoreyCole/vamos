package artifacts

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

type CollectResult struct {
	Entries       []ArtifactEntry
	Screenshots   []string
	HTMLSnapshots []string
	Traces        []string
}

func CollectRunArtifacts(runDir string) (CollectResult, error) {
	result := CollectResult{}
	err := filepath.WalkDir(runDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(runDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch rel {
		case "manifest.json", "failures.json", "report.md", "index.html":
			return nil
		}
		entry, ok := EntryFromRunRelativePath(rel)
		if !ok {
			return nil
		}
		result.Entries = append(result.Entries, entry)
		switch entry.Kind {
		case ArtifactKindScreenshot:
			result.Screenshots = append(result.Screenshots, path)
		case ArtifactKindHTML:
			result.HTMLSnapshots = append(result.HTMLSnapshots, path)
		case ArtifactKindTrace:
			result.Traces = append(result.Traces, path)
		}
		return nil
	})
	if err != nil {
		return CollectResult{}, err
	}
	sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].Path < result.Entries[j].Path })
	sort.Strings(result.Screenshots)
	sort.Strings(result.HTMLSnapshots)
	sort.Strings(result.Traces)
	return result, nil
}

func EntryFromRunRelativePath(rel string) (ArtifactEntry, bool) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	parts := strings.Split(rel, "/")
	if len(parts) != 4 {
		return ArtifactEntry{}, false
	}
	feature, scenario, viewport, file := parts[0], parts[1], parts[2], parts[3]
	label := strings.TrimSuffix(file, filepath.Ext(file))
	if feature == "" || scenario == "" || viewport == "" || label == "" {
		return ArtifactEntry{}, false
	}

	kind := ArtifactKind("")
	switch strings.ToLower(filepath.Ext(file)) {
	case ".png", ".jpg", ".jpeg", ".webp":
		kind = ArtifactKindScreenshot
	case ".html":
		kind = ArtifactKindHTML
	case ".zip":
		kind = ArtifactKindTrace
	default:
		return ArtifactEntry{}, false
	}

	return ArtifactEntry{
		FeatureSlug:  feature,
		ScenarioSlug: scenario,
		Viewport:     viewport,
		Label:        label,
		Kind:         kind,
		Path:         rel,
	}, true
}
