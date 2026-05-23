package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Responses larger than this threshold will log metadata only (length, content-type)
// instead of the full body to reduce log spam from large HTML/SSE responses
const maxResponseBodyLogSize = 500

// Static file extensions to skip logging
var skipExtensions = []string{
	".js", ".js.map",
	".css",
	".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp",
	".woff", ".woff2", ".ttf", ".eot",
}

// shouldSkipLogging returns true for static assets that shouldn't be logged
func shouldSkipLogging(path string) bool {
	for _, ext := range skipExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// LogEntry represents a structured log entry in ndjson format
type LogEntry struct {
	Time              string  `json:"time"`
	RequestID         string  `json:"request_id"`
	Direction         string  `json:"direction"` // "in" or "out"
	Method            string  `json:"method"`
	Path              string  `json:"path"`
	User              string  `json:"user,omitempty"`
	Body              string  `json:"body,omitempty"`
	ResponseBody      string  `json:"response_body,omitempty"`
	ResponseLength    int     `json:"response_length,omitempty"`
	ResponseType      string  `json:"response_type,omitempty"`
	ResponseTruncated bool    `json:"response_truncated,omitempty"`
	Status            int     `json:"status,omitempty"`
	DurationMS        float64 `json:"duration_ms,omitempty"`
	Error             string  `json:"error,omitempty"`
}

type cappedResponseBody struct {
	buf   bytes.Buffer
	limit int
	size  int
}

func newCappedResponseBody(limit int) *cappedResponseBody {
	return &cappedResponseBody{limit: limit}
}

func (b *cappedResponseBody) Write(p []byte) {
	b.size += len(p)
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		return
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	_, _ = b.buf.Write(p)
}

func (b *cappedResponseBody) Len() int {
	return b.size
}

func (b *cappedResponseBody) String() string {
	return b.buf.String()
}

// responseBodyWriter wraps http.ResponseWriter to capture bounded response body metadata.
type responseBodyWriter struct {
	http.ResponseWriter
	body *cappedResponseBody
}

func (w *responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// Flush implements http.Flusher for SSE support
func (w *responseBodyWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// LoggingMiddleware provides structured request/response logging with request body
// capture
func LoggingMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()

			// Skip logging for static assets
			if shouldSkipLogging(req.URL.Path) {
				return next(c)
			}

			start := time.Now()

			// Generate request ID
			requestID := uuid.New().String()
			c.Set("request_id", requestID)
			c.Response().Header().Set("X-Request-ID", requestID)

			// Read and buffer request body for logging
			var bodyBytes []byte
			var bodyStr string
			if req.Body != nil {
				bodyBytes, _ = io.ReadAll(req.Body)
				bodyStr = string(bodyBytes)
				// Replace body so handlers can still read it
				req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}

			// Get user email from context if authenticated (set by auth middleware)
			userEmail := ""
			if email, ok := c.Get("user_email").(string); ok {
				userEmail = email
			}

			// Log incoming request
			inLog := LogEntry{
				Time:      time.Now().Format(time.RFC3339),
				RequestID: requestID,
				Direction: "in",
				Method:    req.Method,
				Path:      req.URL.Path,
				User:      userEmail,
				Body:      bodyStr,
			}
			logJSON(inLog)

			// Wrap response writer to capture only the small prefix needed for logs.
			bodyBuf := newCappedResponseBody(maxResponseBodyLogSize + 1)
			resWriter := &responseBodyWriter{
				ResponseWriter: c.Response().Writer,
				body:           bodyBuf,
			}
			c.Response().Writer = resWriter

			// Execute handler
			err := next(c)

			// Calculate duration
			duration := time.Since(start)
			durationMS := float64(duration.Microseconds()) / 1000.0

			// Capture response body and metadata
			responseBody := bodyBuf.String()
			responseLength := bodyBuf.Len()
			contentType := c.Response().Header().Get("Content-Type")

			// Log outgoing response
			outLog := LogEntry{
				Time:           time.Now().Format(time.RFC3339),
				RequestID:      requestID,
				Direction:      "out",
				Method:         req.Method,
				Path:           req.URL.Path,
				User:           userEmail,
				Status:         c.Response().Status,
				DurationMS:     durationMS,
				ResponseLength: responseLength,
				ResponseType:   contentType,
			}

			// For small responses, include full body; for large ones, just metadata
			if responseLength <= maxResponseBodyLogSize {
				outLog.ResponseBody = responseBody
			} else {
				outLog.ResponseTruncated = true
			}

			if err != nil {
				outLog.Error = err.Error()
			}
			logJSON(outLog)

			return err
		}
	}
}

// logJSON outputs a log entry as ndjson
func logJSON(entry LogEntry) {
	b, err := json.Marshal(entry)
	if err != nil {
		fmt.Printf("Failed to marshal log entry: %v\n", err)
		return
	}
	fmt.Println(string(b))
}
