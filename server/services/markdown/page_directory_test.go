//go:build !integration || unit
// +build !integration unit

package markdown

import (
	"bytes"
	"strings"
	"testing"
)

func TestDirectoryPrimaryPanelRendersAnchors(t *testing.T) {
	t.Parallel()

	args := &DirectoryArgs{Items: []DirectoryItem{
		{Name: "docs", Path: "owner/docs", IsDir: true},
		{Name: "plan.md", Path: "owner/plan.md"},
	}}
	var buf bytes.Buffer
	if err := DirectoryPrimaryPanel(args).Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{`href="/thoughts/owner/docs"`, `href="/thoughts/owner/plan.md"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %s", want, html)
		}
	}
	for _, notWant := range []string{"/thoughts/actions/select-directory", "/thoughts/actions/select-document", "data-on:submit"} {
		if strings.Contains(html, notWant) {
			t.Fatalf("unexpected %q in %s", notWant, html)
		}
	}
}

func TestDirectoryPrimaryPanelPreservesNoThreadChatWorkspaceQuery(t *testing.T) {
	t.Parallel()

	args := &DirectoryArgs{
		ChatLinkState: EmbeddedChatLinkState{Active: true, WorkspaceID: "ws 1", RunID: "run+1"},
		Items: []DirectoryItem{
			{Name: "docs", Path: "owner/docs", IsDir: true},
			{Name: "plan.md", Path: "owner/plan.md"},
		},
	}
	var buf bytes.Buffer
	if err := DirectoryPrimaryPanel(args).Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{"context=chat", "chat_workspace=ws+1", "run=run%2B1"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing preserved chat query %q in %s", want, html)
		}
	}
}

func TestDirectoryPrimaryPanelOmitsChatWorkspaceForActiveThreadQuery(t *testing.T) {
	t.Parallel()

	args := &DirectoryArgs{
		ChatLinkState: EmbeddedChatLinkState{Active: true, WorkspaceID: "ws 1", ThreadID: "th/1", RunID: "run+1"},
		Items: []DirectoryItem{
			{Name: "docs", Path: "owner/docs", IsDir: true},
			{Name: "plan.md", Path: "owner/plan.md"},
		},
	}
	var buf bytes.Buffer
	if err := DirectoryPrimaryPanel(args).Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{"context=chat", "thread=th%2F1", "run=run%2B1"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing preserved chat query %q in %s", want, html)
		}
	}
	if strings.Contains(html, "chat_workspace=ws+1") {
		t.Fatalf("thread-active directory links preserved chat_workspace: %s", html)
	}
}
