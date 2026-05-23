package workspaces

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectStackEmptyCheckoutIsUnavailable(t *testing.T) {
	t.Parallel()

	summary := InspectStack(context.Background(), "")
	if summary.Available {
		t.Fatalf("Available = true, want false")
	}
	if !strings.Contains(summary.Detail, "empty") {
		t.Fatalf("Detail = %q, want empty-path detail", summary.Detail)
	}
}

func TestCheckoutMergeRefCandidatesPreferConfiguredTrunk(t *testing.T) {
	t.Parallel()

	refs := checkoutMergeRefCandidates("develop")
	want := []string{"origin/develop", "develop", "origin/main", "main"}
	if strings.Join(refs, ",") != strings.Join(want, ",") {
		t.Fatalf("refs = %#v, want %#v", refs, want)
	}
}

func TestInspectImplWorkspaceGitUsesConfiguredLocalTrunkFallback(t *testing.T) {
	checkout := t.TempDir()
	if err := os.MkdirAll(filepath.Join(checkout, "pkg", "agents"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(checkout, "README.md"),
		[]byte("test\n"),
		0o644,
	); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runImplSyncGit(t, checkout, "init", "-b", "develop")
	runImplSyncGit(t, checkout, "config", "user.email", "test@example.test")
	runImplSyncGit(t, checkout, "config", "user.name", "Test User")
	runImplSyncGit(t, checkout, "add", "README.md")
	runImplSyncGit(t, checkout, "commit", "-m", "initial")

	state := InspectImplWorkspaceGit(context.Background(), checkout, "develop")
	if !state.Merged || state.MergeRef != "develop" {
		t.Fatalf(
			"merged=%v ref=%q, want merged against develop",
			state.Merged,
			state.MergeRef,
		)
	}
}

func TestGraphiteStackBranchesReturnsTopAndBottom(t *testing.T) {
	t.Parallel()

	log := "◉ feature_slice-3\n│ ◯ feature_slice-2\n│ ◯ feature_slice-1 (needs restack)\n│ ◯ main\n"
	top, bottom := graphiteStackBranches(log, "main", "feature_slice-3")
	if top != "feature_slice-3" {
		t.Fatalf("top branch = %q, want feature_slice-3", top)
	}
	if bottom != "feature_slice-1" {
		t.Fatalf("bottom branch = %q, want feature_slice-1", bottom)
	}
}

func TestInspectImplWorkspaceGitFallsBackToCurrentBranchStack(t *testing.T) {
	checkout := t.TempDir()
	if err := os.MkdirAll(filepath.Join(checkout, "pkg", "agents"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(checkout, "README.md"),
		[]byte("test\n"),
		0o644,
	); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runImplSyncGit(t, checkout, "init", "-b", "feature")
	runImplSyncGit(t, checkout, "config", "user.email", "test@example.test")
	runImplSyncGit(t, checkout, "config", "user.name", "Test User")
	runImplSyncGit(t, checkout, "add", "README.md")
	runImplSyncGit(t, checkout, "commit", "-m", "initial")
	commit := strings.TrimSpace(
		runImplSyncGit(t, checkout, "rev-parse", "--short", "HEAD"),
	)

	state := InspectImplWorkspaceGit(context.Background(), checkout, "develop")
	if state.Branch != "feature" || state.TopBranch != "feature" ||
		state.BottomBranch != "feature" {
		t.Fatalf("state branches = %+v, want current branch fallback", state)
	}
	if state.Commit != commit {
		t.Fatalf("commit = %q, want %q", state.Commit, commit)
	}
	if state.TrunkBranch != "develop" {
		t.Fatalf("trunk = %q, want configured fallback", state.TrunkBranch)
	}
}

func TestGraphiteStackBranchesAllowsTrunkAsTop(t *testing.T) {
	t.Parallel()

	log := "◯ child-workspace-top\n│ ◉ parent-workspace-top\n"
	top, bottom := graphiteStackBranches(
		log,
		"parent-workspace-top",
		"parent-workspace-top",
	)
	if top != "child-workspace-top" {
		t.Fatalf("top branch = %q, want child-workspace-top", top)
	}
	if bottom != "child-workspace-top" {
		t.Fatalf("bottom branch = %q, want child-workspace-top", bottom)
	}
}
