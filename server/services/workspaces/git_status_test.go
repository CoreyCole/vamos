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

func TestInspectMergeProofMultiPeerAncestorWins(t *testing.T) {
	isolateImplSyncGitPath(t)
	parent := t.TempDir()
	subject := filepath.Join(parent, "subject")
	peer := filepath.Join(parent, "peer")
	for _, dir := range []string{subject, peer} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(subject, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runImplSyncGit(t, subject, "init", "-b", "main")
	runImplSyncGit(t, subject, "config", "user.email", "test@example.test")
	runImplSyncGit(t, subject, "config", "user.name", "Test User")
	runImplSyncGit(t, subject, "add", "README.md")
	runImplSyncGit(t, subject, "commit", "-m", "initial")
	runImplSyncGit(t, parent, "clone", subject, peer)
	runImplSyncGit(t, peer, "config", "user.email", "test@example.test")
	runImplSyncGit(t, peer, "config", "user.name", "Test User")
	runImplSyncGit(t, subject, "checkout", "-b", "feature")
	subjectHead := strings.TrimSpace(runImplSyncGit(t, subject, "rev-parse", "--short", "HEAD"))
	if err := os.WriteFile(filepath.Join(peer, "peer.txt"), []byte("peer advance\n"), 0o644); err != nil {
		t.Fatalf("write peer: %v", err)
	}
	runImplSyncGit(t, peer, "add", "peer.txt")
	runImplSyncGit(t, peer, "commit", "-m", "peer tip")
	peerSHA := strings.TrimSpace(runImplSyncGit(t, peer, "rev-parse", "--short", "HEAD"))

	proof := InspectMergeProofMulti(context.Background(), subject, []MergeProofTarget{{
		SourceRef:    "peer:2026-09-08-10-10-54-agent-memory-observable-context@" + peerSHA,
		Commit:       peerSHA,
		CheckoutPath: peer,
		GitRef:       "HEAD",
	}})
	wantRef := "peer:2026-09-08-10-10-54-agent-memory-observable-context@" + peerSHA
	if proof.Kind != MergeProofAncestor || proof.SourceRef != wantRef || proof.TargetCommit != peerSHA {
		t.Fatalf("proof = %+v, want ancestor %s (subject %s)", proof, wantRef, subjectHead)
	}
}

func TestInspectMergeProofMultiCheckoutMainAndStage(t *testing.T) {
	isolateImplSyncGitPath(t)
	parent := t.TempDir()
	subject := filepath.Join(parent, "subject")
	mainCheckout := filepath.Join(parent, "vamos-main")
	stageCheckout := filepath.Join(parent, "vamos")
	for _, dir := range []string{subject, mainCheckout, stageCheckout} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(subject, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runImplSyncGit(t, subject, "init", "-b", "main")
	runImplSyncGit(t, subject, "config", "user.email", "test@example.test")
	runImplSyncGit(t, subject, "config", "user.name", "Test User")
	runImplSyncGit(t, subject, "add", "README.md")
	runImplSyncGit(t, subject, "commit", "-m", "initial")
	runImplSyncGit(t, parent, "clone", subject, mainCheckout)
	runImplSyncGit(t, parent, "clone", subject, stageCheckout)
	runImplSyncGit(t, mainCheckout, "config", "user.email", "test@example.test")
	runImplSyncGit(t, mainCheckout, "config", "user.name", "Test User")
	runImplSyncGit(t, stageCheckout, "config", "user.email", "test@example.test")
	runImplSyncGit(t, stageCheckout, "config", "user.name", "Test User")
	runImplSyncGit(t, subject, "checkout", "-b", "feature")

	if err := os.WriteFile(filepath.Join(mainCheckout, "main-only.txt"), []byte("main tip\n"), 0o644); err != nil {
		t.Fatalf("write main tip: %v", err)
	}
	runImplSyncGit(t, mainCheckout, "add", "main-only.txt")
	runImplSyncGit(t, mainCheckout, "commit", "-m", "main tip")
	mainSHA := strings.TrimSpace(runImplSyncGit(t, mainCheckout, "rev-parse", "--short", "HEAD"))

	proofMain := InspectMergeProofMulti(context.Background(), subject, []MergeProofTarget{{
		SourceRef:    "checkout:main",
		Commit:       mainSHA,
		CheckoutPath: mainCheckout,
		GitRef:       "HEAD",
	}})
	if proofMain.Kind != MergeProofAncestor || proofMain.SourceRef != "checkout:main" {
		t.Fatalf("checkout:main proof = %+v", proofMain)
	}

	if err := os.WriteFile(filepath.Join(stageCheckout, "stage-only.txt"), []byte("stage tip\n"), 0o644); err != nil {
		t.Fatalf("write stage tip: %v", err)
	}
	runImplSyncGit(t, stageCheckout, "add", "stage-only.txt")
	runImplSyncGit(t, stageCheckout, "commit", "-m", "stage tip")
	stageSHA := strings.TrimSpace(runImplSyncGit(t, stageCheckout, "rev-parse", "--short", "HEAD"))

	proofStage := InspectMergeProofMulti(context.Background(), subject, []MergeProofTarget{{
		SourceRef:    "checkout:stage",
		Commit:       stageSHA,
		CheckoutPath: stageCheckout,
		GitRef:       "HEAD",
	}})
	if proofStage.Kind != MergeProofAncestor || proofStage.SourceRef != "checkout:stage" {
		t.Fatalf("checkout:stage proof = %+v", proofStage)
	}
}

func TestInspectMergeProofMultiIgnoresBareLocalMain(t *testing.T) {
	isolateImplSyncGitPath(t)
	checkout := t.TempDir()
	if err := os.WriteFile(filepath.Join(checkout, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runImplSyncGit(t, checkout, "init", "-b", "main")
	runImplSyncGit(t, checkout, "config", "user.email", "test@example.test")
	runImplSyncGit(t, checkout, "config", "user.name", "Test User")
	runImplSyncGit(t, checkout, "add", "README.md")
	runImplSyncGit(t, checkout, "commit", "-m", "initial")
	runImplSyncGit(t, checkout, "checkout", "-b", "feature")
	mainSHA := strings.TrimSpace(runImplSyncGit(t, checkout, "rev-parse", "--short", "main"))

	proof := InspectMergeProofMulti(context.Background(), checkout, []MergeProofTarget{{
		SourceRef:    "main",
		Commit:       mainSHA,
		CheckoutPath: checkout,
		GitRef:       "main",
	}})
	if proof.Kind != MergeProofUnknown || proof.Strong() {
		t.Fatalf("proof = %+v, want unknown for bare local main", proof)
	}
}

func TestMergeProofKeeperTokensMatchDogfoodSlugs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		slug string
		want bool
	}{
		{"2026-09-08-10-10-54-agent-memory-observable-context", true},
		{"2026-09-12-14-30-21-household-rebalancer", true},
		{"2026-09-11-21-25-37-datastar-sse-ui-declaration", true},
		{"2026-09-12-13-33-37-instant-plan-chat-send", false},
		{"main", false},
	}
	for _, tc := range cases {
		got := matchesMergeProofKeeperToken(tc.slug, "vamos-"+tc.slug, tc.slug)
		if got != tc.want {
			t.Fatalf("matchesMergeProofKeeperToken(%q) = %v, want %v", tc.slug, got, tc.want)
		}
	}
	if !isMergeProofKeeperWorkspace(Workspace{Slug: "main", IsMain: true}) {
		t.Fatal("main workspace should be keeper")
	}
	if !isMergeProofKeeperWorkspace(Workspace{Slug: "stage", CheckoutRole: CheckoutRoleStage}) {
		t.Fatal("stage workspace should be keeper")
	}
}
