package markdown

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
)

func (s *Service) savedThreadsWorkbenchConfig(
	c echo.Context,
	userEmail string,
	viewport workbench.ViewportClass,
) *workbench.WorkbenchConfig {
	if s.layoutPrefs == nil || strings.TrimSpace(userEmail) == "" {
		return nil
	}
	config, err := s.layoutPrefs.Get(
		c.Request().Context(),
		userEmail,
		workbench.WorkbenchPageThreads,
		workbench.WorkbenchViewSplit,
		viewport,
	)
	if err != nil {
		return nil
	}
	return config
}

func (s *Service) ResolveThreadArtifact(
	ctx context.Context,
	threadID, rawDoc string,
) (string, error) {
	artifact, _, err := s.resolveThreadArtifact(
		ctx, threadID, rawDoc, strings.TrimSpace(rawDoc) != "",
	)
	if err != nil {
		return "", err
	}
	if _, err := s.GetDirectoryListing(artifact); err == nil {
		return artifact, nil
	}
	if _, err := s.RenderThoughtsDocument(ctx, artifact); err != nil {
		return "", err
	}
	return artifact, nil
}

func (s *Service) resolveThreadArtifact(
	ctx context.Context,
	threadID, rawDoc string,
	hasArtifact bool,
) (string, bool, error) {
	if s.workbenchThreadsRenderer == nil {
		return "", false, errors.New("thread renderer is not configured")
	}
	planDir, err := s.workbenchThreadsRenderer.ResolveSharedThreadPlanDir(ctx, threadID)
	if err != nil || strings.TrimSpace(planDir) == "" {
		return "", false, err
	}
	if hasArtifact {
		artifact, explicit, err := optionalThreadArtifact(rawDoc)
		return artifact, explicit, err
	}
	artifact, err := CanonicalThoughtsDocPath(path.Join(planDir, "design.md"))
	return artifact, false, err
}

func optionalThreadArtifact(raw string) (string, bool, error) {
	trimmed := strings.Trim(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", false, nil
	}
	if trimmed == "thoughts" {
		return "", true, nil
	}
	artifact, err := CanonicalThoughtsDocPath(raw)
	if err == nil {
		return artifact, true, nil
	}
	artifact, dirErr := CanonicalThoughtsDirPath(raw)
	if dirErr != nil {
		return "", false, err
	}
	return artifact, true, nil
}

func (s *Service) artifactContent(
	c echo.Context,
	artifact string,
	explicit bool,
) (templ.Component, *PageArgs, bool) {
	if !explicit {
		return WorkbenchUnavailable("Select a thread to view an artifact."), nil, false
	}
	if directory, err := s.GetDirectoryListing(artifact); err == nil {
		return DirectoryPrimaryPanel(directory), nil, true
	}
	page, err := s.RenderThoughtsDocument(c.Request().Context(), artifact)
	if err != nil {
		return WorkbenchUnavailable("The requested artifact is unavailable."), nil, false
	}
	page.UserEmail, _ = c.Get("user_email").(string)
	return DocumentPanel(BuildDocumentPanelArgs(page)), page, false
}

func (s *Service) indexArtifactComponent(
	c echo.Context,
	artifact string,
	explicit bool,
) templ.Component {
	content, _, _ := s.artifactContent(c, artifact, explicit)
	return content
}

func (s *Service) ServeThreads(c echo.Context) error {
	if s.workbenchThreadsRenderer == nil {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"thread renderer is not configured",
		)
	}
	userEmail, _ := c.Get("user_email").(string)
	artifactPath, hasArtifact, err := optionalThreadArtifact(c.QueryParam("artifact"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	threads, err := s.workbenchThreadsRenderer.RenderWorkbenchThreadList(
		c.Request().Context(), "", artifactPath,
	)
	if err != nil {
		return err
	}
	viewport := viewportClassForRequest(c)
	state, err := workbench.BuildWorkbenchV2State(workbench.WorkbenchV2Args{
		UserEmail:     userEmail,
		ViewportClass: viewport,
		SavedConfig:   s.savedThreadsWorkbenchConfig(c, userEmail, viewport),
		Threads:       threads,
		Chat:          WorkbenchUnavailable("Select a thread to open chat."),
		Artifact:      s.indexArtifactComponent(c, artifactPath, hasArtifact),
		Comments:      WorkbenchUnavailable("Select an artifact to view comments."),
		ThreadsOpen:   true,
		ChatOpen:      false,
		ArtifactOpen:  true,
		CommentsOpen:  false,
	})
	if err != nil {
		return err
	}
	return ThreadWorkbenchPage(
		userEmail,
		state,
	).Render(c.Request().Context(), c.Response().Writer)
}

func (s *Service) ServeThread(c echo.Context) error {
	if s.workbenchThreadsRenderer == nil {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"thread renderer is not configured",
		)
	}
	threadID := strings.TrimSpace(c.Param("threadID"))
	if threadID == "" {
		return echo.NewHTTPError(http.StatusNotFound, "thread not found")
	}
	userEmail, _ := c.Get("user_email").(string)
	threads, err := s.workbenchThreadsRenderer.RenderWorkbenchThreadList(
		c.Request().Context(), threadID, "",
	)
	if errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusNotFound, "thread not found")
	}
	if err != nil {
		return err
	}
	chat, err := s.workbenchThreadsRenderer.RenderSharedThreadChat(
		c.Request().Context(),
		threadID,
		userEmail,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusNotFound, "thread not found")
	}
	if err != nil {
		return err
	}
	artifact, comments, err := s.threadArtifactAndComments(
		c,
		threadID,
		c.QueryParam("artifact"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	viewport := viewportClassForRequest(c)
	state, err := workbench.BuildWorkbenchV2State(workbench.WorkbenchV2Args{
		UserEmail:     userEmail,
		ViewportClass: viewport,
		SavedConfig:   s.savedThreadsWorkbenchConfig(c, userEmail, viewport),
		Threads:       threads,
		Chat:          chat,
		Artifact:      artifact,
		Comments:      comments,
		ThreadsOpen:   true,
		ChatOpen:      true,
		ArtifactOpen:  true,
		CommentsOpen:  false,
	})
	if err != nil {
		return err
	}
	return ThreadWorkbenchPage(
		userEmail,
		state,
	).Render(c.Request().Context(), c.Response().Writer)
}

func (s *Service) threadArtifactComponent(
	c echo.Context,
	threadID, rawDoc string,
) (templ.Component, error) {
	doc, err := s.ResolveThreadArtifact(c.Request().Context(), threadID, rawDoc)
	if err != nil {
		return nil, err
	}
	page, err := s.RenderThoughtsDocument(c.Request().Context(), doc)
	if err != nil {
		return WorkbenchUnavailable("The thread artifact is unavailable."), nil
	}
	page.UserEmail, _ = c.Get("user_email").(string)
	return DocumentPanel(BuildDocumentPanelArgs(page)), nil
}
