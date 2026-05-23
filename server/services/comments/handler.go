package comments

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
	}
}

// RegisterRoutes registers comment routes with Echo under an auth-protected group
func (h *Handler) RegisterRoutes(g *echo.Group) {
	comments := g.Group("/comments")
	comments.POST("", h.CreateComment)
	comments.POST("/replies", h.CreateReply)
	comments.GET("/file", h.GetCommentsForFile)
}

// CreateComment handles POST /api/comments
func (h *Handler) CreateComment(c echo.Context) error {
	// Get user email from session
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not authenticated")
	}

	var req CreateCommentRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	comment, err := h.service.createCommentInternal(c.Request().Context(), userEmail, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, comment)
}

// CreateReply handles POST /api/comments/replies
func (h *Handler) CreateReply(c echo.Context) error {
	// Get user email from session
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not authenticated")
	}

	var req CreateReplyRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	reply, err := h.service.createReplyInternal(c.Request().Context(), userEmail, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, reply)
}

// GetCommentsForFile handles GET /api/comments/file?path=...
func (h *Handler) GetCommentsForFile(c echo.Context) error {
	filePath := c.QueryParam("path")
	if filePath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "file path required")
	}

	response, err := h.service.GetCommentsForFileInternal(c.Request().Context(), filePath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, response)
}
