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
	Name           string
	Path           string
	Href           string
	Endpoint       string
	BrowseEndpoint string
	TargetID       string
	IsDir          bool
}

type ThreadArtifactBrowserResultsArgs struct {
	Query     string
	Entries   []ThreadArtifactEntry
	Directory []ArtifactSearchHit
	Global    []ArtifactSearchHit
}

func artifactSearchQueryMatch(query, name, itemPath string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	needles := []string{q}
	if ext := filepath.Ext(q); ext != "" {
		if base := strings.TrimSuffix(q, ext); base != "" {
			needles = append(needles, base)
		}
	}
	for _, candidate := range []string{name, path.Base(filepath.ToSlash(itemPath))} {
		n := strings.ToLower(strings.TrimSpace(candidate))
		if n == "" {
			continue
		}
		hay := []string{n}
		if ext := filepath.Ext(n); ext != "" {
			if base := strings.TrimSuffix(n, ext); base != "" {
				hay = append(hay, base)
			}
		}
		for _, h := range hay {
			for _, needle := range needles {
				if strings.Contains(h, needle) {
					return true
				}
			}
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
			browser.DirectoryPath,
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

func artifactSearchRelPrefix(dir string) string {
	return strings.Trim(filepath.ToSlash(dir), "/")
}

func artifactSearchUnderDir(rel, dir string) bool {
	if dir == "" {
		return true
	}
	return rel == dir || strings.HasPrefix(rel, dir+"/")
}

func (s *Service) artifactSearchSections(
	cwdDir, query, selectedDoc, threadID string,
) (directory, global []ArtifactSearchHit) {
	cwdDir = artifactSearchRelPrefix(cwdDir)
	directory = s.searchThoughtsNames(query, selectedDoc, threadID, cwdDir, "")
	if cwdDir == "" {
		return directory, nil
	}
	global = s.searchThoughtsNames(query, selectedDoc, threadID, "", cwdDir)
	return directory, global
}

func (s *Service) searchThoughtsNames(
	query, selectedDoc, threadID, startRel, excludeRel string,
) []ArtifactSearchHit {
	if s == nil || strings.TrimSpace(s.basePath) == "" {
		return nil
	}
	start := s.basePath
	if startRel != "" {
		start = filepath.Join(s.basePath, filepath.FromSlash(startRel))
	}
	hits := make([]ArtifactSearchHit, 0, artifactSearchGlobalLimit)
	_ = filepath.WalkDir(start, func(abs string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if abs != start && d.IsDir() &&
			(strings.HasPrefix(name, ".") || skipWorkspaceDocTreeEntry(name, true)) {
			return filepath.SkipDir
		}
		if abs != start && strings.HasPrefix(name, ".") {
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
		if excludeRel != "" && artifactSearchUnderDir(rel, excludeRel) {
			if d.IsDir() && rel == excludeRel {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if !artifactSearchQueryMatch(query, name, rel) {
				return nil
			}
			hits = append(hits, artifactSearchDirHit(name, rel, selectedDoc, threadID))
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

func artifactSearchDirHit(name, rel, selectedDoc, threadID string) ArtifactSearchHit {
	hit := ArtifactSearchHit{
		Name:     name,
		Path:     rel,
		Href:     artifactSearchDirHref(selectedDoc, threadID, rel),
		TargetID: threadArtifactDirectoryID(rel),
		IsDir:    true,
	}
	if strings.TrimSpace(threadID) != "" {
		hit.Endpoint = ThreadArtifactDirectoryEndpointForBrowser(
			threadID, rel, selectedDoc, rel,
		)
		hit.BrowseEndpoint = ThreadArtifactBrowserEndpoint(threadID, selectedDoc, rel)
		return hit
	}
	hit.Endpoint = thoughtsArtifactDirectoryEndpoint(rel, selectedDoc, rel)
	hit.BrowseEndpoint = thoughtsArtifactBrowserEndpoint(selectedDoc, rel)
	return hit
}

func artifactSearchHitEntry(hit ArtifactSearchHit) ThreadArtifactEntry {
	return ThreadArtifactEntry{
		Name:           hit.Name,
		Path:           hit.Path,
		Endpoint:       hit.Endpoint,
		BrowseHref:     hit.Href,
		BrowseEndpoint: hit.BrowseEndpoint,
		TargetID:       hit.TargetID,
		IsDir:          true,
	}
}
