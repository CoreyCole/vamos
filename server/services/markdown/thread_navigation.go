package markdown

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/CoreyCole/vamos/server/services/commentui"
)

type ThreadArtifactEntry struct {
	Name           string
	Path           string
	Href           string
	Endpoint       string
	BrowseHref     string
	BrowseEndpoint string
	TargetID       string
	IsDir          bool
	IsActive       bool
	IsExpanded     bool
	IsLoaded       bool
	Children       []ThreadArtifactEntry
}

type ThreadArtifactBrowserArgs struct {
	ThreadID       string
	DocPath        string
	DirectoryPath  string
	ParentHref     string
	ParentEndpoint string
	Entries        []ThreadArtifactEntry
	HeaderActions  templ.Component
}

func threadArtifactQuery(
	docPath, directoryPath string,
	includeDirectory bool,
) (string, bool) {
	canonicalDoc, err := CanonicalThoughtsDocPath(docPath)
	if err != nil {
		return "", false
	}
	query := url.Values{"artifact": {"thoughts/" + canonicalDoc}}
	if includeDirectory {
		canonicalDir, err := CanonicalThoughtsDirPath(directoryPath)
		if err != nil {
			return "", false
		}
		query.Set("artifact_dir", threadArtifactDirectoryIdentity(canonicalDir))
	}
	return query.Encode(), true
}

func ThreadArtifactHref(threadID, docPath string) string {
	query, ok := threadArtifactQuery(docPath, "", false)
	if !ok {
		return threadArtifactPageBase(threadID)
	}
	return threadArtifactPageBase(threadID) + "?" + query
}

func ThreadArtifactHrefAtDirectory(threadID, docPath, directoryPath string) string {
	query, ok := threadArtifactQuery(docPath, directoryPath, true)
	if !ok {
		return threadArtifactPageBase(threadID)
	}
	return threadArtifactPageBase(threadID) + "?" + query
}

func ThreadArtifactBrowserEndpoint(threadID, docPath, directoryPath string) string {
	query, ok := threadArtifactQuery(docPath, directoryPath, true)
	if !ok {
		return ""
	}
	return threadArtifactActionBase(threadID, "artifact-browser") + "?" + query
}

func ThreadArtifactDirectoryEndpoint(threadID, dirPath string) string {
	canonical, err := CanonicalThoughtsDirPath(dirPath)
	if err != nil {
		return ""
	}
	query := url.Values{"directory": {threadArtifactDirectoryIdentity(canonical)}}
	return threadArtifactActionBase(threadID, "artifact-directory") + "?" + query.Encode()
}

func ThreadArtifactDirectoryEndpointForBrowser(
	threadID, dirPath, docPath, browserDirectory string,
) string {
	canonicalDir, err := CanonicalThoughtsDirPath(dirPath)
	if err != nil {
		return ""
	}
	query, ok := threadArtifactQuery(docPath, browserDirectory, true)
	if !ok {
		return ""
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return ""
	}
	values.Set("directory", threadArtifactDirectoryIdentity(canonicalDir))
	return threadArtifactActionBase(
		threadID,
		"artifact-directory",
	) + "?" + values.Encode()
}

func threadArtifactDirectoryIdentity(directoryPath string) string {
	if directoryPath == "" {
		return "thoughts"
	}
	return "thoughts/" + directoryPath
}

func threadArtifactPageBase(threadID string) string {
	if strings.TrimSpace(threadID) == "" {
		return "/threads"
	}
	return "/threads/" + url.PathEscape(strings.TrimSpace(threadID))
}

func threadArtifactActionBase(threadID, action string) string {
	return threadArtifactPageBase(threadID) + "/" + action
}

func threadArtifactDirectoryID(dirPath string) string {
	sum := sha256.Sum256([]byte(dirPath))
	return "thread-artifact-directory-" + hex.EncodeToString(sum[:8])
}

func threadArtifactChildrenID(entry ThreadArtifactEntry) string {
	return entry.TargetID + "-children"
}

func threadArtifactLoaded(loaded bool) string {
	if loaded {
		return "true"
	}
	return "false"
}

func threadArtifactBrowserClickAction() string {
	return "if (!$_threadArtifactLoading && evt.button === 0 && !evt.metaKey && !evt.ctrlKey && !evt.shiftKey && !evt.altKey) { evt.preventDefault(); @get(el.dataset.artifactEndpoint) }"
}

func threadArtifactDirectoryToggleAction() string {
	return "if (el.open && el.dataset.loaded !== 'true') { @get(el.dataset.artifactEndpoint) }"
}

func (s *Service) validateThreadArtifactBrowserThread(
	c echo.Context,
	threadID string,
) error {
	if strings.TrimSpace(threadID) == "" {
		return nil
	}
	if s.workbenchThreadsRenderer == nil {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"thread renderer is not configured",
		)
	}
	if _, err := s.workbenchThreadsRenderer.ResolveSharedThreadPlanDir(
		c.Request().Context(),
		threadID,
	); errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusNotFound, "thread not found")
	} else if err != nil {
		return err
	}
	return nil
}

func threadArtifactBrowserDirectory(c echo.Context, docPath string) (string, error) {
	if c.Request().URL.Query().Has("artifact_dir") {
		return CanonicalThoughtsDirPath(c.QueryParam("artifact_dir"))
	}
	if docPath == "" {
		return "", nil
	}
	return CanonicalThoughtsDirPath(path.Dir(docPath))
}

func (s *Service) threadArtifactBrowser(
	c echo.Context,
	threadID, docPath string,
) (ThreadArtifactBrowserArgs, error) {
	if err := s.validateThreadArtifactBrowserThread(c, threadID); err != nil {
		return ThreadArtifactBrowserArgs{}, err
	}
	if docPath != "" {
		canonical, _, err := optionalThreadArtifact(docPath)
		if err != nil {
			return ThreadArtifactBrowserArgs{}, err
		}
		docPath = canonical
	}
	directoryPath, err := threadArtifactBrowserDirectory(c, docPath)
	if err != nil {
		return ThreadArtifactBrowserArgs{}, err
	}
	listing, err := s.GetDirectoryListing(directoryPath)
	if err != nil {
		return ThreadArtifactBrowserArgs{}, err
	}
	entries, err := s.buildThreadArtifactEntries(
		threadID,
		listing,
		docPath,
		directoryPath,
	)
	if err != nil {
		return ThreadArtifactBrowserArgs{}, err
	}
	args := ThreadArtifactBrowserArgs{
		ThreadID:      threadID,
		DocPath:       docPath,
		DirectoryPath: directoryPath,
		Entries:       entries,
	}
	if directoryPath != "" && docPath != "" {
		parent := path.Dir(directoryPath)
		args.ParentHref = ThreadArtifactHrefAtDirectory(threadID, docPath, parent)
		args.ParentEndpoint = ThreadArtifactBrowserEndpoint(threadID, docPath, parent)
	}
	return args, nil
}

func (s *Service) buildThreadArtifactEntries(
	threadID string,
	listing *DirectoryArgs,
	activePath, browserDirectory string,
) ([]ThreadArtifactEntry, error) {
	entries := make([]ThreadArtifactEntry, 0, len(listing.Items))
	for _, item := range listing.Items {
		if item.IsDir {
			dirPath, err := CanonicalThoughtsDirPath(item.Path)
			if err != nil {
				return nil, err
			}
			entry := ThreadArtifactEntry{
				Name: item.Name,
				Path: dirPath,
				Endpoint: ThreadArtifactDirectoryEndpointForBrowser(
					threadID,
					dirPath,
					activePath,
					browserDirectory,
				),
				BrowseHref: ThreadArtifactHrefAtDirectory(
					threadID,
					activePath,
					dirPath,
				),
				BrowseEndpoint: ThreadArtifactBrowserEndpoint(
					threadID,
					activePath,
					dirPath,
				),
				TargetID: threadArtifactDirectoryID(dirPath),
				IsDir:    true,
				IsActive: activePath == dirPath,
				IsExpanded: activePath == dirPath ||
					strings.HasPrefix(activePath, dirPath+"/"),
			}
			if entry.IsExpanded {
				children, err := s.GetDirectoryListing(dirPath)
				if err != nil {
					return nil, err
				}
				entry.Children, err = s.buildThreadArtifactEntries(
					threadID,
					children,
					activePath,
					browserDirectory,
				)
				if err != nil {
					return nil, err
				}
				entry.IsLoaded = true
			}
			entries = append(entries, entry)
			continue
		}
		docPath, err := CanonicalThoughtsDocPath(item.Path)
		if err != nil {
			return nil, err
		}
		entries = append(entries, ThreadArtifactEntry{
			Name:     item.Name,
			Path:     docPath,
			Href:     ThreadArtifactHrefAtDirectory(threadID, docPath, browserDirectory),
			IsActive: activePath == docPath,
		})
	}
	return entries, nil
}

func (s *Service) loadedThreadArtifactDirectory(
	c echo.Context,
	threadID, rawDir string,
) (ThreadArtifactEntry, error) {
	if err := s.validateThreadArtifactBrowserThread(c, threadID); err != nil {
		return ThreadArtifactEntry{}, err
	}
	dirPath, err := CanonicalThoughtsDirPath(rawDir)
	if err != nil {
		return ThreadArtifactEntry{}, err
	}
	activePath, _, err := optionalThreadArtifact(c.QueryParam("artifact"))
	if err != nil {
		return ThreadArtifactEntry{}, err
	}
	browserDirectory, err := threadArtifactBrowserDirectory(c, activePath)
	if err != nil {
		return ThreadArtifactEntry{}, err
	}
	listing, err := s.GetDirectoryListing(dirPath)
	if err != nil {
		return ThreadArtifactEntry{}, err
	}
	children, err := s.buildThreadArtifactEntries(
		threadID,
		listing,
		activePath,
		browserDirectory,
	)
	if err != nil {
		return ThreadArtifactEntry{}, err
	}
	name := path.Base(dirPath)
	if dirPath == "" {
		name = "thoughts"
	}
	return ThreadArtifactEntry{
		Name: name,
		Path: dirPath,
		Endpoint: ThreadArtifactDirectoryEndpointForBrowser(
			threadID,
			dirPath,
			activePath,
			browserDirectory,
		),
		BrowseHref:     ThreadArtifactHrefAtDirectory(threadID, activePath, dirPath),
		BrowseEndpoint: ThreadArtifactBrowserEndpoint(threadID, activePath, dirPath),
		TargetID:       threadArtifactDirectoryID(dirPath),
		IsDir:          true,
		IsExpanded:     true,
		IsLoaded:       true,
		Children:       children,
	}, nil
}

func (s *Service) threadArtifactAndComments(
	c echo.Context,
	threadID, rawDoc string,
) (templ.Component, templ.Component, error) {
	hasArtifact := c.Request().URL.Query().Has("artifact")
	doc, explicit, err := s.resolveThreadArtifact(
		c.Request().Context(), threadID, rawDoc, hasArtifact,
	)
	if err != nil {
		return nil, nil, err
	}
	browser, err := s.threadArtifactBrowser(c, threadID, doc)
	if err != nil {
		return nil, nil, err
	}
	content, page, directory := s.artifactContent(c, doc, explicit || !hasArtifact)
	if directory {
		browser.HeaderActions = BuildThreadArtifactHeaderActions(nil, browser.DocPath)
		return ThreadArtifactPane(
			browser,
			WorkbenchUnavailable("Select a file from the artifact browser."),
		), WorkbenchUnavailable("Comments are unavailable for directories."), nil
	}
	if page == nil {
		browser.HeaderActions = BuildThreadArtifactHeaderActions(nil, browser.DocPath)
		return ThreadArtifactPane(browser, content),
			WorkbenchUnavailable("Comments are unavailable for this artifact."), nil
	}
	userEmail, _ := c.Get("user_email").(string)
	threads := []commentui.CommentThreadView{}
	if s.commentService != nil {
		if response, err := s.commentService.GetCommentsForScopeInternal(
			c.Request().Context(),
			page.FilePath,
		); err == nil {
			page.Comments = response
			threads = thoughtsCommentThreads(response.Comments)
		}
	}
	page.CommentUI = s.buildCommentUI(page, userEmail, threads)
	page.CommentUI.HiddenFields["workbench_v2"] = "1"
	page.ViewerArgs.BodyComponent = commentComponentForMode(
		page.ViewerArgs.CommentMode,
		page.CommentUI,
		page.ViewerArgs.BodyComponent,
	)
	panelArgs := BuildDocumentPanelArgs(page)
	browser.HeaderActions = BuildThreadArtifactHeaderActions(page, browser.DocPath)
	panelArgs.Document.WorkbenchActions = nil
	content = DocumentPanel(panelArgs)
	return ThreadArtifactPane(browser, content),
		commentui.CommentsContextPanel(
			commentui.BuildCommentsPanelArgs(page.CommentUI, ""),
		), nil
}

func (s *Service) HandleThreadArtifactBrowser(c echo.Context) error {
	threadID := strings.TrimSpace(c.Param("threadID"))
	if err := s.validateThreadArtifactBrowserThread(c, threadID); err != nil {
		return err
	}
	rawArtifact := c.QueryParam("artifact")
	if strings.TrimSpace(rawArtifact) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "artifact is required")
	}
	artifactPath, _, err := optionalThreadArtifact(rawArtifact)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid artifact")
	}
	if _, err := s.RenderThoughtsDocument(
		c.Request().Context(),
		artifactPath,
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "artifact must be a file")
	}
	browser, err := s.threadArtifactBrowser(c, threadID, artifactPath)
	if err != nil {
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) {
			return httpErr
		}
		return echo.NewHTTPError(http.StatusBadRequest, "invalid artifact directory")
	}
	pageURL := ThreadArtifactHrefAtDirectory(
		threadID,
		artifactPath,
		browser.DirectoryPath,
	)
	encodedURL, err := json.Marshal(pageURL)
	if err != nil {
		return err
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	if err := sse.PatchElementTempl(
		ThreadArtifactBrowser(browser),
		datastar.WithSelectorID("thread-artifact-browser"),
		datastar.WithModeOuter(),
	); err != nil {
		return err
	}
	return sse.ExecuteScript(
		"window.history.pushState({}, '', " + string(encodedURL) + ")",
	)
}

func (s *Service) HandleThreadArtifactDirectory(c echo.Context) error {
	threadID := strings.TrimSpace(c.Param("threadID"))
	rawDir := c.QueryParam("directory")
	if !c.Request().URL.Query().Has("directory") {
		return echo.NewHTTPError(http.StatusBadRequest, "directory is required")
	}
	entry, err := s.loadedThreadArtifactDirectory(c, threadID, rawDir)
	if err != nil {
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) {
			return httpErr
		}
		return echo.NewHTTPError(http.StatusBadRequest, "invalid directory")
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return sse.PatchElementTempl(ThreadArtifactDirectory(entry))
}
