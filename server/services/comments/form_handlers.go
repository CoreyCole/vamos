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
	FilePath     string
	CommentText  string
	SelectedText string
	SectionID    string
	HeadingHint  string
	StartLine    int
	StartColumn  int
	EndLine      int
	EndColumn    int
}

func parseCommentForm(c echo.Context) commentFormData {
	documentPath := c.FormValue("doc_path")
	if strings.TrimSpace(documentPath) == "" {
		documentPath = c.FormValue("file_path")
	}
	return commentFormData{
		FilePath:     documentPath,
		CommentText:  c.FormValue("comment_text"),
		SelectedText: c.FormValue("selected_text"),
		SectionID:    normalizeSectionID(c.FormValue("section_hint")),
		HeadingHint:  c.FormValue("heading_hint"),
		StartLine:    parseFormInt(c, "start_line"),
		StartColumn:  parseFormInt(c, "start_column"),
		EndLine:      parseFormInt(c, "end_line"),
		EndColumn:    parseFormInt(c, "end_column"),
	}
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

func thoughtsCommentThreads(items []CommentWithReplies) []commentui.CommentThreadView {
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
				CreatedAt:   reply.CreatedAt,
				Body:        reply.ReplyText,
			})
		}
		sources = append(sources, commentui.ThreadSource{
			ID:           item.Comment.ID,
			AuthorEmail:  item.Comment.UserEmail,
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

func thoughtsCommentTarget(
	filePath, sectionID, headingHint, userEmail string,
	comments []CommentWithReplies,
) commentui.CommentTargetView {
	sectionID = normalizeSectionID(sectionID)
	prefix := commentui.SafeCommentTargetSlug("thoughts", filePath)
	return commentui.BuildTargetView(commentui.TargetInput{
		Surface:      commentui.CommentSurfaceThoughts,
		IDPrefix:     prefix,
		DocPath:      filePath,
		SectionID:    sectionID,
		HeadingHint:  headingHint,
		UserEmail:    userEmail,
		Threads:      thoughtsCommentThreads(comments),
		Routes:       thoughtsCommentRoutes(),
		HiddenFields: map[string]string{"doc_path": filePath},
	})
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
	return sse.MarshalAndPatchSignals(map[string]any{"rightRailActiveTab": "comments"})
}

func patchThoughtsCommentsPanel(
	sse *datastar.ServerSentEventGenerator,
	filePath, userEmail, activeSectionID string,
	items []CommentWithReplies,
) error {
	args := commentui.CommentableMarkdownArgs{
		Surface:      commentui.CommentSurfaceThoughts,
		IDPrefix:     commentui.SafeCommentTargetSlug("thoughts", filePath),
		DocPath:      filePath,
		Comments:     thoughtsCommentThreads(items),
		Routes:       thoughtsCommentRoutes(),
		HiddenFields: map[string]string{"doc_path": filePath, "context_panel": "1"},
		UserEmail:    userEmail,
	}
	return sse.PatchElementTempl(
		commentui.CommentsContextPanel(
			commentui.BuildCommentsPanelArgs(args, activeSectionID),
		),
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
	target := thoughtsCommentTarget(
		data.FilePath,
		data.SectionID,
		data.HeadingHint,
		userEmail,
		sectionComments,
	)

	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	if err := patchThoughtsCommentTarget(sse, target); err != nil {
		return err
	}
	if err := patchThoughtsCommentsPanel(
		sse,
		data.FilePath,
		userEmail,
		data.SectionID,
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
		},
	)
	if err != nil {
		c.Logger().Errorf("Comment creation error: %v", err)
		return s.renderFormError(c, data, err.Error())
	}

	// Success - patch sidebar, remove form, and reset signals via SSE
	return s.renderCommentSuccess(c, comment, userEmail)
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
	target := thoughtsCommentTarget(
		data.FilePath,
		data.SectionID,
		data.HeadingHint,
		userEmail,
		sectionComments,
	)
	if err := patchThoughtsCommentTargetWithForm(sse, target, data, errMsg); err != nil {
		return err
	}
	if err := patchThoughtsCommentsPanel(
		sse,
		data.FilePath,
		userEmail,
		data.SectionID,
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

	target := thoughtsCommentTarget(
		comment.DocPath,
		sectionID,
		"",
		userEmail,
		sectionComments,
	)
	if err := patchThoughtsCommentTarget(sse, target); err != nil {
		return err
	}
	if err := patchThoughtsCommentsPanel(
		sse,
		comment.DocPath,
		userEmail,
		sectionID,
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

// HandleReplyForm handles reply creation from the Datastar form
func (s *Service) HandleReplyForm(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not authenticated")
	}

	commentID := c.FormValue("comment_id")
	filePath := c.FormValue("doc_path")
	replyText := c.FormValue("reply_text")
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

	target := thoughtsCommentTarget(filePath, sectionID, "", userEmail, sectionComments)
	if err := patchThoughtsCommentTarget(sse, target); err != nil {
		return err
	}
	if err := patchThoughtsCommentsPanel(
		sse,
		filePath,
		userEmail,
		sectionID,
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

	target := thoughtsCommentTarget(filePath, sectionID, "", userEmail, sectionComments)
	if err := patchThoughtsCommentTarget(sse, target); err != nil {
		return err
	}
	if err := patchThoughtsCommentsPanel(
		sse,
		filePath,
		userEmail,
		sectionID,
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
	filePath := c.FormValue("doc_path")

	// Fetch comments for the file
	response, err := s.GetCommentsForFileInternal(c.Request().Context(), filePath)
	if err != nil {
		c.Logger().Errorf("Failed to fetch comments: %v", err)
		response = &GetCommentsResponse{Comments: []CommentWithReplies{}}
	}

	sse := datastar.NewSSE(c.Response().Writer, c.Request())

	if err := patchThoughtsCommentsPanel(
		sse,
		filePath,
		userEmail,
		sectionID,
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
	target := thoughtsCommentTarget(
		data.FilePath,
		data.SectionID,
		data.HeadingHint,
		userEmail,
		sectionComments,
	)
	if err := patchThoughtsCommentTargetWithForm(sse, target, data, ""); err != nil {
		c.Logger().Errorf("Failed to patch shared comment target: %v", err)
		return err
	}
	if err := patchThoughtsCommentsPanel(
		sse,
		data.FilePath,
		userEmail,
		data.SectionID,
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
