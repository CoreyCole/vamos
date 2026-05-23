//go:build !integration || unit
// +build !integration unit

package markdown

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
)

func TestOpenChatForDocumentPatchesEmptyStateWhenNoCandidates(t *testing.T) {
	t.Parallel()

	resolver := &fakeChatWorkspaceCandidateResolver{}
	c, rec := newOpenChatRequest(t, "/thoughts/chat/open", url.Values{
		"doc_path": {"thoughts/user/loose/doc.md"},
	})

	if err := (&Service{chatWorkspaceResolver: resolver}).OpenChatForDocument(
		c,
	); err != nil {
		t.Fatalf("OpenChatForDocument() error = %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "No AGENTS.md workspace root found") ||
		!strings.Contains(body, "thoughts-open-chat-result") {
		t.Fatalf("response body = %s, want empty-state patch", body)
	}
}

func TestOpenChatForDocumentOpensSingleCandidateInPlace(t *testing.T) {
	t.Parallel()

	resolver := &fakeChatWorkspaceCandidateResolver{
		candidates: []ChatWorkspaceCandidate{{
			RootPath: "thoughts/user/plans/plan-a",
			Label:    "plan-a · user/plans",
		}},
		openResult: OpenChatWorkspaceResult{
			WorkspaceID: "ws_1",
			URL:         "/thoughts/?chat_workspace=ws_1",
		},
	}
	c, rec := newOpenChatRequest(t, "/thoughts/actions/open-chat", url.Values{
		"doc_path": {"thoughts/user/plans/plan-a/doc.md"},
	})

	if err := (&Service{chatWorkspaceResolver: resolver}).OpenChatForDocument(
		c,
	); err != nil {
		t.Fatalf("OpenChatForDocument() error = %v", err)
	}
	if resolver.openRoot != "thoughts/user/plans/plan-a" {
		t.Fatalf("openRoot = %q, want single candidate root", resolver.openRoot)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"rightRailActiveTab",
		"chat",
		"agent-chat-composer-attachments",
		"thoughts/user/plans/plan-a/doc.md",
		"doc.md",
		"agent-chat-composer-input",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %q: %s", want, body)
		}
	}
	for _, notWant := range []string{"thoughts-shared-sidebar", "doc-workbench-center-pane"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("response body contains %q: %s", notWant, body)
		}
	}
}

func TestOpenChatForDocumentUsesNearestCandidateInPlace(t *testing.T) {
	t.Parallel()

	resolver := &fakeChatWorkspaceCandidateResolver{
		candidates: []ChatWorkspaceCandidate{
			{RootPath: "thoughts/user/plans/plan-a", Label: "plan-a · user/plans"},
			{
				RootPath: "thoughts/user/plans/plan-a/reviews/r1",
				Label:    "r1 · user/plans/plan-a/reviews",
			},
		},
	}
	c, _ := newOpenChatRequest(t, "/thoughts/actions/open-chat", url.Values{
		"doc_path": {"thoughts/user/plans/plan-a/reviews/r1/review.md"},
	})

	if err := (&Service{chatWorkspaceResolver: resolver}).OpenChatForDocument(
		c,
	); err != nil {
		t.Fatalf("OpenChatForDocument() error = %v", err)
	}
	if resolver.openRoot != "thoughts/user/plans/plan-a/reviews/r1" {
		t.Fatalf("openRoot = %q, want nearest candidate", resolver.openRoot)
	}
}

func TestSelectChatWorkspaceCandidateRedirects(t *testing.T) {
	t.Parallel()

	resolver := &fakeChatWorkspaceCandidateResolver{
		openResult: OpenChatWorkspaceResult{
			WorkspaceID: "ws_2",
			URL:         "/thoughts/?chat_workspace=ws_2",
		},
	}
	c, rec := newOpenChatRequest(t, "/thoughts/chat/select", url.Values{
		"doc_path":  {"thoughts/user/plans/plan-a/doc.md"},
		"root_path": {"thoughts/user/plans/plan-a"},
	})

	if err := (&Service{chatWorkspaceResolver: resolver}).SelectChatWorkspaceCandidate(
		c,
	); err != nil {
		t.Fatalf("SelectChatWorkspaceCandidate() error = %v", err)
	}
	if resolver.openRoot != "thoughts/user/plans/plan-a" {
		t.Fatalf("openRoot = %q, want selected root", resolver.openRoot)
	}
	if body := rec.Body.String(); !strings.Contains(
		body,
		"/thoughts/?chat_workspace=ws_2",
	) {
		t.Fatalf("response body = %s, want redirect URL", body)
	}
}

func newOpenChatRequest(
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

type fakeChatWorkspaceCandidateResolver struct {
	candidates []ChatWorkspaceCandidate
	openResult OpenChatWorkspaceResult
	openRoot   string
}

func (f *fakeChatWorkspaceCandidateResolver) ResolveChatWorkspaceCandidates(
	ctx context.Context,
	userEmail, documentPath string,
) ([]ChatWorkspaceCandidate, error) {
	_ = ctx
	_ = userEmail
	_ = documentPath
	return f.candidates, nil
}

func (f *fakeChatWorkspaceCandidateResolver) OpenChatWorkspace(
	ctx context.Context,
	userEmail, rootPath string,
) (OpenChatWorkspaceResult, error) {
	_ = ctx
	_ = userEmail
	f.openRoot = rootPath
	return f.openResult, nil
}

func TestThoughtsContextPanelRendersEmbeddedChatComponent(t *testing.T) {
	t.Parallel()

	component := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := w.Write(
			[]byte(`<div id="embedded-chat-test">Embedded chat content</div>`),
		)
		return err
	})
	var body bytes.Buffer
	if err := ThoughtsContextPanel(ThoughtsContextArgs{
		Mode:      thoughtsContextModeChat,
		PageArgs:  &PageArgs{FilePath: "thoughts/user/doc.md"},
		Component: component,
	}).Render(t.Context(), &body); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	got := body.String()
	if !strings.Contains(got, "embedded-chat-test") {
		t.Fatalf("body missing embedded component: %s", got)
	}
	if strings.Contains(got, "Open Chat") {
		t.Fatalf("body contains Open Chat fallback: %s", got)
	}
}
