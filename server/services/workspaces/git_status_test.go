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

func TestMergeTruthRefPrefersOriginTrunk(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"":               "origin/main",
		"main":           "origin/main",
		"develop":        "origin/develop",
		"origin/release": "origin/release",
	}
	for input, want := range cases {
		if got := mergeTruthRef(input); got != want {
			t.Fatalf("mergeTruthRef(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestInspectImplWorkspaceGitDoesNotTreatLocalTrunkAsCleanupProof(t *testing.T) {
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
	if state.Merged || state.MergeRef != "" || state.MergeProof.Kind != MergeProofUnknown {
		t.Fatalf(
			"merged=%v ref=%q proof=%q, want no cleanup proof from local develop",
			state.Merged,
			state.MergeRef,
			state.MergeProof.Kind,
		)
	}
}

func TestInspectMergeProofUsesOriginMainAncestor(t *testing.T) {
	checkout := t.TempDir()
	if err := os.WriteFile(filepath.Join(checkout, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runImplSyncGit(t, checkout, "init", "-b", "main")
	runImplSyncGit(t, checkout, "config", "user.email", "test@example.test")
	runImplSyncGit(t, checkout, "config", "user.name", "Test User")
	runImplSyncGit(t, checkout, "add", "README.md")
	runImplSyncGit(t, checkout, "commit", "-m", "initial")
	origin := filepath.Join(t.TempDir(), "origin.git")
	runImplSyncGit(t, checkout, "init", "--bare", origin)
	runImplSyncGit(t, checkout, "remote", "add", "origin", origin)
	runImplSyncGit(t, checkout, "push", "-u", "origin", "main")
	runImplSyncGit(t, checkout, "checkout", "-b", "feature")

	proof := InspectMergeProof(context.Background(), checkout, "main")
	if proof.Kind != MergeProofAncestor || proof.SourceRef != "origin/main" || proof.TargetCommit == "" {
		t.Fatalf("proof = %+v, want ancestor against origin/main", proof)
	}
}

func TestInspectMergeProofUsesPatchEquivalent(t *testing.T) {
	checkout := t.TempDir()
	if err := os.WriteFile(filepath.Join(checkout, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runImplSyncGit(t, checkout, "init", "-b", "main")
	runImplSyncGit(t, checkout, "config", "user.email", "test@example.test")
	runImplSyncGit(t, checkout, "config", "user.name", "Test User")
	runImplSyncGit(t, checkout, "add", "README.md")
	runImplSyncGit(t, checkout, "commit", "-m", "initial")
	origin := filepath.Join(t.TempDir(), "origin.git")
	runImplSyncGit(t, checkout, "init", "--bare", origin)
	runImplSyncGit(t, checkout, "remote", "add", "origin", origin)
	runImplSyncGit(t, checkout, "push", "-u", "origin", "main")
	runImplSyncGit(t, checkout, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(checkout, "feature.txt"), []byte("same patch\n"), 0o644); err != nil {
		t.Fatalf("write feature: %v", err)
	}
	runImplSyncGit(t, checkout, "add", "feature.txt")
	runImplSyncGit(t, checkout, "commit", "-m", "feature")
	runImplSyncGit(t, checkout, "checkout", "main")
	if err := os.WriteFile(filepath.Join(checkout, "feature.txt"), []byte("same patch\n"), 0o644); err != nil {
		t.Fatalf("write feature on main: %v", err)
	}
	runImplSyncGit(t, checkout, "add", "feature.txt")
	runImplSyncGit(t, checkout, "commit", "-m", "equivalent")
	runImplSyncGit(t, checkout, "push", "origin", "main")
	runImplSyncGit(t, checkout, "checkout", "feature")

	proof := InspectMergeProof(context.Background(), checkout, "main")
	if proof.Kind != MergeProofPatchEquivalent || proof.SourceRef != "origin/main" {
		t.Fatalf("proof = %+v, want patch-equivalent against origin/main", proof)
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
