package utils

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/labstack/echo/v4"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// BindFormToProto parses form/JSON data into a proto message using protojson.
// Accepts both camelCase and snake_case field names automatically.
//
// For Datastar forms, use contentType: 'form' and camelCase name attributes.
// The formID parameter is used to extract nested JSON data when using JSON content type
// (e.g., Datastar sends {formID: {fields}} structure).
func BindFormToProto[T proto.Message](c echo.Context, msg T, formID string) error {
	formData := make(map[string]any)
	contentType := c.Request().Header.Get("Content-Type")

	switch {
	case strings.Contains(contentType, "application/x-www-form-urlencoded"):
		if err := c.Request().ParseForm(); err != nil {
			return fmt.Errorf("failed to parse form: %w", err)
		}
		for key, values := range c.Request().Form {
			if len(values) == 1 {
				formData[key] = values[0]
			} else {
				formData[key] = values
			}
		}

	case strings.Contains(contentType, "multipart/form-data"):
		if err := c.Request().ParseMultipartForm(10 << 20); err != nil {
			return fmt.Errorf("failed to parse multipart form: %w", err)
		}
		if c.Request().MultipartForm != nil {
			for key, values := range c.Request().MultipartForm.Value {
				if len(values) == 1 {
					formData[key] = values[0]
				} else {
					formData[key] = values
				}
			}
		}

	default:
		// JSON content type
		if err := c.Bind(&formData); err != nil {
			return fmt.Errorf("failed to parse JSON: %w", err)
		}
		// Extract nested data using explicit formID: {formID: {fields}} -> {fields}
		if formID != "" {
			if nested, ok := formData[formID].(map[string]any); ok {
				formData = nested
			}
		}
	}

	// Convert form data to JSON bytes
	jsonBytes, err := json.Marshal(formData)
	if err != nil {
		return fmt.Errorf("form to JSON conversion failed: %w", err)
	}

	// Use protojson to unmarshal - handles camelCase/snake_case automatically
	opts := protojson.UnmarshalOptions{
		DiscardUnknown: true, // Ignore extra fields (CSRF tokens, etc.)
	}

	if err := opts.Unmarshal(jsonBytes, msg); err != nil {
		return fmt.Errorf("protojson unmarshal failed: %w", err)
	}

	return nil
}
