package workspaces

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

func TestImplWorkspaceActivityTimestampsIgnoreMaintenanceChurn(t *testing.T) {
	ctx := context.Background()
	dbConn, queries := openImplSyncTestDB(t)
	oldUpdatedAt := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)

	upsert := func(activityHash, branch string) db.ImplWorkspace {
		t.Helper()
		row, err := queries.UpsertDiscoveredImplWorkspace(ctx, db.UpsertDiscoveredImplWorkspaceParams{
			ProjectID:        "vamos",
			WorkspaceSlug:    "activity",
			CheckoutPath:     "/tmp/activity",
			DisplayName:      "Activity",
			Host:             "activity.workspaces.example.test",
			Url:              "https://activity.workspaces.example.test/",
			Status:           string(ImplWorkspaceStatusActive),
			Branch:           nullableString(branch),
			CommitHash:       nullableString("abc123"),
			CleanupProofKind: "unknown",
			ActivityHash:     activityHash,
		})
		if err != nil {
			t.Fatalf("UpsertDiscoveredImplWorkspace(%q): %v", activityHash, err)
		}
		return row
	}

	upsert("", "main")
	if _, err := dbConn.ExecContext(ctx, `UPDATE impl_workspaces SET updated_at = ?, activity_hash = '', activity_checked_at = NULL WHERE project_id = 'vamos' AND workspace_slug = 'activity'`, oldUpdatedAt); err != nil {
		t.Fatalf("seed old updated_at: %v", err)
	}

	row := upsert("hash-1", "main")
	if row.ActivityHash != "hash-1" || !row.ActivityCheckedAt.Valid {
		t.Fatalf("activity fields = hash %q checked %+v, want hash-1 and checked timestamp", row.ActivityHash, row.ActivityCheckedAt)
	}
	if !row.UpdatedAt.Equal(oldUpdatedAt) {
		t.Fatalf("first activity hash seed updated_at = %s, want preserved %s", row.UpdatedAt, oldUpdatedAt)
	}

	row = upsert("hash-1", "main")
	if !row.UpdatedAt.Equal(oldUpdatedAt) {
		t.Fatalf("unchanged activity hash updated_at = %s, want preserved %s", row.UpdatedAt, oldUpdatedAt)
	}

	row = upsert("hash-2", "feature")
	if !row.UpdatedAt.After(oldUpdatedAt) {
		t.Fatalf("changed activity hash updated_at = %s, want after %s", row.UpdatedAt, oldUpdatedAt)
	}

	setOldUpdatedAt := func() {
		t.Helper()
		if _, err := dbConn.ExecContext(ctx, `UPDATE impl_workspaces SET status = 'active', updated_at = ? WHERE project_id = 'vamos' AND workspace_slug = 'activity'`, oldUpdatedAt); err != nil {
			t.Fatalf("reset old updated_at: %v", err)
		}
	}
	assertOldUpdatedAt := func(label string) {
		t.Helper()
		row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{ProjectID: "vamos", WorkspaceSlug: "activity"})
		if err != nil {
			t.Fatalf("GetImplWorkspace after %s: %v", label, err)
		}
		if !row.UpdatedAt.Equal(oldUpdatedAt) {
			t.Fatalf("%s updated_at = %s, want preserved %s", label, row.UpdatedAt, oldUpdatedAt)
		}
	}

	setOldUpdatedAt()
	if err := queries.RecordImplWorkspaceEnvRepair(ctx, db.RecordImplWorkspaceEnvRepairParams{ProjectID: "vamos", WorkspaceSlug: "activity"}); err != nil {
		t.Fatalf("RecordImplWorkspaceEnvRepair: %v", err)
	}
	assertOldUpdatedAt("env repair")

	setOldUpdatedAt()
	if err := queries.RecordImplWorkspaceEnvError(ctx, db.RecordImplWorkspaceEnvErrorParams{ProjectID: "vamos", WorkspaceSlug: "activity", EnvLastError: nullableString("broken env")}); err != nil {
		t.Fatalf("RecordImplWorkspaceEnvError: %v", err)
	}
	assertOldUpdatedAt("env error")

	setOldUpdatedAt()
	if _, err := queries.MarkImplWorkspaceMergeUnknown(ctx, db.MarkImplWorkspaceMergeUnknownParams{ProjectID: "vamos", WorkspaceSlug: "activity", CleanupRiskReason: nullableString("not proven")}); err != nil {
		t.Fatalf("MarkImplWorkspaceMergeUnknown: %v", err)
	}
	assertOldUpdatedAt("merge unknown")

	setOldUpdatedAt()
	if _, err := queries.MarkImplWorkspaceMerged(ctx, db.MarkImplWorkspaceMergedParams{ProjectID: "vamos", WorkspaceSlug: "activity", MergeEvidence: nullableString("active checkout HEAD abc123 is ancestor of origin/main"), CleanupProofKind: "ancestor"}); err != nil {
		t.Fatalf("MarkImplWorkspaceMerged: %v", err)
	}
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{ProjectID: "vamos", WorkspaceSlug: "activity"})
	if err != nil {
		t.Fatalf("GetImplWorkspace after merge: %v", err)
	}
	if !row.UpdatedAt.After(oldUpdatedAt) {
		t.Fatalf("merge lifecycle updated_at = %s, want after %s", row.UpdatedAt, oldUpdatedAt)
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

	n, err := queries.MarkImplWorkspaceCleanedUp(ctx, db.MarkImplWorkspaceCleanedUpParams{WorkspaceSlug: "merged"})
	if err != nil {
		t.Fatalf("MarkImplWorkspaceCleanedUp: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows affected = %d, want 1", n)
	}
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{WorkspaceSlug: "merged"})
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusCleanedUp) || !row.CleanedUpAt.Valid {
		t.Fatalf("row status=%q cleaned_up_at=%v, want cleaned_up with timestamp", row.Status, row.CleanedUpAt)
	}
}

func TestImplWorkspaceSyncReclaimsCheckoutPathForRenamedConfiguredWorkspace(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	workDir := makeImplSyncCheckout(t, t.TempDir(), "editable-vamos")
	queries := openImplSyncTestQueries(t)

	_, err := queries.UpsertDiscoveredImplWorkspace(ctx, db.UpsertDiscoveredImplWorkspaceParams{
		ProjectID:     "vamos",
		WorkspaceSlug: "local",
		CheckoutPath:  workDir,
		DisplayName:   "Local checkout",
		Host:          "local.workspaces.example.test",
		Url:           "https://local.workspaces.example.test/",
		Status:        string(ImplWorkspaceStatusActive),
	})
	if err != nil {
		t.Fatalf("seed local workspace: %v", err)
	}

	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(ctx, ImplWorkspaceSyncInput{
		Discovery: ImplWorkspaceDiscoveryConfig{
			ParentDir: parent,
			Domain:    "workspaces.example.test",
			ConfiguredCheckouts: map[string]ConfiguredCheckout{
				"stage": {
					RootPath:    workDir,
					DisplayName: "Stage checkout",
					Role:        CheckoutRoleStage,
					ProjectID:   "vamos",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Upserted != 1 {
		t.Fatalf("result = %+v, want one upsert", result)
	}
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{ProjectID: "vamos", WorkspaceSlug: "stage"})
	if err != nil {
		t.Fatalf("GetImplWorkspace(stage): %v", err)
	}
	if row.CheckoutPath != workDir || row.CheckoutRole != string(CheckoutRoleStage) || row.DisplayName != "Stage checkout" {
		t.Fatalf("row = %+v, want stage identity on existing checkout path", row)
	}
	if _, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{ProjectID: "vamos", WorkspaceSlug: "local"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetImplWorkspace(local) err = %v, want sql.ErrNoRows", err)
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

	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{WorkspaceSlug: "work"})
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
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{WorkspaceSlug: "work"})
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
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{WorkspaceSlug: "stage"})
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

	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{WorkspaceSlug: "2026-05-20-20-18-45-workspace-discovery-sync"})
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
	if metadata.Slug != row.WorkspaceSlug || metadata.ProjectID != row.ProjectID || metadata.CheckoutPath != featureCheckout ||
		metadata.ManagerURL != "https://main.workspaces.example.test" || metadata.RestartToken != "secret-token" {
		t.Fatalf("metadata = %+v", metadata)
	}
}

func TestImplWorkspaceSyncRecordsDiagnosticSuccess(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	mainCheckout := makeImplSyncCheckout(t, parent, "vamos")
	featureCheckout := makeImplSyncCheckout(t, parent, "vamos-2026-06-07_20-29-40_workspace-status-debuggability")
	queries := openImplSyncTestQueries(t)
	now := time.Date(2026, 6, 14, 15, 0, 0, 0, time.UTC)

	result, err := (&ImplWorkspaceSyncer{Queries: queries, Now: func() time.Time { return now }}).Sync(ctx, ImplWorkspaceSyncInput{
		ProjectID: "github.com/coreycole/vamos",
		Discovery: ImplWorkspaceDiscoveryConfig{
			MainCheckoutPath: mainCheckout,
			ParentDir:        parent,
			Domain:           "workspaces.example.test",
		},
		ManagerURL:   "https://main.workspaces.example.test",
		RestartToken: "secret-token",
		TrunkBranch:  "main",
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Upserted != 2 || result.RepairedEnv != 2 || len(result.Warnings) != 0 {
		t.Fatalf("result = %+v, want two upserts, two env repairs, no warnings", result)
	}

	row, err := queries.GetWorkspaceSyncDiagnostic(ctx, db.GetWorkspaceSyncDiagnosticParams{ProjectID: "github.com/coreycole/vamos", SyncKind: "impl_workspaces"})
	if err != nil {
		t.Fatalf("GetWorkspaceSyncDiagnostic: %v", err)
	}
	if row.Status != "ok" || row.Error != "" || !row.FinishedAt.Valid {
		t.Fatalf("diagnostic status/error/finished = %q/%q/%+v, want ok empty valid", row.Status, row.Error, row.FinishedAt)
	}
	if row.Scanned != 2 || row.Discovered != 2 || row.Upserted != 2 || row.RepairedEnv != 2 || !row.Changed {
		t.Fatalf("diagnostic counts = %+v, want discovered/upserted/repaired/changed", row)
	}
	if row.WarningsJson != "[]" {
		t.Fatalf("warnings_json = %q, want []", row.WarningsJson)
	}
	metadata := readImplSyncMetadata(t, featureCheckout)
	if metadata.ProjectID != "github.com/coreycole/vamos" {
		t.Fatalf("metadata project id = %q, want github.com/coreycole/vamos", metadata.ProjectID)
	}
}

func TestImplWorkspaceSyncRecordsEnvRepairWarning(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	checkout := makeImplSyncCheckout(t, parent, "vamos-2026-06-07_20-29-40_workspace-status-debuggability")
	queries := openImplSyncTestQueries(t)
	metadataPath := WorkspaceMetadataPath(checkout)
	if err := os.MkdirAll(metadataPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace.env path: %v", err)
	}

	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(ctx, ImplWorkspaceSyncInput{
		ProjectID: "github.com/coreycole/vamos",
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
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "workspace_env_repair_failed" {
		t.Fatalf("warnings = %+v, want workspace_env_repair_failed", result.Warnings)
	}

	row, err := queries.GetWorkspaceSyncDiagnostic(ctx, db.GetWorkspaceSyncDiagnosticParams{ProjectID: "github.com/coreycole/vamos", SyncKind: "impl_workspaces"})
	if err != nil {
		t.Fatalf("GetWorkspaceSyncDiagnostic: %v", err)
	}
	var warnings []WorkspaceDiagnostic
	if err := json.Unmarshal([]byte(row.WarningsJson), &warnings); err != nil {
		t.Fatalf("unmarshal warnings_json %q: %v", row.WarningsJson, err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %+v, want one", warnings)
	}
	warning := warnings[0]
	if warning.Code != "workspace_env_repair_failed" || warning.ProjectID != "github.com/coreycole/vamos" || warning.WorkspaceSlug != "2026-06-07-20-29-40-workspace-status-debuggability" || warning.CheckoutPath != checkout {
		t.Fatalf("warning = %+v, want target fields", warning)
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

func TestImplWorkspaceSyncerKeepsMissingUnprovenActive(t *testing.T) {
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
	if result.CleanedUp != 0 || result.Merged != 0 || !result.Changed {
		t.Fatalf("result = %+v, want missing unproven active and changed", result)
	}
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{WorkspaceSlug: "2026-05-20-20-18-45-workspace-discovery-sync"})
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusActive) || row.CleanedUpAt.Valid || row.CleanupProofKind != string(MergeProofUnknown) || !strings.Contains(row.CleanupRiskReason.String, "no commit evidence") {
		t.Fatalf(
			"row status=%q cleaned_up_at=%v proof=%q risk=%+v, want active unknown risk",
			row.Status,
			row.CleanedUpAt,
			row.CleanupProofKind,
			row.CleanupRiskReason,
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
	if result.Merged != 1 || result.CleanedUp != 0 || !result.Changed {
		t.Fatalf("result = %+v, want missing checkout merged", result)
	}
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{WorkspaceSlug: "missing-workspace"})
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusMerged) || !row.MergedAt.Valid || row.CleanupProofKind != string(MergeProofAncestor) ||
		!strings.Contains(row.MergeEvidence.String, mainCommit) {
		t.Fatalf(
			"row status=%q merged_at=%v evidence=%+v, want merged evidence",
			row.Status,
			row.MergedAt,
			row.MergeEvidence,
		)
	}
}

func TestImplWorkspaceSyncerKeepsMissingUnprovenActiveWithRisk(t *testing.T) {
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
	if result.CleanedUp != 0 || !result.Changed {
		t.Fatalf(
			"result = %+v, want missing unproven active and changed",
			result,
		)
	}
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{WorkspaceSlug: "missing-workspace"})
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusActive) || row.CleanedUpAt.Valid || row.CleanupProofKind != string(MergeProofUnknown) || !strings.Contains(row.CleanupRiskReason.String, "not proven merged") {
		t.Fatalf(
			"row status=%q cleaned_up_at=%v proof=%q risk=%+v, want active unknown risk",
			row.Status,
			row.CleanedUpAt,
			row.CleanupProofKind,
			row.CleanupRiskReason,
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
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{WorkspaceSlug: "2026-05-20-20-18-45-workspace-discovery-sync"})
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusMerged) || !row.MergedAt.Valid || row.CleanupProofKind != string(MergeProofAncestor) ||
		!strings.Contains(row.MergeEvidence.String, commit) ||
		!strings.Contains(row.MergeEvidence.String, "origin/main") {
		t.Fatalf(
			"row status=%q merged_at=%v evidence=%+v, want merged HEAD evidence",
			row.Status,
			row.MergedAt,
			row.MergeEvidence,
		)
	}
}

func TestImplWorkspaceSyncerKeepsConfiguredStageRoleActiveWhenHeadIsMerged(t *testing.T) {
	isolateImplSyncGitPath(t)
	ctx := context.Background()
	parent := t.TempDir()
	checkout := makeImplSyncCheckout(t, t.TempDir(), "stage-checkout")
	initImplSyncGitRepo(t, checkout)
	queries := openImplSyncTestQueries(t)

	result, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(
		ctx,
		ImplWorkspaceSyncInput{
			ProjectID: "example.com/alpha/app",
			Discovery: ImplWorkspaceDiscoveryConfig{
				ProjectID: "example.com/alpha/app",
				ParentDir: parent,
				Domain:    "workspaces.example.test",
				ConfiguredCheckouts: map[string]ConfiguredCheckout{
					"stage": {
						RootPath:    checkout,
						DisplayName: "Stage",
						Role:        CheckoutRoleStage,
						ProjectID:   "example.com/alpha/app",
					},
				},
			},
			TrunkBranch: "main",
		},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Merged != 0 {
		t.Fatalf("result = %+v, want protected stage not counted merged", result)
	}
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{ProjectID: "example.com/alpha/app", WorkspaceSlug: "stage"})
	if err != nil {
		t.Fatalf("GetImplWorkspace(stage): %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusActive) || row.CheckoutRole != string(CheckoutRoleStage) || row.ProjectID != "example.com/alpha/app" {
		t.Fatalf("row status=%q role=%q project=%q, want active stage alpha", row.Status, row.CheckoutRole, row.ProjectID)
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
	first, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{WorkspaceSlug: "2026-05-20-20-18-45-workspace-discovery-sync"})
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
	second, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{WorkspaceSlug: "2026-05-20-20-18-45-workspace-discovery-sync"})
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

func TestImplWorkspaceSyncerKeepsCachedMergedProofAfterFetchFailure(t *testing.T) {
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
	runImplSyncGit(t, checkout, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
	result, err := syncer.Sync(ctx, input)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if result.Merged != 1 || !result.Changed {
		t.Fatalf("result = %+v, want cached merged proof change", result)
	}
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{WorkspaceSlug: "2026-05-20-20-18-45-workspace-discovery-sync"})
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusMerged) || row.CleanupProofKind != string(MergeProofCached) || !strings.Contains(row.MergeEvidence.String, "cached") {
		t.Fatalf("row status=%q proof=%q evidence=%+v, want cached merged proof", row.Status, row.CleanupProofKind, row.MergeEvidence)
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
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{WorkspaceSlug: "2026-05-20-20-18-45-workspace-discovery-sync"})
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if row.Status != string(ImplWorkspaceStatusActive) || row.MergedAt.Valid ||
		row.MergeEvidence.Valid || row.CleanupProofKind != string(MergeProofUnknown) || !row.CleanupRiskReason.Valid {
		t.Fatalf(
			"row status=%q merged_at=%v evidence=%+v, want active with cleared merge fields and unknown proof",
			row.Status,
			row.MergedAt,
			row.MergeEvidence,
		)
	}
}

func TestPlanWorkspaceBindingMatchesProject(t *testing.T) {
	binding := PlanWorkspaceBinding{PlanDir: "thoughts/agent/plans/demo", ProjectID: "datastarui"}
	if !PlanWorkspaceBindingMatchesProject(binding, "thoughts/agent/plans/demo", "datastarui") {
		t.Fatal("binding should match same project")
	}
	if PlanWorkspaceBindingMatchesProject(binding, "thoughts/agent/plans/demo", "vamos") {
		t.Fatal("binding should not match a different project")
	}
	legacy := PlanWorkspaceBinding{PlanDir: "thoughts/agent/plans/demo"}
	if !PlanWorkspaceBindingMatchesProject(legacy, "thoughts/agent/plans/demo", "vamos") {
		t.Fatal("legacy binding without project id should remain compatible")
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
	_, err := queries.UpsertDiscoveredPlanWorkspace(ctx, db.UpsertDiscoveredPlanWorkspaceParams{
		PlanDirRel:        planDir,
		ProjectID:         "vamos",
		PlanDir:           planDir,
		Label:             "workspace discovery sync",
		ArtifactUpdatedAt: time.Now(),
		QrspiLifecycle:    "implement",
	})
	if err != nil {
		t.Fatalf("UpsertDiscoveredPlanWorkspace: %v", err)
	}
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

	_, err = (&ImplWorkspaceSyncer{Queries: queries}).Sync(ctx, ImplWorkspaceSyncInput{
		ProjectID: "vamos",
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
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{ProjectID: "vamos", WorkspaceSlug: "2026-05-20-20-18-45-workspace-discovery-sync"})
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
	bindings, err := queries.ListPlanWorkspaceImplBindings(ctx, planDir)
	if err != nil {
		t.Fatalf("ListPlanWorkspaceImplBindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].ProjectID != "vamos" || bindings[0].Status != "active" || bindings[0].BindingSource != "binding_file" {
		t.Fatalf("plan workspace impl bindings = %#v", bindings)
	}
}

func TestImplWorkspaceSyncerCanonicalizesAbsolutePlanBinding(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	checkout := makeImplSyncCheckout(
		t,
		parent,
		"cn-agents-2026-05-20_20-18-45_workspace-discovery-sync",
	)
	dbConn, queries := openImplSyncTestDB(t)
	if _, err := dbConn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	planDir := "creative-mode-agent/plans/2026-05-20_20-18-45_workspace-discovery-sync"
	_, err := queries.UpsertDiscoveredPlanWorkspace(ctx, db.UpsertDiscoveredPlanWorkspaceParams{
		PlanDirRel:        planDir,
		ProjectID:         "vamos",
		PlanDir:           filepath.Join(parent, "thoughts", planDir),
		Label:             "workspace discovery sync",
		ArtifactUpdatedAt: time.Now(),
		QrspiLifecycle:    "implement",
	})
	if err != nil {
		t.Fatalf("UpsertDiscoveredPlanWorkspace: %v", err)
	}
	if err := WritePlanWorkspaceBinding(
		PlanWorkspaceBindingPath(checkout),
		PlanWorkspaceBinding{
			PlanDir:       filepath.Join(parent, "thoughts", planDir, "plan.md"),
			WorkspaceSlug: "2026-05-20-20-18-45-workspace-discovery-sync",
			CheckoutPath:  checkout,
		},
	); err != nil {
		t.Fatalf("WritePlanWorkspaceBinding: %v", err)
	}

	_, err = (&ImplWorkspaceSyncer{Queries: queries}).Sync(ctx, ImplWorkspaceSyncInput{
		ProjectID: "vamos",
		Discovery: ImplWorkspaceDiscoveryConfig{
			ParentDir: parent,
			Domain:    "workspaces.example.test",
		},
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{ProjectID: "vamos", WorkspaceSlug: "2026-05-20-20-18-45-workspace-discovery-sync"})
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if !row.PlanDirRel.Valid || row.PlanDirRel.String != planDir {
		t.Fatalf("plan_dir_rel = %+v, want %q", row.PlanDirRel, planDir)
	}
}

func TestImplWorkspaceSyncerIgnoresMissingPlanBindingWithForeignKeys(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	checkout := makeImplSyncCheckout(
		t,
		parent,
		"cn-agents-2026-05-20_20-18-45_workspace-discovery-sync",
	)
	dbConn, queries := openImplSyncTestDB(t)
	if _, err := dbConn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	if err := WritePlanWorkspaceBinding(
		PlanWorkspaceBindingPath(checkout),
		PlanWorkspaceBinding{
			PlanDir:       "thoughts/creative-mode-agent/plans/missing-plan",
			WorkspaceSlug: "2026-05-20-20-18-45-workspace-discovery-sync",
			CheckoutPath:  checkout,
		},
	); err != nil {
		t.Fatalf("WritePlanWorkspaceBinding: %v", err)
	}

	_, err := (&ImplWorkspaceSyncer{Queries: queries}).Sync(ctx, ImplWorkspaceSyncInput{
		ProjectID: "vamos",
		Discovery: ImplWorkspaceDiscoveryConfig{
			ParentDir: parent,
			Domain:    "workspaces.example.test",
		},
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	row, err := queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{ProjectID: "vamos", WorkspaceSlug: "2026-05-20-20-18-45-workspace-discovery-sync"})
	if err != nil {
		t.Fatalf("GetImplWorkspace: %v", err)
	}
	if row.PlanDirRel.Valid || row.PlanDir.Valid {
		t.Fatalf("plan binding = rel:%+v dir:%+v, want cleared missing plan", row.PlanDirRel, row.PlanDir)
	}
	violations, err := dbConn.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer violations.Close()
	if violations.Next() {
		t.Fatal("foreign_key_check reported violations")
	}
}

func openImplSyncTestQueries(t *testing.T) *db.Queries {
	t.Helper()
	_, queries := openImplSyncTestDB(t)
	return queries
}

func openImplSyncTestDB(t *testing.T) (*sql.DB, *db.Queries) {
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
	return dbConn, db.New(dbConn)
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
	origin := filepath.Join(t.TempDir(), "origin.git")
	runImplSyncGit(t, checkout, "init", "--bare", origin)
	runImplSyncGit(t, checkout, "remote", "add", "origin", origin)
	runImplSyncGit(t, checkout, "push", "-u", "origin", "main")
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
