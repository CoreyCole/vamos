package comments

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/commentui"
)

type commentFormData struct {
	FilePath        string
	CommentText     string
	SelectedText    string
	SectionID       string
	HeadingHint     string
	StartLine       int
	StartColumn     int
	EndLine         int
	EndColumn       int
	TargetChrome    commentui.CommentTargetChrome
	SelectionPrefix string
}

func parseCommentForm(c echo.Context) commentFormData {
	documentPath := c.FormValue("doc_path")
	if strings.TrimSpace(documentPath) == "" {
		documentPath = c.FormValue("file_path")
	}
	return commentFormData{
		FilePath:        documentPath,
		CommentText:     c.FormValue("comment_text"),
		SelectedText:    c.FormValue("selected_text"),
		SectionID:       normalizeSectionID(c.FormValue("section_hint")),
		HeadingHint:     c.FormValue("heading_hint"),
		StartLine:       parseFormInt(c, "start_line"),
		StartColumn:     parseFormInt(c, "start_column"),
		EndLine:         parseFormInt(c, "end_line"),
		EndColumn:       parseFormInt(c, "end_column"),
		TargetChrome:    commentui.CommentTargetChrome(c.FormValue("comment_target_chrome")),
		SelectionPrefix: c.FormValue("comment_selection_prefix"),
	}
}

type thoughtsCommentTargetOptions struct {
	Chrome          commentui.CommentTargetChrome
	SelectionPrefix string
}

func parseFormInt(c echo.Context, name string) int {
	value := strings.TrimSpace(c.FormValue(name))
	if value == "" {
		return 0
	}
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func normalizeSectionID(sectionID string) string {
	sectionID = strings.TrimSpace(sectionID)
	if sectionID == "" {
		return "document"
	}
	return sectionID
}

func thoughtsCommentRoutes() commentui.CommentRoutes {
	return commentui.CommentRoutes{
		Show:   "/forms/comments/show",
		Create: "/forms/comments",
		Cancel: "/forms/comments/cancel",
		Expand: "/forms/comments/expand",
		Reply: func(string) string {
			return "/forms/replies"
		},
		Resolve: func(string) string {
			return "/forms/resolve"
		},
	}
}

func (s *Service) thoughtsCommentThreads(items []CommentWithReplies) []commentui.CommentThreadView {
	sources := make([]commentui.ThreadSource, 0, len(items))
	for _, item := range items {
		sectionID := normalizeSectionID(item.Comment.SectionHint.String)
		if !item.Comment.SectionHint.Valid {
			sectionID = "document"
		}
		headingHint := ""
		if item.Comment.HeadingHint.Valid {
			headingHint = item.Comment.HeadingHint.String
		}
		replies := make([]commentui.ReplySource, 0, len(item.Replies))
		for _, reply := range item.Replies {
			replies = append(replies, commentui.ReplySource{
				AuthorEmail: reply.UserEmail,
				ActorLabel:  s.commentDisplayName(reply.UserEmail),
				CreatedAt:   reply.CreatedAt,
				Body:        reply.ReplyText,
			})
		}
		sources = append(sources, commentui.ThreadSource{
			ID:           item.Comment.ID,
			AuthorEmail:  item.Comment.UserEmail,
			ActorLabel:   s.commentDisplayName(item.Comment.UserEmail),
			CreatedAt:    item.Comment.CreatedAt,
			Body:         item.Comment.CommentText,
			SelectedText: item.Comment.SelectedText,
			SectionID:    sectionID,
			HeadingHint:  headingHint,
			Resolved:     item.Comment.Resolved,
			Replies:      replies,
			HiddenFields: map[string]string{
				"comment_id": item.Comment.ID,
				"doc_path":   item.Comment.DocPath,
			},
		})
	}
	return commentui.BuildThreadViews(sources)
}

func (s *Service) thoughtsCommentThreadsWithSectionTitles(
	filePath string,
	comments []CommentWithReplies,
) []commentui.CommentThreadView {
	threads := s.thoughtsCommentThreads(comments)
	titles := s.sectionTitlesForFile(filePath)
	for i := range threads {
		if strings.TrimSpace(threads[i].HeadingHint) == "" {
			threads[i].HeadingHint = titles[normalizeSectionID(threads[i].SectionID)]
		}
	}
	return threads
}

func (s *Service) thoughtsCommentTarget(
	filePath, sectionID, headingHint, userEmail string,
	comments []CommentWithReplies,
	options ...thoughtsCommentTargetOptions,
) commentui.CommentTargetView {
	sectionID = normalizeSectionID(sectionID)
	prefix := commentui.SafeCommentTargetSlug("thoughts", filePath)
	hiddenFields := map[string]string{"doc_path": filePath}
	var opts thoughtsCommentTargetOptions
	if len(options) > 0 {
		opts = options[0]
		if opts.Chrome != "" {
			hiddenFields["comment_target_chrome"] = string(opts.Chrome)
		}
		if opts.SelectionPrefix != "" {
			hiddenFields["comment_selection_prefix"] = opts.SelectionPrefix
		}
	}
	target := commentui.BuildTargetView(commentui.TargetInput{
		Surface:      commentui.CommentSurfaceThoughts,
		IDPrefix:     prefix,
		DocPath:      filePath,
		SectionID:    sectionID,
		HeadingHint:  headingHint,
		UserEmail:    userEmail,
		Threads:      s.thoughtsCommentThreads(comments),
		Routes:       thoughtsCommentRoutes(),
		HiddenFields: hiddenFields,
	})
	target.Chrome = opts.Chrome
	target.SelectionSignalPrefix = opts.SelectionPrefix
	return target
}

func patchThoughtsCommentTarget(
	sse *datastar.ServerSentEventGenerator,
	target commentui.CommentTargetView,
) error {
	return sse.PatchElementTempl(
		commentui.CommentTarget(target),
		datastar.WithSelectorID(target.ID),
	)
}

func patchThoughtsCommentTargetWithForm(
	sse *datastar.ServerSentEventGenerator,
	target commentui.CommentTargetView,
	data commentFormData,
	errMsg string,
) error {
	return sse.PatchElementTempl(
		commentui.CommentTargetWithForm(target, commentui.CommentFormView{
			ID:           "comment-" + target.SectionID,
			Target:       target,
			SelectedText: data.SelectedText,
			Error:        errMsg,
		}),
		datastar.WithSelectorID(target.ID),
	)
}

func patchOpenCommentsSignal(sse *datastar.ServerSentEventGenerator) error {
	return sse.MarshalAndPatchSignals(map[string]any{
		"rightRailActiveTab": "comments",
		"workbench": map[string]any{
			"focused": false,
			"regions": map[string]any{
				"docWorkbenchRight": map[string]any{"visible": true},
			},
		},
	})
}

func (s *Service) patchThoughtsCommentsPanel(
	sse *datastar.ServerSentEventGenerator,
	filePath, userEmail, activeSectionID, activeSectionLabel string,
	items []CommentWithReplies,
) error {
	args := commentui.CommentableMarkdownArgs{
		Surface:      commentui.CommentSurfaceThoughts,
		IDPrefix:     commentui.SafeCommentTargetSlug("thoughts", filePath),
		DocPath:      filePath,
		Comments:     s.thoughtsCommentThreadsWithSectionTitles(filePath, items),
		Routes:       thoughtsCommentRoutes(),
		HiddenFields: map[string]string{"doc_path": filePath, "context_panel": "1"},
		UserEmail:    userEmail,
	}
	panelArgs := commentui.BuildCommentsPanelArgs(args, activeSectionID)
	if label := strings.TrimSpace(activeSectionLabel); label != "" {
		panelArgs.ActiveSectionLabel = label
	}
	return sse.PatchElementTempl(
		commentui.CommentsContextPanel(panelArgs),
		datastar.WithSelectorID(commentui.CommentsContextPanelID),
	)
}

// HandleCancelCommentForm handles canceling the comment form
func (s *Service) HandleCancelCommentForm(c echo.Context) error {
	data := parseCommentForm(c)
	userEmail, _ := c.Get("user_email").(string)

	response, err := s.GetCommentsForFileInternal(c.Request().Context(), data.FilePath)
	if err != nil {
		c.Logger().Errorf("Failed to fetch comments: %v", err)
		response = &GetCommentsResponse{Comments: []CommentWithReplies{}}
	}

	sectionComments := filterCommentsBySection(response.Comments, data.SectionID)
	target := s.thoughtsCommentTarget(
		data.FilePath,
		data.SectionID,
		data.HeadingHint,
		userEmail,
		sectionComments,
		thoughtsCommentTargetOptions{Chrome: data.TargetChrome, SelectionPrefix: data.SelectionPrefix},
	)

	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	if err := patchThoughtsCommentTarget(sse, target); err != nil {
		return err
	}
	if err := s.patchThoughtsCommentsPanel(
		sse,
		data.FilePath,
		userEmail,
		data.SectionID,
		data.HeadingHint,
		response.Comments,
	); err != nil {
		return err
	}
	return sse.MarshalAndPatchSignals(map[string]any{
		"section_" + data.SectionID + "_form_open": false,
	})
}

// HandleCommentForm handles comment creation from the Datastar form
func (s *Service) HandleCommentForm(c echo.Context) error {
	// Get user email from session
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not authenticated")
	}

	data := parseCommentForm(c)
	if strings.TrimSpace(data.CommentText) == "" {
		return s.renderFormError(c, data, "Comment text cannot be empty")
	}

	comment, err := s.createCommentInternal(
		c.Request().Context(),
		userEmail,
		CreateCommentRequest{
			FilePath:     data.FilePath,
			CommentText:  data.CommentText,
			SelectedText: data.SelectedText,
			StartLine:    data.StartLine,
			StartColumn:  data.StartColumn,
			EndLine:      data.EndLine,
			EndColumn:    data.EndColumn,
			SectionID:    data.SectionID,
			HeadingHint:  data.HeadingHint,
		},
	)
	if err != nil {
		c.Logger().Errorf("Comment creation error: %v", err)
		return s.renderFormError(c, data, err.Error())
	}

	// Success - patch sidebar, remove form, and reset signals via SSE
	return s.renderCommentSuccess(c, comment, userEmail, data.TargetChrome, data.SelectionPrefix)
}

// renderFormError re-renders the form with an error message via SSE
func (s *Service) renderFormError(
	c echo.Context,
	data commentFormData,
	errMsg string,
) error {
	userEmail, _ := c.Get("user_email").(string)
	response, err := s.GetCommentsForFileInternal(c.Request().Context(), data.FilePath)
	if err != nil {
		response = &GetCommentsResponse{Comments: []CommentWithReplies{}}
	}
	sectionComments := filterCommentsBySection(response.Comments, data.SectionID)

	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	target := s.thoughtsCommentTarget(
		data.FilePath,
		data.SectionID,
		data.HeadingHint,
		userEmail,
		sectionComments,
		thoughtsCommentTargetOptions{Chrome: data.TargetChrome, SelectionPrefix: data.SelectionPrefix},
	)
	if err := patchThoughtsCommentTargetWithForm(sse, target, data, errMsg); err != nil {
		return err
	}
	if err := s.patchThoughtsCommentsPanel(
		sse,
		data.FilePath,
		userEmail,
		data.SectionID,
		data.HeadingHint,
		response.Comments,
	); err != nil {
		return err
	}
	return patchOpenCommentsSignal(sse)
}

// renderCommentSuccess patches the specific section's comment target and resets state via
// SSE
func (s *Service) renderCommentSuccess(
	c echo.Context,
	comment *db.WorkspaceDocComment,
	userEmail string,
	targetChrome commentui.CommentTargetChrome,
	selectionPrefix string,
) error {
	sse := datastar.NewSSE(c.Response().Writer, c.Request())

	// Get section ID from comment
	sectionID := comment.SectionHint.String
	if !comment.SectionHint.Valid || sectionID == "" {
		sectionID = "document"
	}

	// Fetch updated comments for the file
	response, err := s.GetCommentsForFileInternal(
		c.Request().Context(),
		comment.DocPath,
	)
	if err != nil {
		c.Logger().Errorf("Failed to fetch comments: %v", err)
		return err
	}

	// Filter to just this section's comments
	sectionComments := filterCommentsBySection(response.Comments, sectionID)

	target := s.thoughtsCommentTarget(
		comment.DocPath,
		sectionID,
		"",
		userEmail,
		sectionComments,
		thoughtsCommentTargetOptions{Chrome: targetChrome, SelectionPrefix: selectionPrefix},
	)
	if err := patchThoughtsCommentTarget(sse, target); err != nil {
		return err
	}
	if err := s.patchThoughtsCommentsPanel(
		sse,
		comment.DocPath,
		userEmail,
		sectionID,
		comment.HeadingHint.String,
		response.Comments,
	); err != nil {
		return err
	}
	if err := patchOpenCommentsSignal(sse); err != nil {
		return err
	}

	return sse.MarshalAndPatchSignals(map[string]any{
		"section_" + sectionID + "_form_open": false,
	})
}

// extractFilePathFromReferer extracts thoughts path from referer URL
func extractFilePathFromReferer(referer string) string {
	if referer == "" {
		return ""
	}
	parsed, err := url.Parse(referer)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(parsed.Path, "/")
}

func (s *Service) recoverReplyFormValues(c echo.Context, commentID, filePath string) (string, string) {
	if _, err := s.queries.GetDocumentComment(c.Request().Context(), commentID); err == nil {
		if _, pathErr := canonicalThoughtsPath(filePath); pathErr == nil {
			return commentID, filePath
		}
	}
	if err := c.Request().ParseForm(); err != nil {
		return commentID, filePath
	}
	var recoveredCommentID string
	var recoveredFilePath string
	for _, values := range c.Request().Form {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if recoveredFilePath == "" {
				if path, err := canonicalThoughtsPath(value); err == nil {
					recoveredFilePath = path
				}
			}
			if recoveredCommentID == "" {
				if _, err := s.queries.GetDocumentComment(c.Request().Context(), value); err == nil {
					recoveredCommentID = value
				}
			}
		}
	}
	if recoveredCommentID != "" && recoveredCommentID != commentID {
		c.Logger().Warnf("Recovered reply comment_id from morphed hidden fields: %q -> %q", commentID, recoveredCommentID)
		commentID = recoveredCommentID
	}
	if recoveredFilePath != "" && recoveredFilePath != filePath {
		c.Logger().Warnf("Recovered reply doc_path from morphed hidden fields: %q -> %q", filePath, recoveredFilePath)
		filePath = recoveredFilePath
	}
	return commentID, filePath
}

// HandleReplyForm handles reply creation from the Datastar form
func (s *Service) HandleReplyForm(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not authenticated")
	}

	commentID := c.FormValue("comment_id")
	filePath := c.FormValue("doc_path")
	replyText := c.FormValue("reply_text")
	commentID, filePath = s.recoverReplyFormValues(c, commentID, filePath)
	if filePath == "" {
		c.Logger().
			Warnf("Reply form validation failed: missing file path (user: %s)", userEmail)
		return echo.NewHTTPError(http.StatusBadRequest, "file path is required")
	}
	if commentID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "comment ID is required")
	}
	if strings.TrimSpace(replyText) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "reply text cannot be empty")
	}

	// Create reply
	reply, err := s.createReplyInternal(
		c.Request().Context(),
		userEmail,
		CreateReplyRequest{
			CommentID: commentID,
			ReplyText: replyText,
		},
	)
	if err != nil {
		c.Logger().Errorf("Failed to create reply: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create reply")
	}

	c.Logger().Infof("Reply created: %s", reply.ID)

	// Get the parent comment to find its section ID
	parentComment, err := s.queries.GetDocumentComment(
		c.Request().Context(),
		commentID,
	)
	if err != nil {
		c.Logger().Errorf("Failed to get parent comment: %v", err)
		return echo.NewHTTPError(
			http.StatusInternalServerError,
			"failed to get parent comment",
		)
	}

	// Patch the section target via SSE
	return s.renderReplySectionTarget(c, filePath, userEmail, parentComment)
}

func (s *Service) renderReplySectionTarget(
	c echo.Context,
	filePath, userEmail string,
	parentComment db.WorkspaceDocComment,
) error {
	sse := datastar.NewSSE(c.Response().Writer, c.Request())

	// Get section ID from parent comment
	sectionID := parentComment.SectionHint.String
	if !parentComment.SectionHint.Valid || sectionID == "" {
		sectionID = "document"
	}

	response, err := s.GetCommentsForFileInternal(c.Request().Context(), filePath)
	if err != nil {
		return err
	}

	// Filter to just this section's comments
	sectionComments := filterCommentsBySection(response.Comments, sectionID)

	target := s.thoughtsCommentTarget(filePath, sectionID, "", userEmail, sectionComments)
	if err := patchThoughtsCommentTarget(sse, target); err != nil {
		return err
	}
	if err := s.patchThoughtsCommentsPanel(
		sse,
		filePath,
		userEmail,
		sectionID,
		parentComment.HeadingHint.String,
		response.Comments,
	); err != nil {
		return err
	}
	return patchOpenCommentsSignal(sse)
}

// HandleResolveComment handles marking a comment as resolved
func (s *Service) HandleResolveComment(c echo.Context) error {
	// Get user email from session
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not authenticated")
	}

	commentID := c.FormValue("comment_id")
	filePath := c.FormValue("doc_path")
	if commentID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "comment ID is required")
	}
	if filePath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "file path is required")
	}

	// Get the comment's section ID BEFORE resolving (since it will be hidden after)
	comment, err := s.queries.GetDocumentComment(c.Request().Context(), commentID)
	if err != nil {
		c.Logger().Errorf("Failed to get comment %s: %v", commentID, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get comment")
	}

	sectionID := comment.SectionHint.String
	if !comment.SectionHint.Valid || sectionID == "" {
		sectionID = "document"
	}

	// Resolve the comment
	err = s.ResolveComment(c.Request().Context(), commentID)
	if err != nil {
		c.Logger().Errorf("Failed to resolve comment %s: %v", commentID, err)
		return echo.NewHTTPError(
			http.StatusInternalServerError,
			"failed to resolve comment",
		)
	}

	c.Logger().Infof("Comment resolved successfully: %s", commentID)

	// Fetch updated comments
	response, err := s.GetCommentsForFileInternal(c.Request().Context(), filePath)
	if err != nil {
		c.Logger().
			Errorf("Failed to fetch updated comments for file %s: %v", filePath, err)
		return err
	}

	// Filter to just this section's unresolved comments for the inline target.
	sectionComments := filterCommentsBySection(response.Comments, sectionID)

	// Use SSE to patch the specific section target
	sse := datastar.NewSSE(c.Response().Writer, c.Request())

	target := s.thoughtsCommentTarget(filePath, sectionID, "", userEmail, sectionComments)
	if err := patchThoughtsCommentTarget(sse, target); err != nil {
		return err
	}
	if err := s.patchThoughtsCommentsPanel(
		sse,
		filePath,
		userEmail,
		sectionID,
		comment.HeadingHint.String,
		response.Comments,
	); err != nil {
		return err
	}
	return patchOpenCommentsSignal(sse)
}

// HandleExpandSectionComments handles expanding section comments on mobile
// Opens the mobile sheet and populates it with the section's comments (no form)
func (s *Service) HandleExpandSectionComments(c echo.Context) error {
	userEmail, _ := c.Get("user_email").(string)

	sectionID := normalizeSectionID(c.FormValue("section_hint"))
	headingHint := c.FormValue("heading_hint")
	filePath := c.FormValue("doc_path")

	// Fetch comments for the file
	response, err := s.GetCommentsForFileInternal(c.Request().Context(), filePath)
	if err != nil {
		c.Logger().Errorf("Failed to fetch comments: %v", err)
		response = &GetCommentsResponse{Comments: []CommentWithReplies{}}
	}

	sse := datastar.NewSSE(c.Response().Writer, c.Request())

	if err := s.patchThoughtsCommentsPanel(
		sse,
		filePath,
		userEmail,
		sectionID,
		headingHint,
		response.Comments,
	); err != nil {
		return err
	}
	if err := patchOpenCommentsSignal(sse); err != nil {
		return err
	}

	return sse.MarshalAndPatchSignals(map[string]any{
		"section_" + sectionID + "_expanded": true,
	})
}

// HandleShowCommentForm handles showing the comment form when button is clicked
// Backend receives selection data and SSE patches the section target with form
// Also patches mobile sheet content for responsive experience
func (s *Service) HandleShowCommentForm(c echo.Context) error {
	// Get user email from session
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not authenticated")
	}

	data := parseCommentForm(c)

	// Fetch existing comments for the file
	response, err := s.GetCommentsForFileInternal(c.Request().Context(), data.FilePath)
	if err != nil {
		c.Logger().Errorf("Failed to fetch comments: %v", err)
		// Continue with empty comments list
		response = &GetCommentsResponse{Comments: []CommentWithReplies{}}
	}

	// Filter to section's comments
	sectionComments := filterCommentsBySection(response.Comments, data.SectionID)

	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	target := s.thoughtsCommentTarget(
		data.FilePath,
		data.SectionID,
		data.HeadingHint,
		userEmail,
		sectionComments,
		thoughtsCommentTargetOptions{Chrome: data.TargetChrome, SelectionPrefix: data.SelectionPrefix},
	)
	if err := patchThoughtsCommentTargetWithForm(sse, target, data, ""); err != nil {
		c.Logger().Errorf("Failed to patch shared comment target: %v", err)
		return err
	}
	if err := s.patchThoughtsCommentsPanel(
		sse,
		data.FilePath,
		userEmail,
		data.SectionID,
		data.HeadingHint,
		response.Comments,
	); err != nil {
		return err
	}
	if err := patchOpenCommentsSignal(sse); err != nil {
		return err
	}

	return sse.MarshalAndPatchSignals(map[string]any{
		"section_" + data.SectionID + "_form_open": true,
		"section_" + data.SectionID + "_expanded":  true,
	})
}
