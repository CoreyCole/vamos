//go:build !integration || unit
// +build !integration unit

package markdown

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/CoreyCole/vamos/pkg/db"
)

func TestResolveChatWorkspaceCandidatesUsesAgentsAncestors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	thoughtsRoot := filepath.Join(root, "thoughts")
	planRoot := filepath.Join(thoughtsRoot, "creative-mode-agent", "plans", "plan-a")
	reviewRoot := filepath.Join(planRoot, "reviews", "r1")
	mustMkdirAll(t, reviewRoot)
	mustWriteFile(
		t,
		filepath.Join(thoughtsRoot, "creative-mode-agent", "AGENTS.md"),
		[]byte("# User"),
	)
	mustWriteFile(t, filepath.Join(planRoot, "AGENTS.md"), []byte("# Plan"))
	mustWriteFile(t, filepath.Join(planRoot, "doc.md"), []byte("# Doc"))
	mustWriteFile(t, filepath.Join(reviewRoot, "AGENTS.md"), []byte("# Review"))
	mustWriteFile(t, filepath.Join(reviewRoot, "review.md"), []byte("# Review doc"))

	queries := newWorkspaceResolverTestQueries(t)
	resolver := NewDBChatWorkspaceCandidateResolver(queries, thoughtsRoot, nil)

	got, err := resolver.ResolveChatWorkspaceCandidates(
		ctx,
		"user@example.com",
		"thoughts/creative-mode-agent/plans/plan-a/doc.md",
	)
	if err != nil {
		t.Fatalf("ResolveChatWorkspaceCandidates(plan doc) error = %v", err)
	}
	if len(got) != 1 || got[0].RootPath != "thoughts/creative-mode-agent/plans/plan-a" {
		t.Fatalf(
			"ResolveChatWorkspaceCandidates(plan doc) = %+v, want plan root only",
			got,
		)
	}

	got, err = resolver.ResolveChatWorkspaceCandidates(
		ctx,
		"user@example.com",
		"thoughts/creative-mode-agent/plans/plan-a/reviews/r1/review.md",
	)
	if err != nil {
		t.Fatalf("ResolveChatWorkspaceCandidates(review doc) error = %v", err)
	}
	want := []string{
		"thoughts/creative-mode-agent/plans/plan-a",
		"thoughts/creative-mode-agent/plans/plan-a/reviews/r1",
	}
	if len(got) != len(want) {
		t.Fatalf("ResolveChatWorkspaceCandidates(review doc) = %+v, want %v", got, want)
	}
	for i := range want {
		if got[i].RootPath != want[i] {
			t.Fatalf(
				"candidate %d root = %q, want %q; all candidates = %+v",
				i,
				got[i].RootPath,
				want[i],
				got,
			)
		}
	}
}

func TestResolveChatWorkspaceCandidatesExcludesTopLevelAgents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	thoughtsRoot := filepath.Join(root, "thoughts")
	userRoot := filepath.Join(thoughtsRoot, "creative-mode-agent")
	mustMkdirAll(t, userRoot)
	mustWriteFile(t, filepath.Join(userRoot, "AGENTS.md"), []byte("# User"))

	got, err := NewDBChatWorkspaceCandidateResolver(
		newWorkspaceResolverTestQueries(t),
		thoughtsRoot,
		nil,
	).ResolveChatWorkspaceCandidates(
		ctx,
		"user@example.com",
		"thoughts/creative-mode-agent/AGENTS.md",
	)
	if err != nil {
		t.Fatalf("ResolveChatWorkspaceCandidates() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf(
			"ResolveChatWorkspaceCandidates() = %+v, want no top-level candidates",
			got,
		)
	}
}

func TestResolveChatWorkspaceCandidatesMarksExistingWorkspace(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	thoughtsRoot := filepath.Join(root, "thoughts")
	planRoot := filepath.Join(thoughtsRoot, "creative-mode-agent", "plans", "plan-a")
	mustMkdirAll(t, planRoot)
	mustWriteFile(t, filepath.Join(planRoot, "AGENTS.md"), []byte("# Plan"))
	mustWriteFile(t, filepath.Join(planRoot, "doc.md"), []byte("# Doc"))

	queries := newWorkspaceResolverTestQueries(t)
	createResolverWorkspace(
		t,
		queries,
		"workspace-plan",
		"other-user@example.com",
		planRoot,
	)

	got, err := NewDBChatWorkspaceCandidateResolver(queries, thoughtsRoot, nil).
		ResolveChatWorkspaceCandidates(ctx, "user@example.com", "thoughts/creative-mode-agent/plans/plan-a/doc.md")
	if err != nil {
		t.Fatalf("ResolveChatWorkspaceCandidates() error = %v", err)
	}
	if len(got) != 1 || !got[0].HasWorkspace || got[0].WorkspaceID != "workspace-plan" {
		t.Fatalf(
			"ResolveChatWorkspaceCandidates() = %+v, want shared existing workspace marker",
			got,
		)
	}
}

func TestOpenChatWorkspaceValidatesCandidateRoot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	thoughtsRoot := filepath.Join(root, "thoughts")
	planRoot := filepath.Join(thoughtsRoot, "creative-mode-agent", "plans", "plan-a")
	otherRoot := filepath.Join(
		thoughtsRoot,
		"creative-mode-agent",
		"plans",
		"not-a-workspace",
	)
	mustMkdirAll(t, planRoot)
	mustMkdirAll(t, otherRoot)
	mustWriteFile(t, filepath.Join(planRoot, "AGENTS.md"), []byte("# Plan"))

	opener := &fakeChatWorkspaceOpener{workspace: db.Workspace{ID: "ws_1"}}
	resolver := NewDBChatWorkspaceCandidateResolver(
		newWorkspaceResolverTestQueries(t),
		thoughtsRoot,
		opener,
	)

	if _, err := resolver.OpenChatWorkspace(
		ctx,
		"user@example.com",
		"thoughts/creative-mode-agent/plans/not-a-workspace",
	); err == nil {
		t.Fatal("OpenChatWorkspace() error = nil, want rejection for non-candidate root")
	}

	got, err := resolver.OpenChatWorkspace(
		ctx,
		"user@example.com",
		"thoughts/creative-mode-agent/plans/plan-a",
	)
	if err != nil {
		t.Fatalf("OpenChatWorkspace(valid candidate) error = %v", err)
	}
	if got.WorkspaceID != "ws_1" || got.URL != "/thoughts/?chat_workspace=ws_1" {
		t.Fatalf("OpenChatWorkspace(valid candidate) = %+v, want ws_1 thoughts URL", got)
	}
	if opener.input.RootDocPath != planRoot ||
		opener.input.UserEmail != "user@example.com" {
		t.Fatalf("opener input = %+v, want selected absolute root/user", opener.input)
	}
}

type fakeChatWorkspaceOpener struct {
	input     ChatWorkspaceOpenInput
	workspace db.Workspace
}

func (f *fakeChatWorkspaceOpener) GetOrCreateWorkspaceForRootDocPath(
	ctx context.Context,
	input ChatWorkspaceOpenInput,
) (db.Workspace, error) {
	_ = ctx
	f.input = input
	workspace := f.workspace
	workspace.RootDocPath = input.RootDocPath
	return workspace, nil
}
