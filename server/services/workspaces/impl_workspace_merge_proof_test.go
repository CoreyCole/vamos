package workspaces

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/db"
)

func TestImplWorkspaceSyncerMarksMergedFromPeerAncestorOnly(t *testing.T) {
	isolateImplSyncGitPath(t)
	ctx := context.Background()
	parent := t.TempDir()

	shared := filepath.Join(t.TempDir(), "shared.git")
	runImplSyncGit(t, t.TempDir(), "init", "--bare", shared)

	subject := makeImplSyncCheckout(t, parent, "vamos-2026-09-12_13-33-37_instant-plan-chat-send")
	peer := makeImplSyncCheckout(t, parent, "vamos-2026-09-08_10-10-54_agent-memory-observable-context")
	seedRelatedImplSyncRepos(t, shared, subject, peer)

	// Advance peer beyond origin/main; subject follows once then peer advances again.
	mid := addImplSyncGitCommit(t, peer, "peer-mid.txt", "mid\n", "peer mid")
	fetchFastForwardFrom(t, subject, peer)
	peerSHA := addImplSyncGitCommit(t, peer, "peer-advance.txt", "peer tip\n", "peer advance")
	runImplSyncGit(t, subject, "checkout", "-B", "feature")
	_ = mid

	queries := openImplSyncTestQueries(t)
	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(ctx, ImplWorkspaceSyncInput{
		Discovery: ImplWorkspaceDiscoveryConfig{
			ParentDir: parent,
			Domain:    "workspaces.example.test",
		},
		TrunkBranch: "main",
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Merged < 1 {
		t.Fatalf("result = %+v, want subject merged via peer (mid=%s)", result, mid)
	}

	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{
		WorkspaceSlug: "2026-09-12-13-33-37-instant-plan-chat-send",
	})
	if err != nil {
		t.Fatalf("GetImplWorkspace(subject): %v", err)
	}
	wantRef := "peer:2026-09-08-10-10-54-agent-memory-observable-context@" + peerSHA
	if row.Status != string(ImplWorkspaceStatusMerged) ||
		row.CleanupProofKind != string(MergeProofAncestor) ||
		row.CleanupProofSourceRef.String != wantRef {
		t.Fatalf("subject row status=%q kind=%q source=%q, want merged %s",
			row.Status, row.CleanupProofKind, row.CleanupProofSourceRef.String, wantRef)
	}

	keeper, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{
		WorkspaceSlug: "2026-09-08-10-10-54-agent-memory-observable-context",
	})
	if err != nil {
		t.Fatalf("GetImplWorkspace(keeper): %v", err)
	}
	if keeper.Status != string(ImplWorkspaceStatusActive) {
		t.Fatalf("keeper status=%q, want active", keeper.Status)
	}
}

func TestImplWorkspaceSyncerMarksMergedFromCheckoutMainAndStage(t *testing.T) {
	isolateImplSyncGitPath(t)
	ctx := context.Background()
	parent := t.TempDir()

	mainCheckout := makeImplSyncCheckout(t, parent, "vamos-main")
	stageCheckout := makeImplSyncCheckout(t, parent, "stage-checkout")
	subject := makeImplSyncCheckout(t, parent, "vamos-2026-09-12_13-33-37_instant-plan-chat-send")

	// No origin remote: origin/main must not steal the strong hit.
	initImplSyncGitRepoNoOrigin(t, mainCheckout)
	cloneImplSyncRepoInto(t, mainCheckout, stageCheckout)
	cloneImplSyncRepoInto(t, mainCheckout, subject)

	runImplSyncGit(t, subject, "checkout", "-b", "feature")
	mainSHA := addImplSyncGitCommit(t, mainCheckout, "main-tip.txt", "main tip\n", "main tip")

	queries := openImplSyncTestQueries(t)
	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(ctx, ImplWorkspaceSyncInput{
		Discovery: ImplWorkspaceDiscoveryConfig{
			ParentDir:        parent,
			MainCheckoutPath: mainCheckout,
			MainCheckoutName: "vamos-main",
			Domain:           "workspaces.example.test",
			ConfiguredCheckouts: map[string]ConfiguredCheckout{
				"main": {
					RootPath:    mainCheckout,
					DisplayName: "Main",
					IsMain:      true,
					Role:        CheckoutRoleMain,
				},
				"stage": {
					RootPath:    stageCheckout,
					DisplayName: "Stage",
					Role:        CheckoutRoleStage,
				},
			},
		},
		TrunkBranch: "main",
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Merged < 1 {
		t.Fatalf("result = %+v, want subject merged via checkout:main (%s)", result, mainSHA)
	}
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{
		WorkspaceSlug: "2026-09-12-13-33-37-instant-plan-chat-send",
	})
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusMerged) || row.CleanupProofSourceRef.String != "checkout:main" {
		t.Fatalf("row status=%q source=%q target=%q, want merged checkout:main",
			row.Status, row.CleanupProofSourceRef.String, row.CleanupProofTargetCommit.String)
	}

	parent2 := t.TempDir()
	main2 := makeImplSyncCheckout(t, parent2, "vamos-main")
	stage2 := makeImplSyncCheckout(t, parent2, "stage-checkout")
	subject2 := makeImplSyncCheckout(t, parent2, "vamos-2026-09-12_11-50-20_datastarui-toast-copy")
	initImplSyncGitRepoNoOrigin(t, main2)
	cloneImplSyncRepoInto(t, main2, stage2)
	cloneImplSyncRepoInto(t, main2, subject2)
	// Put subject on stage-only lineage so checkout:main (still at base) is not a hit.
	mid := addImplSyncGitCommit(t, stage2, "stage-mid.txt", "stage mid\n", "stage mid")
	fetchFastForwardFrom(t, subject2, stage2)
	_ = addImplSyncGitCommit(t, stage2, "stage-tip.txt", "stage tip\n", "stage tip")
	runImplSyncGit(t, subject2, "checkout", "-B", "feature")
	_ = mid

	queries2 := openImplSyncTestQueries(t)
	result2, err := (&ImplWorkspaceSyncer{Queries: queries2}).Sync(ctx, ImplWorkspaceSyncInput{
		Discovery: ImplWorkspaceDiscoveryConfig{
			ParentDir:        parent2,
			MainCheckoutPath: main2,
			MainCheckoutName: "vamos-main",
			Domain:           "workspaces.example.test",
			ConfiguredCheckouts: map[string]ConfiguredCheckout{
				"main": {
					RootPath: main2, DisplayName: "Main", IsMain: true, Role: CheckoutRoleMain,
				},
				"stage": {
					RootPath: stage2, DisplayName: "Stage", Role: CheckoutRoleStage,
				},
			},
		},
		TrunkBranch: "main",
	})
	if err != nil {
		t.Fatalf("Sync stage case: %v", err)
	}
	row2, err := queries2.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{
		WorkspaceSlug: "2026-09-12-11-50-20-datastarui-toast-copy",
	})
	if err != nil {
		t.Fatalf("GetImplWorkspace(stage case): %v", err)
	}
	if result2.Merged < 1 || row2.Status != string(ImplWorkspaceStatusMerged) ||
		row2.CleanupProofSourceRef.String != "checkout:stage" {
		t.Fatalf("stage case result=%+v row status=%q source=%q, want checkout:stage",
			result2, row2.Status, row2.CleanupProofSourceRef.String)
	}
}

func TestImplWorkspaceSyncerKeepsActiveWhenOnlyLocalMainBranchMatches(t *testing.T) {
	isolateImplSyncGitPath(t)
	ctx := context.Background()
	parent := t.TempDir()
	checkout := makeImplSyncCheckout(t, parent, "vamos-2026-09-12_13-33-37_instant-plan-chat-send")

	runImplSyncGit(t, checkout, "init", "-b", "main")
	runImplSyncGit(t, checkout, "config", "user.email", "test@example.test")
	runImplSyncGit(t, checkout, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(checkout, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runImplSyncGit(t, checkout, "add", "README.md", "pkg/agents/go.mod")
	runImplSyncGit(t, checkout, "commit", "-m", "initial")
	origin := filepath.Join(t.TempDir(), "origin.git")
	runImplSyncGit(t, checkout, "init", "--bare", origin)
	runImplSyncGit(t, checkout, "remote", "add", "origin", origin)
	runImplSyncGit(t, checkout, "push", "-u", "origin", "main")
	runImplSyncGit(t, checkout, "checkout", "-b", "feature")
	addImplSyncGitCommit(t, checkout, "feature.txt", "feature only\n", "feature")
	runImplSyncGit(t, checkout, "checkout", "main")
	runImplSyncGit(t, checkout, "merge", "--ff-only", "feature")
	runImplSyncGit(t, checkout, "checkout", "feature")

	queries := openImplSyncTestQueries(t)
	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(ctx, ImplWorkspaceSyncInput{
		Discovery: ImplWorkspaceDiscoveryConfig{
			ParentDir: parent,
			Domain:    "workspaces.example.test",
		},
		TrunkBranch: "main",
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{
		WorkspaceSlug: "2026-09-12-13-33-37-instant-plan-chat-send",
	})
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusActive) || row.CleanupProofKind != string(MergeProofUnknown) {
		t.Fatalf("result=%+v row status=%q kind=%q source=%q, want active unknown (local main is not strong)",
			result, row.Status, row.CleanupProofKind, row.CleanupProofSourceRef.String)
	}
}

func TestImplWorkspaceSyncerKeepersNeverLeaveActiveViaPeerOrCheckout(t *testing.T) {
	isolateImplSyncGitPath(t)
	ctx := context.Background()
	parent := t.TempDir()
	shared := filepath.Join(t.TempDir(), "shared.git")
	runImplSyncGit(t, t.TempDir(), "init", "--bare", shared)

	mainCheckout := makeImplSyncCheckout(t, parent, "vamos-main")
	am := makeImplSyncCheckout(t, parent, "vamos-2026-09-08_10-10-54_agent-memory-observable-context")
	rebalancer := makeImplSyncCheckout(t, parent, "vamos-2026-09-12_14-30-21_household-rebalancer")
	datastar := makeImplSyncCheckout(t, parent, "vamos-2026-09-11_21-25-37_datastar-sse-ui-declaration")
	peer := makeImplSyncCheckout(t, parent, "vamos-2026-09-12_13-33-37_instant-plan-chat-send")

	seedRelatedImplSyncRepos(t, shared, mainCheckout, am, rebalancer, datastar, peer)
	addImplSyncGitCommit(t, peer, "peer-tip.txt", "peer\n", "peer tip")
	for _, keeperPath := range []string{am, rebalancer, datastar} {
		runImplSyncGit(t, keeperPath, "checkout", "-b", "feature")
	}

	queries := openImplSyncTestQueries(t)
	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(ctx, ImplWorkspaceSyncInput{
		Discovery: ImplWorkspaceDiscoveryConfig{
			ParentDir:        parent,
			MainCheckoutPath: mainCheckout,
			MainCheckoutName: "vamos-main",
			Domain:           "workspaces.example.test",
			ConfiguredCheckouts: map[string]ConfiguredCheckout{
				"main": {RootPath: mainCheckout, DisplayName: "Main", IsMain: true, Role: CheckoutRoleMain},
			},
		},
		TrunkBranch: "main",
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	for _, slug := range []string{
		"2026-09-08-10-10-54-agent-memory-observable-context",
		"2026-09-12-14-30-21-household-rebalancer",
		"2026-09-11-21-25-37-datastar-sse-ui-declaration",
		"main",
	} {
		row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{WorkspaceSlug: slug})
		if err != nil {
			t.Fatalf("GetImplWorkspace(%s): %v", slug, err)
		}
		if row.Status != string(ImplWorkspaceStatusActive) {
			t.Fatalf("keeper %s status=%q source=%q result=%+v, want active",
				slug, row.Status, row.CleanupProofSourceRef.String, result)
		}
	}
}

func TestImplWorkspaceSyncerMissingCheckoutPeerAndCheckoutParity(t *testing.T) {
	isolateImplSyncGitPath(t)
	ctx := context.Background()
	parent := t.TempDir()
	shared := filepath.Join(t.TempDir(), "shared.git")
	runImplSyncGit(t, t.TempDir(), "init", "--bare", shared)

	mainCheckout := makeImplSyncCheckout(t, parent, "vamos-main")
	stageCheckout := makeImplSyncCheckout(t, parent, "stage-checkout")
	peer := makeImplSyncCheckout(t, parent, "vamos-2026-09-08_10-10-54_agent-memory-observable-context")
	seedRelatedImplSyncRepos(t, shared, mainCheckout, stageCheckout, peer)

	baseCommit := strings.TrimSpace(runImplSyncGit(t, mainCheckout, "rev-parse", "--short", "HEAD"))
	peerSHA := addImplSyncGitCommit(t, peer, "peer-tip.txt", "peer\n", "peer tip")

	queries := openImplSyncTestQueries(t)
	_, err := queries.UpsertDiscoveredImplWorkspace(ctx, db.UpsertDiscoveredImplWorkspaceParams{
		WorkspaceSlug: "missing-instant-plan",
		CheckoutPath:  filepath.Join(parent, "vamos-2026-09-12_13-33-37_instant-plan-chat-send"),
		DisplayName:   "missing instant plan",
		Host:          "missing-instant-plan.workspaces.example.test",
		Url:           "https://missing-instant-plan.workspaces.example.test/",
		Status:        string(ImplWorkspaceStatusActive),
		CommitHash:    nullableString(baseCommit),
		TrunkBranch:   nullableString("main"),
	})
	if err != nil {
		t.Fatalf("seed missing: %v", err)
	}

	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(ctx, ImplWorkspaceSyncInput{
		Discovery: ImplWorkspaceDiscoveryConfig{
			ParentDir:        parent,
			MainCheckoutPath: mainCheckout,
			MainCheckoutName: "vamos-main",
			Domain:           "workspaces.example.test",
			ConfiguredCheckouts: map[string]ConfiguredCheckout{
				"main":  {RootPath: mainCheckout, DisplayName: "Main", IsMain: true, Role: CheckoutRoleMain},
				"stage": {RootPath: stageCheckout, DisplayName: "Stage", Role: CheckoutRoleStage},
			},
		},
		TrunkBranch: "main",
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{WorkspaceSlug: "missing-instant-plan"})
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if result.Merged < 1 || row.Status != string(ImplWorkspaceStatusMerged) {
		t.Fatalf("result=%+v row=%q/%q, want missing merged", result, row.Status, row.CleanupProofSourceRef.String)
	}
	if row.CleanupProofSourceRef.String != "origin/main" &&
		row.CleanupProofSourceRef.String != "checkout:main" &&
		row.CleanupProofSourceRef.String != "checkout:stage" &&
		!strings.HasPrefix(row.CleanupProofSourceRef.String, "peer:2026-09-08-10-10-54-agent-memory-observable-context@") {
		t.Fatalf("missing source=%q peerSHA=%s, want main/stage/peer parity target", row.CleanupProofSourceRef.String, peerSHA)
	}

	parent2 := t.TempDir()
	shared2 := filepath.Join(t.TempDir(), "shared2.git")
	runImplSyncGit(t, t.TempDir(), "init", "--bare", shared2)
	main2 := makeImplSyncCheckout(t, parent2, "vamos-main")
	peer2 := makeImplSyncCheckout(t, parent2, "vamos-2026-09-08_10-10-54_agent-memory-observable-context")
	seedRelatedImplSyncRepos(t, shared2, main2, peer2)
	mid := addImplSyncGitCommit(t, peer2, "only-peer.txt", "only peer\n", "only on peer lineage")
	peerTip := addImplSyncGitCommit(t, peer2, "peer-further.txt", "further\n", "peer further")

	queries2 := openImplSyncTestQueries(t)
	_, err = queries2.UpsertDiscoveredImplWorkspace(ctx, db.UpsertDiscoveredImplWorkspaceParams{
		WorkspaceSlug: "missing-peer-only",
		CheckoutPath:  filepath.Join(parent2, "vamos-missing-peer-only"),
		DisplayName:   "missing peer only",
		Host:          "missing-peer-only.workspaces.example.test",
		Url:           "https://missing-peer-only.workspaces.example.test/",
		Status:        string(ImplWorkspaceStatusActive),
		CommitHash:    nullableString(mid),
		TrunkBranch:   nullableString("main"),
	})
	if err != nil {
		t.Fatalf("seed missing peer-only: %v", err)
	}
	result2, err := (&ImplWorkspaceSyncer{Queries: queries2}).Sync(ctx, ImplWorkspaceSyncInput{
		Discovery: ImplWorkspaceDiscoveryConfig{
			ParentDir:        parent2,
			MainCheckoutPath: main2,
			MainCheckoutName: "vamos-main",
			Domain:           "workspaces.example.test",
			ConfiguredCheckouts: map[string]ConfiguredCheckout{
				"main": {RootPath: main2, DisplayName: "Main", IsMain: true, Role: CheckoutRoleMain},
			},
		},
		TrunkBranch: "main",
	})
	if err != nil {
		t.Fatalf("Sync peer-only: %v", err)
	}
	row2, err := queries2.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{WorkspaceSlug: "missing-peer-only"})
	if err != nil {
		t.Fatalf("GetImplWorkspace(peer-only): %v", err)
	}
	wantPeer := "peer:2026-09-08-10-10-54-agent-memory-observable-context@" + peerTip
	if result2.Merged < 1 || row2.Status != string(ImplWorkspaceStatusMerged) || row2.CleanupProofSourceRef.String != wantPeer {
		t.Fatalf("peer-only result=%+v status=%q source=%q, want %s", result2, row2.Status, row2.CleanupProofSourceRef.String, wantPeer)
	}
}

func seedRelatedImplSyncRepos(t *testing.T, bare string, checkouts ...string) {
	t.Helper()
	if len(checkouts) == 0 {
		t.Fatal("seedRelatedImplSyncRepos requires checkouts")
	}
	first := checkouts[0]
	runImplSyncGit(t, first, "init", "-b", "main")
	runImplSyncGit(t, first, "config", "user.email", "test@example.test")
	runImplSyncGit(t, first, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(first, "README.md"), []byte("shared\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runImplSyncGit(t, first, "add", "README.md", "pkg/agents/go.mod")
	runImplSyncGit(t, first, "commit", "-m", "initial")
	runImplSyncGit(t, first, "remote", "add", "origin", bare)
	runImplSyncGit(t, first, "push", "-u", "origin", "main")
	for _, checkout := range checkouts[1:] {
		name := filepath.Base(checkout)
		tmp := filepath.Join(t.TempDir(), name)
		runImplSyncGit(t, t.TempDir(), "clone", bare, tmp)
		runImplSyncGit(t, tmp, "config", "user.email", "test@example.test")
		runImplSyncGit(t, tmp, "config", "user.name", "Test User")
		_ = os.RemoveAll(checkout)
		if err := os.Rename(tmp, checkout); err != nil {
			t.Fatalf("rename clone into %s: %v", checkout, err)
		}
		runImplSyncGit(t, checkout, "checkout", "-B", "main", "origin/main")
		_ = os.MkdirAll(filepath.Join(checkout, "pkg", "agents"), 0o755)
		_ = os.WriteFile(filepath.Join(checkout, "pkg/agents/go.mod"), []byte("module github.com/CoreyCole/vamos\n"), 0o644)
	}
}

func initImplSyncGitRepoNoOrigin(t *testing.T, checkout string) string {
	t.Helper()
	runImplSyncGit(t, checkout, "init", "-b", "main")
	runImplSyncGit(t, checkout, "config", "user.email", "test@example.test")
	runImplSyncGit(t, checkout, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(checkout, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runImplSyncGit(t, checkout, "add", "README.md", "pkg/agents/go.mod")
	runImplSyncGit(t, checkout, "commit", "-m", "initial")
	return strings.TrimSpace(runImplSyncGit(t, checkout, "rev-parse", "--short", "HEAD"))
}

func cloneImplSyncRepoInto(t *testing.T, source, dest string) {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), filepath.Base(dest))
	runImplSyncGit(t, t.TempDir(), "clone", source, tmp)
	runImplSyncGit(t, tmp, "config", "user.email", "test@example.test")
	runImplSyncGit(t, tmp, "config", "user.name", "Test User")
	_ = os.RemoveAll(dest)
	if err := os.Rename(tmp, dest); err != nil {
		t.Fatalf("rename clone into %s: %v", dest, err)
	}
	runImplSyncGit(t, dest, "checkout", "-B", "main")
	_ = os.MkdirAll(filepath.Join(dest, "pkg", "agents"), 0o755)
	_ = os.WriteFile(filepath.Join(dest, "pkg/agents/go.mod"), []byte("module github.com/CoreyCole/vamos\n"), 0o644)
}

func fetchFastForwardFrom(t *testing.T, dest, source string) {
	t.Helper()
	cmd := exec.Command("git", "remote", "remove", "syncpeer")
	cmd.Dir = dest
	_ = cmd.Run()
	runImplSyncGit(t, dest, "remote", "add", "syncpeer", source)
	runImplSyncGit(t, dest, "fetch", "syncpeer", "HEAD:refs/remotes/syncpeer/tip")
	runImplSyncGit(t, dest, "merge", "--ff-only", "syncpeer/tip")
}
