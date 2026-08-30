package markdown

import (
	"errors"
	"path"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
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
	docPath, err := CanonicalThoughtsDocPath(docPath)
	if err != nil {
		return ThreadArtifactBrowserArgs{}, err
	}
	dir := path.Dir(docPath)
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
	doc, err := s.ResolveThreadArtifact(c.Request().Context(), threadID, rawDoc)
	if err != nil {
		return nil, err
	}
	browser, err := s.threadArtifactBrowser(c, threadID, doc)
	if err != nil {
		return WorkbenchUnavailable("The thread artifact is unavailable."), nil
	}
	if directory, err := s.GetDirectoryListing(doc); err == nil {
		return ThreadArtifactPane(browser, DirectoryPrimaryPanel(directory)), nil
	}
	page, err := s.RenderThoughtsDocument(c.Request().Context(), doc)
	if err != nil {
		return ThreadArtifactPane(
			browser,
			WorkbenchUnavailable("The thread artifact is unavailable."),
		), nil
	}
	page.UserEmail, _ = c.Get("user_email").(string)
	return ThreadArtifactPane(browser, DocumentPanel(BuildDocumentPanelArgs(page))), nil
}
