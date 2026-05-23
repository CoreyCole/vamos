package testhelpers

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
)

// FormTestHelper provides utilities for testing templ form components
type FormTestHelper struct {
	Doc *goquery.Document
}

// RenderToDocument renders a templ component and parses it into a goquery document
func RenderToDocument(t *testing.T, component templ.Component) *FormTestHelper {
	t.Helper()

	r, w := io.Pipe()
	go func() {
		_ = component.Render(context.Background(), w)
		_ = w.Close()
	}()

	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	return &FormTestHelper{Doc: doc}
}

// GetFormValues extracts all form field values from the selected form element
func (h *FormTestHelper) GetFormValues(selector string) url.Values {
	values := url.Values{}
	form := h.Doc.Find(selector)

	// Extract input values
	form.Find("input").Each(func(i int, s *goquery.Selection) {
		name, exists := s.Attr("name")
		if !exists || name == "" {
			return
		}
		inputType, _ := s.Attr("type")

		// Skip unchecked checkboxes and radios
		if inputType == "checkbox" || inputType == "radio" {
			if _, checked := s.Attr("checked"); !checked {
				return
			}
		}
		value, _ := s.Attr("value")
		values.Add(name, value)
	})

	// Extract select values
	form.Find("select").Each(func(i int, s *goquery.Selection) {
		name, _ := s.Attr("name")
		if name == "" {
			return
		}
		if selected := s.Find("option[selected]"); selected.Length() > 0 {
			value, _ := selected.Attr("value")
			values.Add(name, value)
		}
	})

	// Extract textarea values
	form.Find("textarea").Each(func(i int, s *goquery.Selection) {
		name, _ := s.Attr("name")
		if name != "" {
			values.Add(name, s.Text())
		}
	})

	return values
}

// ToEchoContext creates an echo.Context with form data for testing handlers
func (h *FormTestHelper) ToEchoContext(t *testing.T, formSelector string) echo.Context {
	t.Helper()

	formValues := h.GetFormValues(formSelector)

	req := httptest.NewRequest(
		http.MethodPost,
		"/",
		bytes.NewBufferString(formValues.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	e := echo.New()
	return e.NewContext(req, httptest.NewRecorder())
}
