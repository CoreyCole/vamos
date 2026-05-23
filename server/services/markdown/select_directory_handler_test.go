package markdown

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func newDirectorySelectionService(t *testing.T) *Service {
	t.Helper()
	root := t.TempDir()
	planDir := filepath.Join(root, "owner", "plan-a")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(planDir, "design.md"),
		[]byte("# Design"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func newPostFormContext(
	t *testing.T,
	path string,
	form url.Values,
) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	return c, rec
}

func TestHandleSelectDirectoryRejectsEscapingPath(t *testing.T) {
	service := newDirectorySelectionService(t)
	c, _ := newPostFormContext(t, "/thoughts/actions/select-directory", url.Values{
		"dir_path": {"../secret"},
	})

	err := service.HandleSelectDirectory(c)
	if httpErr, ok := err.(*echo.HTTPError); !ok ||
		httpErr.Code != http.StatusBadRequest {
		t.Fatalf("HandleSelectDirectory() error = %#v, want HTTP 400", err)
	}
}

func TestHandleSelectDirectoryPatchesPrimarySidebarAndURL(t *testing.T) {
	service := newDirectorySelectionService(t)
	c, rec := newPostFormContext(t, "/thoughts/actions/select-directory", url.Values{
		"dir_path": {"owner/plan-a"},
	})

	if err := service.HandleSelectDirectory(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"selector #doc-workbench-viewer-region, #thoughts-directory-region",
		"thoughts-directory-primary",
		"thoughts-shared-sidebar",
		"thoughts-url-sync",
		`data-replace-url="&#34;/thoughts/owner/plan-a&#34;"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %q:\n%s", want, body)
		}
	}
	replaceURLIndex := strings.Index(
		body,
		`data-replace-url="&#34;/thoughts/owner/plan-a&#34;"`,
	)
	if replaceURLIndex < 0 {
		t.Fatalf("response body missing replace-url patch:\n%s", body)
	}
	primaryPatch := body[:replaceURLIndex]
	if !strings.Contains(primaryPatch, `id="thoughts-url-sync"`) {
		t.Fatalf(
			"primary patch should install URL sync target before data-replace-url patch:\n%s",
			body,
		)
	}
	for _, unwanted := range []string{
		"docWorkbenchSidebar",
		"workspaceDocTreeOpen",
		"workbench.regions.docWorkbenchSidebar",
	} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("response body contains unwanted signal %q:\n%s", unwanted, body)
		}
	}
}
