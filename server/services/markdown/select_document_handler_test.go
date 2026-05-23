package markdown

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	commentsvc "github.com/CoreyCole/vamos/server/services/comments"
	dbsvc "github.com/CoreyCole/vamos/server/services/db"
)

func newDocumentSelectionService(t *testing.T) *Service {
	t.Helper()
	root := t.TempDir()
	planDir := filepath.Join(root, "owner", "plan-a")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"AGENTS.md":  "# Workspace\n",
		"design.md":  "# Design\n",
		"outline.md": "# Outline\n",
	} {
		if err := os.WriteFile(
			filepath.Join(planDir, name),
			[]byte(content),
			0o644,
		); err != nil {
			t.Fatal(err)
		}
	}
	database, err := dbsvc.NewService(filepath.Join(t.TempDir(), "comments.db"))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	commentService := commentsvc.NewService(
		database.DB(),
		"test-commit",
		"https://github.com/example/repo/blob",
		root,
	)
	service, err := NewService(root, commentService, nil)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestThoughtsDocURLWithChatStatePreservesChatQuery(t *testing.T) {
	t.Parallel()

	got := ThoughtsDocURLWithChatState(
		"creative-mode-agent/plans/a/next.md",
		"section-1",
		DocumentEmbeddedChatSelection{
			WorkspaceID: "ws_1",
			ThreadID:    "th_1",
			RunID:       "run_1",
		},
	)
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", got, err)
	}
	if parsed.Path != "/thoughts/creative-mode-agent/plans/a/next.md" ||
		parsed.Fragment != "section-1" {
		t.Fatalf("url = %q, path=%q fragment=%q", got, parsed.Path, parsed.Fragment)
	}
	query := parsed.Query()
	for key, want := range map[string]string{
		"context":        "chat",
		"chat_workspace": "ws_1",
		"thread":         "th_1",
		"run":            "run_1",
	} {
		if gotValue := query.Get(key); gotValue != want {
			t.Fatalf("query[%q] = %q, want %q in %q", key, gotValue, want, got)
		}
	}
}

func TestHandleSelectDocumentPreservesChatStateInURL(t *testing.T) {
	t.Parallel()

	service := newDocumentSelectionService(t)
	c, rec := newPostFormContext(t, "/thoughts/actions/select-document", url.Values{
		"doc_path":       {"owner/plan-a/design.md"},
		"preserve_chat":  {"true"},
		"chat_workspace": {"ws_1"},
		"thread_id":      {"th_1"},
		"run_id":         {"run_1"},
	})

	if err := service.HandleSelectDocument(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"context=chat",
		"chat_workspace=ws_1",
		"thread=th_1",
		"run=run_1",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %q:\n%s", want, body)
		}
	}
}

func TestHandleSelectDocumentPatchesDocumentWorkbenchChrome(t *testing.T) {
	t.Parallel()

	service := newDocumentSelectionService(t)
	c, rec := newPostFormContext(t, "/thoughts/actions/select-document", url.Values{
		"doc_path": {"owner/plan-a/design.md"},
	})

	if err := service.HandleSelectDocument(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"selector #workbench-root",
		"doc-workbench-sidebar-region",
		"doc-workbench-center-region",
		"doc-workbench-right-region",
		"doc-workbench-center-pane",
		"doc-workbench-viewer-region",
		"thoughts-document-panel",
		"thoughts-shared-sidebar",
		"thoughts-url-sync",
		"Open Workspaces and Files",
		"Open Chat",
		`data-replace-url="&#34;/thoughts/owner/plan-a/design.md&#34;"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %q:\n%s", want, body)
		}
	}
	for _, unwanted := range []string{
		"selector #thoughts-directory-region",
		"workspace-doc-tree-header",
		"workspaceDocTreeOpen",
	} {
		if strings.Contains(body, unwanted) {
			t.Fatalf(
				"response body should not put related docs tree in topbar %q:\n%s",
				unwanted,
				body,
			)
		}
	}
}

func TestHandleSelectDocumentWithHashDispatchesWorkbenchSectionNav(t *testing.T) {
	service := newDocumentSelectionService(t)
	c, rec := newPostFormContext(t, "/thoughts/actions/select-document", url.Values{
		"doc_path": {"owner/plan-a/design.md"},
		"hash":     {"design"},
	})

	if err := service.HandleSelectDocument(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`data-replace-url="&#34;/thoughts/owner/plan-a/design.md#design&#34;"`,
		"workbench-section-nav",
		`detail: { hash: 'design', updateURL: false }`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %q:\n%s", want, body)
		}
	}
	for _, unwanted := range []string{
		"data-doc-section-target",
		"workbenchScrollDocumentSection",
	} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("response body contains unwanted %q:\n%s", unwanted, body)
		}
	}
}
