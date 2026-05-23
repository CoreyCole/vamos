//go:build !integration || unit
// +build !integration unit

package agentchat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/conversation"
)

func TestOpenDocumentWorkspaceUsesNearestAgentsContext(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	thoughtsRoot := filepath.Join(t.TempDir(), "thoughts")
	planRoot := filepath.Join(thoughtsRoot, "creative-mode-agent", "plans", "plan-a")
	if err := os.MkdirAll(filepath.Join(planRoot, "notes"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(planRoot, "AGENTS.md"),
		[]byte("context"),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile AGENTS.md: %v", err)
	}
	doc := filepath.Join(planRoot, "notes", "design.md")
	if err := os.WriteFile(doc, []byte("# Design"), 0o600); err != nil {
		t.Fatalf("WriteFile doc: %v", err)
	}
	service.thoughtsRoot = thoughtsRoot

	result, err := service.OpenDocumentWorkspace(
		t.Context(),
		"user@example.com",
		"thoughts/creative-mode-agent/plans/plan-a/notes/design.md",
	)
	if err != nil {
		t.Fatalf("OpenDocumentWorkspace: %v", err)
	}
	if result.Workspace.RootDocPath != planRoot {
		t.Fatalf("artifact root = %q, want %q", result.Workspace.RootDocPath, planRoot)
	}
	if result.RelPath != "notes/design.md" {
		t.Fatalf("rel path = %q, want notes/design.md", result.RelPath)
	}
	stored, err := service.queries.GetWorkspace(t.Context(), result.Workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if !stored.SelectedDocPath.Valid ||
		stored.SelectedDocPath.String != "notes/design.md" {
		t.Fatalf("selected doc = %#v, want notes/design.md", stored.SelectedDocPath)
	}
}

func TestOpenDocumentWorkspaceFallsBackToDocumentDirectoryWithoutAgents(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	thoughtsRoot := filepath.Join(t.TempDir(), "thoughts")
	docDir := filepath.Join(thoughtsRoot, "research", "topic-a")
	if err := os.MkdirAll(docDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	doc := filepath.Join(docDir, "notes.md")
	if err := os.WriteFile(doc, []byte("# Notes"), 0o600); err != nil {
		t.Fatalf("WriteFile doc: %v", err)
	}
	service.thoughtsRoot = thoughtsRoot

	result, err := service.OpenDocumentWorkspace(
		t.Context(),
		"user@example.com",
		"research/topic-a/notes.md",
	)
	if err != nil {
		t.Fatalf("OpenDocumentWorkspace: %v", err)
	}
	if result.Workspace.RootDocPath != docDir {
		t.Fatalf("artifact root = %q, want %q", result.Workspace.RootDocPath, docDir)
	}
	if result.RelPath != "notes.md" {
		t.Fatalf("rel path = %q, want notes.md", result.RelPath)
	}
}

func TestStartWorkspaceThreadPassesSelectedDocPathToAgentContext(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	fakeTemporal := &fakeTemporalStarter{}
	service.temporal = fakeTemporal
	thoughtsRoot := filepath.Join(t.TempDir(), "thoughts")
	docDir := filepath.Join(thoughtsRoot, "research")
	if err := os.MkdirAll(docDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(docDir, "notes.md"),
		[]byte("# Notes\n\nImportant doc context."),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile doc: %v", err)
	}
	service.thoughtsRoot = thoughtsRoot

	result, err := service.OpenDocumentWorkspace(
		t.Context(),
		"user@example.com",
		"research/notes.md",
	)
	if err != nil {
		t.Fatalf("OpenDocumentWorkspace: %v", err)
	}
	if _, _, _, err := service.StartWorkspaceThread(
		t.Context(),
		result.Workspace.ID,
		"user@example.com",
		"summarize this",
	); err != nil {
		t.Fatalf("StartWorkspaceThread: %v", err)
	}
	input, ok := fakeTemporal.lastInput.(conversation.RunInput)
	if !ok {
		t.Fatalf("last input = %T, want conversation.RunInput", fakeTemporal.lastInput)
	}
	fullPath := filepath.Join(docDir, "notes.md")
	for _, want := range []string{"read this file", "File: `" + fullPath + "`"} {
		if !strings.Contains(input.Context, want) {
			t.Fatalf("context missing %q: %s", want, input.Context)
		}
	}
	if strings.Contains(input.Context, "Important doc context.") {
		t.Fatalf(
			"context should pass a file path, not stuff document contents: %s",
			input.Context,
		)
	}
	if input.Prompt != "summarize this" {
		t.Fatalf("prompt = %q, want original user prompt", input.Prompt)
	}
}
