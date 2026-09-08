package markdown

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"
)

type ArtifactSearchHit struct {
	Name  string
	Path  string
	Href  string
	IsDir bool
}

type ThreadArtifactBrowserResultsArgs struct {
	Query     string
	Entries   []ThreadArtifactEntry
	Directory []ArtifactSearchHit
	Global    []ArtifactSearchHit
}

func artifactSearchDirSuffix(isDir bool) string {
	if isDir {
		return "/"
	}
	return ""
}

func artifactSearchHitFromEntry(entry ThreadArtifactEntry) ArtifactSearchHit {
	href := entry.Href
	if entry.IsDir {
		href = entry.BrowseHref
	}
	return ArtifactSearchHit{
		Name:  entry.Name,
		Path:  entry.Path,
		Href:  href,
		IsDir: entry.IsDir,
	}
}

func artifactSearchQueryMatch(query, name, itemPath string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	for _, candidate := range []string{name, path.Base(filepath.ToSlash(itemPath))} {
		n := strings.ToLower(strings.TrimSpace(candidate))
		if n == "" {
			continue
		}
		if strings.Contains(n, q) {
			return true
		}
		ext := strings.ToLower(filepath.Ext(n))
		if ext != "" && strings.Contains(strings.TrimSuffix(n, ext), q) {
			return true
		}
	}
	return false
}

func (s *Service) HandleThoughtsArtifactSearch(c echo.Context) error {
	query := strings.TrimSpace(c.QueryParam("q"))
	selectedDoc, err := s.thoughtsSelectedDoc(c)
	if err != nil {
		return err
	}
	threadID := strings.TrimSpace(c.QueryParam("thread"))
	var browser ThreadArtifactBrowserArgs
	if selectedDoc != "" {
		browser, err = s.threadArtifactBrowser(c, threadID, selectedDoc)
	} else {
		dirPath, dirErr := threadArtifactBrowserDirectory(c, "")
		if dirErr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid artifact directory")
		}
		browser, err = s.thoughtsDirectoryArtifactBrowser(c, dirPath)
	}
	if err != nil {
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) {
			return httpErr
		}
		return echo.NewHTTPError(http.StatusBadRequest, "invalid artifact directory")
	}
	if threadID == "" {
		browser = remapThreadArtifactBrowserForThoughts(browser, selectedDoc)
	}
	results := ThreadArtifactBrowserResultsArgs{
		Query:   query,
		Entries: browser.Entries,
	}
	if query != "" {
		results.Directory, results.Global = s.artifactSearchSections(
			browser.Entries,
			query,
			selectedDoc,
			threadID,
		)
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return sse.PatchElementTempl(
		ThreadArtifactBrowserResults(results),
		datastar.WithSelectorID("thread-artifact-browser-results"),
		datastar.WithModeOuter(),
	)
}

func (s *Service) artifactSearchSections(
	cwdEntries []ThreadArtifactEntry,
	query, selectedDoc, threadID string,
) (directory, global []ArtifactSearchHit) {
	skip := make(map[string]struct{}, len(cwdEntries))
	for _, entry := range cwdEntries {
		skip[entry.Path] = struct{}{}
		if artifactSearchQueryMatch(query, entry.Name, entry.Path) {
			directory = append(directory, artifactSearchHitFromEntry(entry))
		}
	}
	global = s.searchThoughtsGlobal(query, selectedDoc, threadID, skip)
	return directory, global
}

func (s *Service) searchThoughtsGlobal(
	query, selectedDoc, threadID string,
	skip map[string]struct{},
) []ArtifactSearchHit {
	if s == nil || strings.TrimSpace(s.basePath) == "" {
		return nil
	}
	hits := make([]ArtifactSearchHit, 0, artifactSearchGlobalLimit)
	_ = filepath.WalkDir(s.basePath, func(abs string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if abs != s.basePath && d.IsDir() &&
			(strings.HasPrefix(name, ".") || skipWorkspaceDocTreeEntry(name, true)) {
			return filepath.SkipDir
		}
		if abs != s.basePath && strings.HasPrefix(name, ".") {
			return nil
		}
		rel, relErr := filepath.Rel(s.basePath, abs)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || rel == "" {
			return nil
		}
		if _, dup := skip[rel]; dup {
			return nil
		}
		if d.IsDir() {
			if !artifactSearchQueryMatch(query, name, rel) {
				return nil
			}
			hits = append(hits, ArtifactSearchHit{
				Name:  name,
				Path:  rel,
				Href:  artifactSearchDirHref(selectedDoc, threadID, rel),
				IsDir: true,
			})
		} else {
			if !isThoughtsRenderableFile(name) {
				return nil
			}
			if !artifactSearchQueryMatch(query, name, rel) {
				return nil
			}
			hits = append(hits, ArtifactSearchHit{
				Name: displayDocumentName(name),
				Path: rel,
				Href: artifactSearchFileHref(threadID, rel),
			})
		}
		if len(hits) >= artifactSearchGlobalLimit*4 {
			return fs.SkipAll
		}
		return nil
	})
	sort.Slice(hits, func(i, j int) bool {
		li := strings.ToLower(hits[i].Name)
		lj := strings.ToLower(hits[j].Name)
		if li != lj {
			return li < lj
		}
		return hits[i].Path < hits[j].Path
	})
	if len(hits) > artifactSearchGlobalLimit {
		hits = hits[:artifactSearchGlobalLimit]
	}
	return hits
}

func artifactSearchFileHref(threadID, docPath string) string {
	if strings.TrimSpace(threadID) != "" {
		return ThreadArtifactHref(threadID, docPath)
	}
	return ThoughtsDocURL(docPath, "")
}

func artifactSearchDirHref(selectedDoc, threadID, dirPath string) string {
	if strings.TrimSpace(threadID) != "" {
		if strings.TrimSpace(selectedDoc) != "" {
			return ThreadArtifactHrefAtDirectory(threadID, selectedDoc, dirPath)
		}
		return ThreadArtifactHrefAtDirectory(threadID, dirPath, dirPath)
	}
	return thoughtsArtifactPageURL(selectedDoc, dirPath)
}
