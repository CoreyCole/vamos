package markdown

import (
	"errors"
	"path"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/services/commentui"
)

type ThreadArtifactEntry struct {
	Name  string
	Path  string
	IsDir bool
	Href  string
}

type ThreadArtifactBrowserArgs struct {
	ThreadID string
	DocPath  string
	Parent   string
	Entries  []ThreadArtifactEntry
}

func ThreadArtifactHref(_, docPath string) string {
	canonical, err := CanonicalThoughtsDocPath(docPath)
	if err != nil {
		return "/thoughts/"
	}
	return ThoughtsDocURL(canonical, "")
}

func (s *Service) threadArtifactBrowser(
	c echo.Context,
	threadID, docPath string,
) (ThreadArtifactBrowserArgs, error) {
	if _, err := s.workbenchThreadsRenderer.ResolveSharedThreadPlanDir(
		c.Request().Context(), threadID,
	); err != nil {
		return ThreadArtifactBrowserArgs{}, errors.New("thread artifact is unavailable")
	}
	if docPath != "" {
		var err error
		docPath, err = CanonicalThoughtsDocPath(docPath)
		if err != nil {
			return ThreadArtifactBrowserArgs{}, err
		}
	}
	dir := path.Dir(docPath)
	if docPath == "" {
		dir = ""
	}
	listing, err := s.GetDirectoryListing(dir)
	if err != nil {
		return ThreadArtifactBrowserArgs{}, err
	}
	entries := make([]ThreadArtifactEntry, 0, len(listing.Items))
	for _, item := range listing.Items {
		entries = append(entries, ThreadArtifactEntry{
			Name: item.Name, Path: item.Path, IsDir: item.IsDir,
			Href: DirectoryItemHref(item, ThoughtsWorkbenchLinkState{}),
		})
	}
	parent := ""
	if listing.Path != "" {
		parent = ThoughtsDirURL(listing.Parent)
	}
	return ThreadArtifactBrowserArgs{
		ThreadID: threadID, DocPath: docPath, Parent: parent, Entries: entries,
	}, nil
}

func (s *Service) threadArtifactPane(
	c echo.Context,
	threadID, rawDoc string,
) (templ.Component, error) {
	artifact, _, err := s.threadArtifactAndComments(c, threadID, rawDoc)
	return artifact, err
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
		return WorkbenchUnavailable(
				"The thread artifact is unavailable.",
			), WorkbenchUnavailable(
				"Comments are unavailable for this artifact.",
			), nil
	}
	content, page, directory := s.artifactContent(c, doc, explicit || !hasArtifact)
	if directory {
		return ThreadArtifactPane(browser, content),
			WorkbenchUnavailable("Comments are unavailable for directories."), nil
	}
	if page == nil {
		return ThreadArtifactPane(browser, content),
			WorkbenchUnavailable("Comments are unavailable for this artifact."), nil
	}
	userEmail, _ := c.Get("user_email").(string)
	threads := []commentui.CommentThreadView{}
	if response, err := s.commentService.GetCommentsForScopeInternal(
		c.Request().Context(),
		doc,
	); err == nil {
		page.Comments = response
		threads = thoughtsCommentThreads(response.Comments)
	}
	page.CommentUI = s.buildCommentUI(page, userEmail, threads)
	page.CommentUI.HiddenFields["workbench_v2"] = "1"
	page.ViewerArgs.BodyComponent = commentComponentForMode(
		page.ViewerArgs.CommentMode,
		page.CommentUI,
		page.ViewerArgs.BodyComponent,
	)
	content = DocumentPanel(BuildDocumentPanelArgs(page))
	return ThreadArtifactPane(browser, content),
		commentui.CommentsContextPanel(
			commentui.BuildCommentsPanelArgs(page.CommentUI, ""),
		), nil
}
