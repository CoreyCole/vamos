package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func insertPlanWorkspaceForProjectTest(t *testing.T, ctx context.Context, q *Queries, rel, projectID string) {
	t.Helper()
	_, err := q.UpsertDiscoveredPlanWorkspace(ctx, UpsertDiscoveredPlanWorkspaceParams{
		PlanDirRel:        rel,
		ProjectID:         projectID,
		PlanDir:           "thoughts/" + rel,
		Label:             rel,
		ArtifactUpdatedAt: time.Now(),
		QrspiLifecycle:    "implement",
	})
	if err != nil {
		t.Fatalf("UpsertDiscoveredPlanWorkspace(%s): %v", rel, err)
	}
}

func TestPlanWorkspaceProjectRolesEnforceOnePrimaryAndAllowManyRelated(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)
	insertPlanWorkspaceForProjectTest(t, ctx, q, "agent/plans/multi", "vamos")

	if _, err := q.UpsertPlanWorkspaceProject(ctx, UpsertPlanWorkspaceProjectParams{
		PlanDirRel:     "agent/plans/multi",
		ProjectID:      "vamos",
		Role:           "primary",
		DeclaredSource: "plan.md",
	}); err != nil {
		t.Fatalf("upsert primary: %v", err)
	}
	if _, err := q.UpsertPlanWorkspaceProject(ctx, UpsertPlanWorkspaceProjectParams{
		PlanDirRel:     "agent/plans/multi",
		ProjectID:      "datastarui",
		Role:           "related",
		DeclaredSource: "plan.md",
	}); err != nil {
		t.Fatalf("upsert related datastarui: %v", err)
	}
	if _, err := q.UpsertPlanWorkspaceProject(ctx, UpsertPlanWorkspaceProjectParams{
		PlanDirRel:     "agent/plans/multi",
		ProjectID:      "cn-agents",
		Role:           "related",
		DeclaredSource: "plan.md",
	}); err != nil {
		t.Fatalf("upsert related cn-agents: %v", err)
	}
	if _, err := q.UpsertPlanWorkspaceProject(ctx, UpsertPlanWorkspaceProjectParams{
		PlanDirRel:     "agent/plans/multi",
		ProjectID:      "other-primary",
		Role:           "primary",
		DeclaredSource: "plan.md",
	}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Fatalf("second active primary err = %v, want unique constraint", err)
	}

	roles, err := q.ListPlanWorkspaceProjects(ctx, "agent/plans/multi")
	if err != nil {
		t.Fatalf("ListPlanWorkspaceProjects: %v", err)
	}
	if len(roles) != 3 {
		t.Fatalf("roles len = %d, want 3: %#v", len(roles), roles)
	}
	if roles[0].ProjectID != "vamos" || roles[0].Role != "primary" {
		t.Fatalf("first role = %#v, want primary vamos", roles[0])
	}
}

func TestPlanWorkspaceImplBindingAllowsPlannedRowsWithoutWorkspace(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)
	insertPlanWorkspaceForProjectTest(t, ctx, q, "agent/plans/multi", "vamos")

	binding, err := q.UpsertPlanWorkspaceImplBinding(ctx, UpsertPlanWorkspaceImplBindingParams{
		PlanDirRel:    "agent/plans/multi",
		ProjectID:     "datastarui",
		Status:        "planned",
		BindingSource: "metadata",
	})
	if err != nil {
		t.Fatalf("UpsertPlanWorkspaceImplBinding: %v", err)
	}
	if binding.WorkspaceSlug.Valid || binding.CheckoutPath.Valid || binding.Url.Valid {
		t.Fatalf("planned binding has workspace fields: %#v", binding)
	}

	bindings, err := q.ListPlanWorkspaceImplBindings(ctx, "agent/plans/multi")
	if err != nil {
		t.Fatalf("ListPlanWorkspaceImplBindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].ProjectID != "datastarui" || bindings[0].Status != "planned" {
		t.Fatalf("bindings = %#v, want planned datastarui", bindings)
	}
}

func TestListPlanWorkspacesIncludesRelatedProjectRoles(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)
	insertPlanWorkspaceForProjectTest(t, ctx, q, "agent/plans/multi", "vamos")
	insertPlanWorkspaceForProjectTest(t, ctx, q, "agent/plans/other", "cn-agents")
	if _, err := q.UpsertPlanWorkspaceProject(ctx, UpsertPlanWorkspaceProjectParams{
		PlanDirRel:     "agent/plans/multi",
		ProjectID:      "datastarui",
		Role:           "related",
		DeclaredSource: "plan.md",
	}); err != nil {
		t.Fatalf("upsert related: %v", err)
	}

	rows, err := q.ListCurrentPlanWorkspaces(ctx, "datastarui")
	if err != nil {
		t.Fatalf("ListCurrentPlanWorkspaces: %v", err)
	}
	if len(rows) != 1 || rows[0].PlanDirRel != "agent/plans/multi" {
		t.Fatalf("related filter rows = %#v", rows)
	}
}

func TestArchiveMissingPlanWorkspaceProjectsArchivesRemovedRoles(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)
	insertPlanWorkspaceForProjectTest(t, ctx, q, "agent/plans/multi", "vamos")
	for _, projectID := range []string{"vamos", "datastarui", "cn-agents"} {
		role := "related"
		if projectID == "vamos" {
			role = "primary"
		}
		if _, err := q.UpsertPlanWorkspaceProject(ctx, UpsertPlanWorkspaceProjectParams{
			PlanDirRel:     "agent/plans/multi",
			ProjectID:      projectID,
			Role:           role,
			DeclaredSource: "plan.md",
		}); err != nil {
			t.Fatalf("upsert role %s: %v", projectID, err)
		}
	}

	archived, err := q.ArchiveMissingPlanWorkspaceProjects(ctx, ArchiveMissingPlanWorkspaceProjectsParams{
		PlanDirRel: "agent/plans/multi",
		ProjectIds: []string{"vamos", "datastarui"},
	})
	if err != nil {
		t.Fatalf("ArchiveMissingPlanWorkspaceProjects: %v", err)
	}
	if archived != 1 {
		t.Fatalf("archived = %d, want 1", archived)
	}
	roles, err := q.ListPlanWorkspaceProjects(ctx, "agent/plans/multi")
	if err != nil {
		t.Fatalf("ListPlanWorkspaceProjects: %v", err)
	}
	if len(roles) != 2 {
		t.Fatalf("active roles len = %d, want 2: %#v", len(roles), roles)
	}
	if _, err := q.UpsertPlanWorkspaceProject(ctx, UpsertPlanWorkspaceProjectParams{
		PlanDirRel:     "agent/plans/multi",
		ProjectID:      "cn-agents",
		Role:           "related",
		DeclaredSource: "plan.md",
	}); err != nil {
		t.Fatalf("restore archived related: %v", err)
	}
	restored, err := q.ListPlanWorkspaceProjects(ctx, "agent/plans/multi")
	if err != nil {
		t.Fatalf("ListPlanWorkspaceProjects restored: %v", err)
	}
	if len(restored) != 3 || restored[2].ArchivedAt != (sql.NullTime{}) {
		t.Fatalf("restored roles = %#v, want active restored cn-agents", restored)
	}
}

func TestUpsertDiscoveredPlanWorkspaceClearsMissingFromDiskArchive(t *testing.T) {
	ctx := context.Background()
	dbConn, q := openWorkspaceDocsTestDB(t)
	rel := "agent/plans/rediscover-missing"

	row, err := q.UpsertDiscoveredPlanWorkspace(ctx, UpsertDiscoveredPlanWorkspaceParams{
		PlanDirRel:        rel,
		ProjectID:         "vamos",
		PlanDir:           "thoughts/" + rel,
		Label:             "rediscover-missing",
		ArtifactUpdatedAt: time.Now(),
		QrspiLifecycle:    "plan",
	})
	if err != nil {
		t.Fatalf("initial upsert: %v", err)
	}
	if row.ArchiveReason != "" || row.ArchivedAt.Valid {
		t.Fatalf("fresh row archive state = %#v", row)
	}

	archived, err := q.ArchiveMissingPlanWorkspaces(ctx, []string{"agent/plans/other"})
	if err != nil {
		t.Fatalf("ArchiveMissingPlanWorkspaces: %v", err)
	}
	if archived != 1 {
		t.Fatalf("archived = %d, want 1", archived)
	}
	missing, err := q.GetPlanWorkspace(ctx, rel)
	if err != nil {
		t.Fatalf("GetPlanWorkspace after missing archive: %v", err)
	}
	if !missing.ArchivedAt.Valid || missing.ArchiveReason != "missing_from_disk" {
		t.Fatalf("missing archive state = %#v", missing)
	}

	restored, err := q.UpsertDiscoveredPlanWorkspace(ctx, UpsertDiscoveredPlanWorkspaceParams{
		PlanDirRel:        rel,
		ProjectID:         "vamos",
		PlanDir:           "thoughts/" + rel,
		Label:             "rediscover-missing",
		ArtifactUpdatedAt: time.Now(),
		QrspiLifecycle:    "plan",
	})
	if err != nil {
		t.Fatalf("rediscover upsert: %v", err)
	}
	if restored.ArchivedAt.Valid {
		t.Fatalf("rediscover should clear archived_at, got %#v", restored.ArchivedAt)
	}
	if restored.ArchiveReason != "" || restored.ArchivedByEmail != "" {
		t.Fatalf("rediscover should clear archive metadata, got reason=%q email=%q", restored.ArchiveReason, restored.ArchivedByEmail)
	}
	_ = dbConn
}

func TestUpsertDiscoveredPlanWorkspaceKeepsManualArchiveSticky(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)
	rel := "agent/plans/manual-sticky"

	if _, err := q.UpsertDiscoveredPlanWorkspace(ctx, UpsertDiscoveredPlanWorkspaceParams{
		PlanDirRel:        rel,
		ProjectID:         "vamos",
		PlanDir:           "thoughts/" + rel,
		Label:             "manual-sticky",
		ArtifactUpdatedAt: time.Now(),
		QrspiLifecycle:    "plan",
	}); err != nil {
		t.Fatalf("initial upsert: %v", err)
	}

	manualAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	if _, err := q.db.ExecContext(ctx, `
UPDATE plan_workspaces
SET archived_at = ?, archive_reason = 'manual', archived_by_email = 'corey@example.com'
WHERE plan_dir_rel = ?`, manualAt, rel); err != nil {
		t.Fatalf("seed manual archive: %v", err)
	}

	before, err := q.GetPlanWorkspace(ctx, rel)
	if err != nil {
		t.Fatalf("GetPlanWorkspace before rediscover: %v", err)
	}
	if !before.ArchivedAt.Valid || before.ArchiveReason != "manual" || before.ArchivedByEmail != "corey@example.com" {
		t.Fatalf("manual seed state = %#v", before)
	}

	after, err := q.UpsertDiscoveredPlanWorkspace(ctx, UpsertDiscoveredPlanWorkspaceParams{
		PlanDirRel:        rel,
		ProjectID:         "vamos",
		PlanDir:           "thoughts/" + rel,
		Label:             "manual-sticky-updated",
		ArtifactUpdatedAt: time.Now(),
		QrspiLifecycle:    "implement",
	})
	if err != nil {
		t.Fatalf("rediscover upsert: %v", err)
	}
	if after.Label != "manual-sticky-updated" || after.QrspiLifecycle != "implement" {
		t.Fatalf("rediscover should still update non-archive fields: %#v", after)
	}
	if !after.ArchivedAt.Valid {
		t.Fatal("manual archive archived_at should stay set")
	}
	if !after.ArchivedAt.Time.Equal(before.ArchivedAt.Time) {
		t.Fatalf("archived_at changed from %v to %v", before.ArchivedAt.Time, after.ArchivedAt.Time)
	}
	if after.ArchiveReason != "manual" || after.ArchivedByEmail != "corey@example.com" {
		t.Fatalf("manual archive metadata cleared: reason=%q email=%q", after.ArchiveReason, after.ArchivedByEmail)
	}

	// Manual rows already archived should not be touched by missing-disk archive sync.
	n, err := q.ArchiveMissingPlanWorkspaces(ctx, []string{"agent/plans/other"})
	if err != nil {
		t.Fatalf("ArchiveMissingPlanWorkspaces: %v", err)
	}
	if n != 0 {
		t.Fatalf("ArchiveMissingPlanWorkspaces rows = %d, want 0 (manual already archived)", n)
	}
	still, err := q.GetPlanWorkspace(ctx, rel)
	if err != nil {
		t.Fatalf("GetPlanWorkspace after missing sync: %v", err)
	}
	if still.ArchiveReason != "manual" || still.ArchivedByEmail != "corey@example.com" {
		t.Fatalf("missing sync changed manual archive: %#v", still)
	}
}

func TestArchiveAllActivePlanWorkspacesSetsMissingFromDiskReason(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)
	insertPlanWorkspaceForProjectTest(t, ctx, q, "agent/plans/all-active", "vamos")

	n, err := q.ArchiveAllActivePlanWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ArchiveAllActivePlanWorkspaces: %v", err)
	}
	if n != 1 {
		t.Fatalf("archived = %d, want 1", n)
	}
	row, err := q.GetPlanWorkspace(ctx, "agent/plans/all-active")
	if err != nil {
		t.Fatalf("GetPlanWorkspace: %v", err)
	}
	if !row.ArchivedAt.Valid || row.ArchiveReason != "missing_from_disk" {
		t.Fatalf("archive-all state = %#v", row)
	}
}

func TestManualArchivePlanWorkspaceStickyAndIdempotent(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)
	rel := "agent/plans/manual-api"
	insertPlanWorkspaceForProjectTest(t, ctx, q, rel, "vamos")

	first, err := q.ManualArchivePlanWorkspace(ctx, ManualArchivePlanWorkspaceParams{
		PlanDirRel:      rel,
		ArchivedByEmail: "corey@example.com",
	})
	if err != nil {
		t.Fatalf("ManualArchivePlanWorkspace: %v", err)
	}
	if !first.ArchivedAt.Valid || first.ArchiveReason != "manual" || first.ArchivedByEmail != "corey@example.com" {
		t.Fatalf("first archive state = %#v", first)
	}

	second, err := q.ManualArchivePlanWorkspace(ctx, ManualArchivePlanWorkspaceParams{
		PlanDirRel:      rel,
		ArchivedByEmail: "other@example.com",
	})
	if err != nil {
		t.Fatalf("idempotent ManualArchivePlanWorkspace: %v", err)
	}
	if !second.ArchivedAt.Valid || !second.ArchivedAt.Time.Equal(first.ArchivedAt.Time) {
		t.Fatalf("archived_at changed on idempotent archive: first=%v second=%v", first.ArchivedAt, second.ArchivedAt)
	}
	if second.ArchiveReason != "manual" || second.ArchivedByEmail != "corey@example.com" {
		t.Fatalf("idempotent archive should keep original actor: %#v", second)
	}

	current, err := q.ListCurrentPlanWorkspaces(ctx, "")
	if err != nil {
		t.Fatalf("ListCurrentPlanWorkspaces: %v", err)
	}
	for _, row := range current {
		if row.PlanDirRel == rel {
			t.Fatalf("manually archived plan still in current list: %#v", row)
		}
	}

	archived, err := q.ListManualArchivedPlanWorkspaces(ctx, "")
	if err != nil {
		t.Fatalf("ListManualArchivedPlanWorkspaces: %v", err)
	}
	if len(archived) != 1 || archived[0].PlanDirRel != rel {
		t.Fatalf("manual archived list = %#v, want %s", archived, rel)
	}
}

func TestUnarchiveManualPlanWorkspaceAndRefuseMissingFromDisk(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)

	manualRel := "agent/plans/manual-unarchive"
	insertPlanWorkspaceForProjectTest(t, ctx, q, manualRel, "vamos")
	if _, err := q.ManualArchivePlanWorkspace(ctx, ManualArchivePlanWorkspaceParams{
		PlanDirRel:      manualRel,
		ArchivedByEmail: "corey@example.com",
	}); err != nil {
		t.Fatalf("seed manual archive: %v", err)
	}
	cleared, err := q.UnarchiveManualPlanWorkspace(ctx, manualRel)
	if err != nil {
		t.Fatalf("UnarchiveManualPlanWorkspace: %v", err)
	}
	if cleared.ArchivedAt.Valid || cleared.ArchiveReason != "" || cleared.ArchivedByEmail != "" {
		t.Fatalf("unarchive did not clear fields: %#v", cleared)
	}

	missingRel := "agent/plans/missing-unarchive"
	insertPlanWorkspaceForProjectTest(t, ctx, q, missingRel, "vamos")
	if _, err := q.ArchiveMissingPlanWorkspaces(ctx, []string{"agent/plans/other"}); err != nil {
		t.Fatalf("ArchiveMissingPlanWorkspaces: %v", err)
	}
	missing, err := q.GetPlanWorkspace(ctx, missingRel)
	if err != nil {
		t.Fatalf("GetPlanWorkspace: %v", err)
	}
	if missing.ArchiveReason != "missing_from_disk" {
		t.Fatalf("expected missing_from_disk, got %#v", missing)
	}
	if _, err := q.UnarchiveManualPlanWorkspace(ctx, missingRel); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unarchive missing_from_disk err = %v, want sql.ErrNoRows", err)
	}
}

func TestListManualArchivedExcludesMissingFromDisk(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)
	insertPlanWorkspaceForProjectTest(t, ctx, q, "agent/plans/manual-listed", "vamos")
	insertPlanWorkspaceForProjectTest(t, ctx, q, "agent/plans/missing-listed", "vamos")

	if _, err := q.ManualArchivePlanWorkspace(ctx, ManualArchivePlanWorkspaceParams{
		PlanDirRel:      "agent/plans/manual-listed",
		ArchivedByEmail: "corey@example.com",
	}); err != nil {
		t.Fatalf("manual archive: %v", err)
	}
	if _, err := q.ArchiveMissingPlanWorkspaces(ctx, []string{"agent/plans/manual-listed"}); err != nil {
		t.Fatalf("missing archive: %v", err)
	}

	archived, err := q.ListManualArchivedPlanWorkspaces(ctx, "")
	if err != nil {
		t.Fatalf("ListManualArchivedPlanWorkspaces: %v", err)
	}
	if len(archived) != 1 || archived[0].PlanDirRel != "agent/plans/manual-listed" {
		t.Fatalf("archived list = %#v", archived)
	}
	if archived[0].ArchiveReason != "manual" {
		t.Fatalf("archive_reason = %q", archived[0].ArchiveReason)
	}
}
