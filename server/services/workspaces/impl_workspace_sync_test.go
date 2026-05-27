package workspaces

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/CoreyCole/vamos/pkg/db"
)

func TestImplWorkspaceProofFieldsRoundTrip(t *testing.T) {
	ctx := context.Background()
	queries := openImplSyncTestQueries(t)
	proofAt := time.Now().UTC().Truncate(time.Second)

	row, err := queries.UpsertDiscoveredImplWorkspace(
		ctx,
		db.UpsertDiscoveredImplWorkspaceParams{
			WorkspaceSlug:            "proof",
			CheckoutPath:             "/tmp/proof",
			DisplayName:              "Proof",
			Host:                     "proof.workspaces.example.test",
			Url:                      "https://proof.workspaces.example.test/",
			Status:                   string(ImplWorkspaceStatusMerged),
			MergeEvidence:            nullableString("active checkout HEAD abc123 is ancestor of origin/main"),
			CleanupProofKind:         "ancestor",
			CleanupProofSourceRef:    nullableString("origin/main"),
			CleanupProofTargetCommit: nullableString("abc123"),
			CleanupProofAt:           sql.NullTime{Time: proofAt, Valid: true},
			CleanupRiskReason:        nullableString(""),
		},
	)
	if err != nil {
		t.Fatalf("UpsertDiscoveredImplWorkspace: %v", err)
	}
	if row.CleanupProofKind != "ancestor" || row.CleanupProofSourceRef.String != "origin/main" || row.CleanupProofTargetCommit.String != "abc123" || !row.CleanupProofAt.Valid {
		t.Fatalf("proof fields = kind %q source %+v target %+v at %+v, want ancestor origin/main abc123 valid time", row.CleanupProofKind, row.CleanupProofSourceRef, row.CleanupProofTargetCommit, row.CleanupProofAt)
	}
}

func TestImplWorkspaceCleanedUpMarksMergedRows(t *testing.T) {
	ctx := context.Background()
	queries := openImplSyncTestQueries(t)
	_, err := queries.UpsertDiscoveredImplWorkspace(
		ctx,
		db.UpsertDiscoveredImplWorkspaceParams{
			WorkspaceSlug:         "merged",
			CheckoutPath:          "/tmp/merged",
			DisplayName:           "Merged",
			Host:                  "merged.workspaces.example.test",
			Url:                   "https://merged.workspaces.example.test/",
			Status:                string(ImplWorkspaceStatusMerged),
			MergeEvidence:         nullableString("active checkout HEAD abc123 is ancestor of origin/main"),
			CleanupProofKind:      "ancestor",
			CleanupProofSourceRef: nullableString("origin/main"),
		},
	)
	if err != nil {
		t.Fatalf("UpsertDiscoveredImplWorkspace: %v", err)
	}

	n, err := queries.MarkImplWorkspaceCleanedUp(ctx, "merged")
	if err != nil {
		t.Fatalf("MarkImplWorkspaceCleanedUp: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows affected = %d, want 1", n)
	}
	row, err := queries.GetImplWorkspace(ctx, "merged")
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusCleanedUp) || !row.CleanedUpAt.Valid {
		t.Fatalf("row status=%q cleaned_up_at=%v, want cleaned_up with timestamp", row.Status, row.CleanedUpAt)
	}
}

func TestImplWorkspaceSyncIncludesConfiguredCheckouts(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	workDir := makeImplSyncCheckout(t, t.TempDir(), "editable-vamos")
	queries := openImplSyncTestQueries(t)

	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(
		ctx,
		ImplWorkspaceSyncInput{
			Discovery: ImplWorkspaceDiscoveryConfig{
				ParentDir: parent,
				Domain:    "workspaces.example.test",
				ConfiguredCheckouts: map[string]ConfiguredCheckout{
					"work": {
						RootPath:    workDir,
						DisplayName: "Working checkout",
					},
				},
			},
			ManagerURL:   "https://main.workspaces.example.test",
			RestartToken: "secret-token",
		},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Upserted != 1 || result.RepairedEnv != 1 || !result.Changed {
		t.Fatalf("result = %+v, want one configured upsert/env repair/change", result)
	}

	row, err := queries.GetImplWorkspace(ctx, "work")
	if err != nil {
		t.Fatalf("GetImplWorkspace(work): %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusActive) {
		t.Fatalf("status = %q, want active", row.Status)
	}
	if row.CheckoutPath != workDir || row.DisplayName != "Working checkout" {
		t.Fatalf("row = %+v, want configured checkout path/name", row)
	}
	if row.Host != "work.workspaces.example.test" || row.Url != "https://work.workspaces.example.test/" {
		t.Fatalf("host/url = %q/%q", row.Host, row.Url)
	}
}

func TestImplWorkspaceSyncDoesNotCleanupConfiguredCheckoutWhenMissingFromScan(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	queries := openImplSyncTestQueries(t)
	_, err := queries.UpsertDiscoveredImplWorkspace(
		ctx,
		db.UpsertDiscoveredImplWorkspaceParams{
			WorkspaceSlug: "work",
			CheckoutPath:  filepath.Join(parent, "vamos"),
			DisplayName:   "Working checkout",
			Host:          "work.workspaces.example.test",
			Url:           "https://work.workspaces.example.test/",
			Status:        string(ImplWorkspaceStatusActive),
		},
	)
	if err != nil {
		t.Fatalf("UpsertDiscoveredImplWorkspace: %v", err)
	}

	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(
		ctx,
		ImplWorkspaceSyncInput{
			Discovery: ImplWorkspaceDiscoveryConfig{
				ParentDir: parent,
				Domain:    "workspaces.example.test",
				ConfiguredCheckouts: map[string]ConfiguredCheckout{
					"work": {RootPath: filepath.Join(parent, "missing-vamos")},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.CleanedUp != 0 || result.Merged != 0 || result.Changed {
		t.Fatalf("result = %+v, want configured missing checkout preserved", result)
	}
	row, err := queries.GetImplWorkspace(ctx, "work")
	if err != nil {
		t.Fatalf("GetImplWorkspace(work): %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusActive) || row.CleanedUpAt.Valid || row.MergedAt.Valid {
		t.Fatalf("row = %+v, want active configured checkout preserved", row)
	}
}

func TestImplWorkspaceSyncerDoesNotCleanupRowsOutsideDiscoveryScope(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	vamosCheckout := makeImplSyncCheckout(t, parent, "vamos")
	makeImplSyncCheckout(t, parent, "monorepo-feature")
	queries := openImplSyncTestQueries(t)
	_, err := queries.UpsertDiscoveredImplWorkspace(
		ctx,
		db.UpsertDiscoveredImplWorkspaceParams{
			WorkspaceSlug: "stage",
			CheckoutPath:  vamosCheckout,
			DisplayName:   "Stage",
			Host:          "stage.workspaces.example.test",
			Url:           "https://stage.workspaces.example.test/",
			Status:        string(ImplWorkspaceStatusActive),
			Branch:        nullableString("main"),
			CommitHash:    nullableString(gitCommit(ctx, vamosCheckout)),
			TrunkBranch:   nullableString("main"),
		},
	)
	if err != nil {
		t.Fatalf("UpsertDiscoveredImplWorkspace: %v", err)
	}

	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(
		ctx,
		ImplWorkspaceSyncInput{
			Discovery: ImplWorkspaceDiscoveryConfig{
				ParentDir:        parent,
				Domain:           "workspaces.example.test",
				CheckoutPrefixes: []string{"monorepo"},
				MainCheckoutPath: filepath.Join(parent, "monorepo-main"),
			},
		},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.CleanedUp != 0 || result.Merged != 0 {
		t.Fatalf("result = %+v, want out-of-scope vamos row preserved", result)
	}
	row, err := queries.GetImplWorkspace(ctx, "stage")
	if err != nil {
		t.Fatalf("GetImplWorkspace(stage): %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusActive) {
		t.Fatalf("status = %q, want active", row.Status)
	}
}

func TestImplWorkspaceSyncerCreatesRowsAndWorkspaceEnv(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	mainCheckout := makeImplSyncCheckout(t, parent, "cn-agents")
	featureCheckout := makeImplSyncCheckout(
		t,
		parent,
		"cn-agents-2026-05-20_20-18-45_workspace-discovery-sync",
	)
	queries := openImplSyncTestQueries(t)

	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(
		ctx,
		ImplWorkspaceSyncInput{
			Discovery: ImplWorkspaceDiscoveryConfig{
				MainCheckoutPath: mainCheckout,
				ParentDir:        parent,
				Domain:           "workspaces.example.test",
			},
			ManagerURL:   "https://main.workspaces.example.test",
			RestartToken: "secret-token",
			TrunkBranch:  "main",
		},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Upserted != 2 || result.RepairedEnv != 2 || !result.Changed {
		t.Fatalf("result = %+v, want two upserts, two env repairs, changed", result)
	}

	row, err := queries.GetImplWorkspace(
		ctx,
		"2026-05-20-20-18-45-workspace-discovery-sync",
	)
	if err != nil {
		t.Fatalf("GetImplWorkspace(feature): %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusActive) {
		t.Fatalf("status = %q, want active", row.Status)
	}
	if row.CheckoutPath != featureCheckout {
		t.Fatalf("checkout_path = %q, want %q", row.CheckoutPath, featureCheckout)
	}
	if row.Url != "https://2026-05-20-20-18-45-workspace-discovery-sync.workspaces.example.test/" {
		t.Fatalf("url = %q", row.Url)
	}

	metadata := readImplSyncMetadata(t, featureCheckout)
	if metadata.Slug != row.WorkspaceSlug || metadata.CheckoutPath != featureCheckout ||
		metadata.ManagerURL != "https://main.workspaces.example.test" || metadata.RestartToken != "secret-token" {
		t.Fatalf("metadata = %+v", metadata)
	}
}

func TestImplWorkspaceSyncerRepairsStaleWorkspaceEnv(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	checkout := makeImplSyncCheckout(
		t,
		parent,
		"cn-agents-2026-05-20_20-18-45_workspace-discovery-sync",
	)
	queries := openImplSyncTestQueries(t)
	if err := WriteMetadata(WorkspaceMetadataPath(checkout), WorkspaceMetadata{
		Slug:         "wrong",
		CheckoutPath: filepath.Join(parent, "wrong"),
		ManagerURL:   "https://work.example.test",
		RestartToken: "old-token",
		PID:          123,
		Port:         456,
	}); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}

	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(
		ctx,
		ImplWorkspaceSyncInput{
			Discovery: ImplWorkspaceDiscoveryConfig{
				ParentDir: parent,
				Domain:    "workspaces.example.test",
			},
			ManagerURL:   "https://main.workspaces.example.test",
			RestartToken: "secret-token",
		},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.RepairedEnv != 1 {
		t.Fatalf("RepairedEnv = %d, want 1", result.RepairedEnv)
	}

	metadata := readImplSyncMetadata(t, checkout)
	if metadata.Slug != "2026-05-20-20-18-45-workspace-discovery-sync" ||
		metadata.CheckoutPath != checkout ||
		metadata.ManagerURL != "https://main.workspaces.example.test" ||
		metadata.RestartToken != "secret-token" {
		t.Fatalf("metadata = %+v", metadata)
	}
	if metadata.PID != 123 || metadata.Port != 456 {
		t.Fatalf(
			"metadata pid/port = %d/%d, want preserved 123/456",
			metadata.PID,
			metadata.Port,
		)
	}
}

func TestImplWorkspaceSyncerSkipsActiveLifecycle(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	checkout := makeImplSyncCheckout(
		t,
		parent,
		"cn-agents-2026-05-20_20-18-45_workspace-discovery-sync",
	)
	queries := openImplSyncTestQueries(t)
	if err := WriteMetadata(WorkspaceMetadataPath(checkout), WorkspaceMetadata{
		Slug:         "wrong",
		CheckoutPath: filepath.Join(parent, "wrong"),
		ManagerURL:   "https://old.example.test",
		RestartToken: "old-token",
	}); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}
	if err := (FileBundleStore{}).WriteLifecycle(
		Workspace{CheckoutPath: checkout},
		WorkspaceLifecycleState{
			ObservedState: WorkspaceObservedStarting,
		},
	); err != nil {
		t.Fatalf("WriteLifecycle: %v", err)
	}

	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(
		ctx,
		ImplWorkspaceSyncInput{
			Discovery: ImplWorkspaceDiscoveryConfig{
				ParentDir: parent,
				Domain:    "workspaces.example.test",
			},
			ManagerURL:   "https://main.workspaces.example.test",
			RestartToken: "secret-token",
		},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.RepairedEnv != 0 {
		t.Fatalf("RepairedEnv = %d, want 0", result.RepairedEnv)
	}
	metadata := readImplSyncMetadata(t, checkout)
	if metadata.Slug != "wrong" || metadata.CheckoutPath == checkout {
		t.Fatalf("metadata = %+v, want stale values preserved", metadata)
	}
}

func TestImplWorkspaceSyncerMarksMissingCleanedUp(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	checkout := makeImplSyncCheckout(
		t,
		parent,
		"cn-agents-2026-05-20_20-18-45_workspace-discovery-sync",
	)
	queries := openImplSyncTestQueries(t)
	syncer := &ImplWorkspaceSyncer{Queries: queries}
	input := ImplWorkspaceSyncInput{
		Discovery: ImplWorkspaceDiscoveryConfig{
			ParentDir: parent,
			Domain:    "workspaces.example.test",
		},
		ManagerURL:   "https://main.workspaces.example.test",
		RestartToken: "secret-token",
	}
	if _, err := syncer.Sync(ctx, input); err != nil {
		t.Fatalf("initial Sync: %v", err)
	}
	if err := os.RemoveAll(checkout); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	result, err := syncer.Sync(ctx, input)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if result.CleanedUp != 1 || !result.Changed {
		t.Fatalf("result = %+v, want one cleaned up and changed", result)
	}
	row, err := queries.GetImplWorkspace(
		ctx,
		"2026-05-20-20-18-45-workspace-discovery-sync",
	)
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusCleanedUp) || !row.CleanedUpAt.Valid {
		t.Fatalf(
			"row status=%q cleaned_up_at=%v, want cleaned_up with timestamp",
			row.Status,
			row.CleanedUpAt,
		)
	}
}

func TestImplWorkspaceSyncerMarksMissingMergedWithMainEvidence(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	mainCheckout := makeImplSyncCheckout(t, parent, "cn-agents")
	mainCommit := initImplSyncGitRepo(t, mainCheckout)
	queries := openImplSyncTestQueries(t)
	_, err := queries.UpsertDiscoveredImplWorkspace(
		ctx,
		db.UpsertDiscoveredImplWorkspaceParams{
			WorkspaceSlug: "missing-workspace",
			CheckoutPath:  filepath.Join(parent, "cn-agents-missing-workspace"),
			DisplayName:   "missing workspace",
			Host:          "missing-workspace.workspaces.example.test",
			Url:           "https://missing-workspace.workspaces.example.test/",
			Status:        string(ImplWorkspaceStatusActive),
			CommitHash:    nullableString(mainCommit),
			TrunkBranch:   nullableString("main"),
		},
	)
	if err != nil {
		t.Fatalf("UpsertDiscoveredImplWorkspace: %v", err)
	}

	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(
		ctx,
		ImplWorkspaceSyncInput{
			Discovery: ImplWorkspaceDiscoveryConfig{
				MainCheckoutPath: mainCheckout,
				ParentDir:        parent,
				Domain:           "workspaces.example.test",
			},
			ManagerURL:   "https://main.workspaces.example.test",
			RestartToken: "secret-token",
			TrunkBranch:  "main",
		},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Merged != 2 || result.CleanedUp != 0 || !result.Changed {
		t.Fatalf("result = %+v, want missing and discovered checkouts merged", result)
	}
	row, err := queries.GetImplWorkspace(ctx, "missing-workspace")
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusMerged) || !row.MergedAt.Valid ||
		!strings.Contains(row.MergeEvidence.String, mainCommit) {
		t.Fatalf(
			"row status=%q merged_at=%v evidence=%+v, want merged evidence",
			row.Status,
			row.MergedAt,
			row.MergeEvidence,
		)
	}
}

func TestImplWorkspaceSyncerMarksMissingCleanedUpWithoutMergeEvidence(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	mainCheckout := makeImplSyncCheckout(t, parent, "cn-agents")
	initImplSyncGitRepo(t, mainCheckout)
	otherCheckout := makeImplSyncCheckout(t, t.TempDir(), "cn-agents-other")
	initImplSyncGitRepo(t, otherCheckout)
	if err := os.WriteFile(
		filepath.Join(otherCheckout, "README.md"),
		[]byte("other\n"),
		0o644,
	); err != nil {
		t.Fatalf("write other README: %v", err)
	}
	runImplSyncGit(t, otherCheckout, "add", "README.md")
	runImplSyncGit(t, otherCheckout, "commit", "-m", "other")
	otherCommit := strings.TrimSpace(
		runImplSyncGit(t, otherCheckout, "rev-parse", "--short", "HEAD"),
	)
	queries := openImplSyncTestQueries(t)
	_, err := queries.UpsertDiscoveredImplWorkspace(
		ctx,
		db.UpsertDiscoveredImplWorkspaceParams{
			WorkspaceSlug: "missing-workspace",
			CheckoutPath:  filepath.Join(parent, "cn-agents-missing-workspace"),
			DisplayName:   "missing workspace",
			Host:          "missing-workspace.workspaces.example.test",
			Url:           "https://missing-workspace.workspaces.example.test/",
			Status:        string(ImplWorkspaceStatusActive),
			CommitHash:    nullableString(otherCommit),
			TrunkBranch:   nullableString("main"),
		},
	)
	if err != nil {
		t.Fatalf("UpsertDiscoveredImplWorkspace: %v", err)
	}

	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(
		ctx,
		ImplWorkspaceSyncInput{
			Discovery: ImplWorkspaceDiscoveryConfig{
				MainCheckoutPath: mainCheckout,
				ParentDir:        parent,
				Domain:           "workspaces.example.test",
			},
			ManagerURL:   "https://main.workspaces.example.test",
			RestartToken: "secret-token",
			TrunkBranch:  "main",
		},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.CleanedUp != 1 || result.Merged != 1 || !result.Changed {
		t.Fatalf(
			"result = %+v, want one cleaned up, one discovered merged, and changed",
			result,
		)
	}
	row, err := queries.GetImplWorkspace(ctx, "missing-workspace")
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusCleanedUp) || !row.CleanedUpAt.Valid {
		t.Fatalf(
			"row status=%q cleaned_up_at=%v, want cleaned_up",
			row.Status,
			row.CleanedUpAt,
		)
	}
}

func TestImplWorkspaceSyncerMarksDiscoveredMergedWithHeadEvidence(t *testing.T) {
	isolateImplSyncGitPath(t)
	ctx := context.Background()
	parent := t.TempDir()
	checkout := makeImplSyncCheckout(
		t,
		parent,
		"cn-agents-2026-05-20_20-18-45_workspace-discovery-sync",
	)
	commit := initImplSyncGitRepo(t, checkout)
	queries := openImplSyncTestQueries(t)

	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(
		ctx,
		ImplWorkspaceSyncInput{
			Discovery: ImplWorkspaceDiscoveryConfig{
				ParentDir: parent,
				Domain:    "workspaces.example.test",
			},
			TrunkBranch: "main",
		},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Merged != 1 || !result.Changed {
		t.Fatalf("result = %+v, want discovered merged change", result)
	}
	row, err := queries.GetImplWorkspace(
		ctx,
		"2026-05-20-20-18-45-workspace-discovery-sync",
	)
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusMerged) || !row.MergedAt.Valid ||
		!strings.Contains(row.MergeEvidence.String, commit) ||
		!strings.Contains(row.MergeEvidence.String, "main") {
		t.Fatalf(
			"row status=%q merged_at=%v evidence=%+v, want merged HEAD evidence",
			row.Status,
			row.MergedAt,
			row.MergeEvidence,
		)
	}
}

func TestImplWorkspaceSyncerDoesNotReportUnchangedDiscoveredMerged(t *testing.T) {
	isolateImplSyncGitPath(t)
	ctx := context.Background()
	parent := t.TempDir()
	checkout := makeImplSyncCheckout(
		t,
		parent,
		"cn-agents-2026-05-20_20-18-45_workspace-discovery-sync",
	)
	initImplSyncGitRepo(t, checkout)
	queries := openImplSyncTestQueries(t)
	syncer := &ImplWorkspaceSyncer{Queries: queries}
	input := ImplWorkspaceSyncInput{
		Discovery: ImplWorkspaceDiscoveryConfig{
			ParentDir: parent,
			Domain:    "workspaces.example.test",
		},
		TrunkBranch: "main",
	}

	if _, err := syncer.Sync(ctx, input); err != nil {
		t.Fatalf("initial Sync: %v", err)
	}
	first, err := queries.GetImplWorkspace(
		ctx,
		"2026-05-20-20-18-45-workspace-discovery-sync",
	)
	if err != nil {
		t.Fatalf("GetImplWorkspace(first): %v", err)
	}
	result, err := syncer.Sync(ctx, input)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if result.Merged != 0 || result.Changed {
		t.Fatalf("result = %+v, want unchanged merged sync quiet", result)
	}
	second, err := queries.GetImplWorkspace(
		ctx,
		"2026-05-20-20-18-45-workspace-discovery-sync",
	)
	if err != nil {
		t.Fatalf("GetImplWorkspace(second): %v", err)
	}
	if !nullTimesEqual(first.MergedAt, second.MergedAt) ||
		!nullStringsEqual(first.MergeEvidence, second.MergeEvidence) {
		t.Fatalf(
			"merged evidence changed: first=%v/%+v second=%v/%+v",
			first.MergedAt,
			first.MergeEvidence,
			second.MergedAt,
			second.MergeEvidence,
		)
	}
}

func TestImplWorkspaceSyncerRestoresDiscoveredMergedCheckoutToActiveWhenHeadChanges(
	t *testing.T,
) {
	isolateImplSyncGitPath(t)
	ctx := context.Background()
	parent := t.TempDir()
	checkout := makeImplSyncCheckout(
		t,
		parent,
		"cn-agents-2026-05-20_20-18-45_workspace-discovery-sync",
	)
	initImplSyncGitRepo(t, checkout)
	queries := openImplSyncTestQueries(t)
	syncer := &ImplWorkspaceSyncer{Queries: queries}
	input := ImplWorkspaceSyncInput{
		Discovery: ImplWorkspaceDiscoveryConfig{
			ParentDir: parent,
			Domain:    "workspaces.example.test",
		},
		TrunkBranch: "main",
	}

	if _, err := syncer.Sync(ctx, input); err != nil {
		t.Fatalf("initial Sync: %v", err)
	}
	runImplSyncGit(t, checkout, "checkout", "-b", "new-unmerged-work")
	addImplSyncGitCommit(
		t,
		checkout,
		"feature.txt",
		"new work\n",
		"new unmerged work",
	)

	result, err := syncer.Sync(ctx, input)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if result.Merged != 0 || !result.Changed {
		t.Fatalf("result = %+v, want restored active change", result)
	}
	row, err := queries.GetImplWorkspace(
		ctx,
		"2026-05-20-20-18-45-workspace-discovery-sync",
	)
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusActive) || row.MergedAt.Valid ||
		row.MergeEvidence.Valid {
		t.Fatalf(
			"row status=%q merged_at=%v evidence=%+v, want active with cleared merge fields",
			row.Status,
			row.MergedAt,
			row.MergeEvidence,
		)
	}
}

func TestImplWorkspaceSyncerAttachesPlanBinding(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	checkout := makeImplSyncCheckout(
		t,
		parent,
		"cn-agents-2026-05-20_20-18-45_workspace-discovery-sync",
	)
	queries := openImplSyncTestQueries(t)
	planDir := "thoughts/creative-mode-agent/plans/2026-05-20_20-18-45_workspace-discovery-sync"
	if err := WritePlanWorkspaceBinding(
		PlanWorkspaceBindingPath(checkout),
		PlanWorkspaceBinding{
			PlanDir:       planDir,
			WorkspaceSlug: "2026-05-20-20-18-45-workspace-discovery-sync",
			CheckoutPath:  checkout,
		},
	); err != nil {
		t.Fatalf("WritePlanWorkspaceBinding: %v", err)
	}

	_, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(ctx, ImplWorkspaceSyncInput{
		Discovery: ImplWorkspaceDiscoveryConfig{
			ParentDir: parent,
			Domain:    "workspaces.example.test",
		},
		ManagerURL:   "https://main.workspaces.example.test",
		RestartToken: "secret-token",
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	row, err := queries.GetImplWorkspace(
		ctx,
		"2026-05-20-20-18-45-workspace-discovery-sync",
	)
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if !row.PlanDirRel.Valid || row.PlanDirRel.String != planDir || !row.PlanDir.Valid ||
		row.PlanDir.String != planDir {
		t.Fatalf(
			"plan binding = rel:%+v dir:%+v, want %q",
			row.PlanDirRel,
			row.PlanDir,
			planDir,
		)
	}
}

func openImplSyncTestQueries(t *testing.T) *db.Queries {
	t.Helper()
	dbConn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	schemaPath := filepath.Join("..", "..", "..", "pkg", "db", "migrations", "schema.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema %s: %v", schemaPath, err)
	}
	if _, err := dbConn.Exec(string(schema)); err != nil {
		t.Fatalf("exec schema: %v", err)
	}
	return db.New(dbConn)
}

func makeImplSyncCheckout(t *testing.T, parent, name string) string {
	t.Helper()
	checkout := filepath.Join(parent, name)
	packageDir := filepath.Join(checkout, "pkg", "agents")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(packageDir, "go.mod"),
		[]byte("module github.com/CoreyCole/vamos\n"),
		0o644,
	); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	return checkout
}

func initImplSyncGitRepo(t *testing.T, checkout string) string {
	t.Helper()
	runImplSyncGit(t, checkout, "init", "-b", "main")
	runImplSyncGit(t, checkout, "config", "user.email", "test@example.test")
	runImplSyncGit(t, checkout, "config", "user.name", "Test User")
	if err := os.WriteFile(
		filepath.Join(checkout, "README.md"),
		[]byte("test\n"),
		0o644,
	); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runImplSyncGit(t, checkout, "add", "README.md", "pkg/agents/go.mod")
	runImplSyncGit(t, checkout, "commit", "-m", "initial")
	return strings.TrimSpace(runImplSyncGit(t, checkout, "rev-parse", "--short", "HEAD"))
}

func isolateImplSyncGitPath(t *testing.T) {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find git: %v", err)
	}
	t.Setenv("PATH", filepath.Dir(gitPath))
}

func addImplSyncGitCommit(t *testing.T, checkout, filename, body, message string) string {
	t.Helper()
	if err := os.WriteFile(
		filepath.Join(checkout, filename),
		[]byte(body),
		0o644,
	); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
	runImplSyncGit(t, checkout, "add", filename)
	runImplSyncGit(t, checkout, "commit", "-m", message)
	return strings.TrimSpace(runImplSyncGit(t, checkout, "rev-parse", "--short", "HEAD"))
}

func runImplSyncGit(t *testing.T, checkout string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = checkout
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func readImplSyncMetadata(t *testing.T, checkout string) WorkspaceMetadata {
	t.Helper()
	metadata, err := ReadMetadata(WorkspaceMetadataPath(checkout))
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if strings.TrimSpace(metadata.RestartToken) == "" {
		t.Fatalf("metadata missing restart token: %+v", metadata)
	}
	return metadata
}
