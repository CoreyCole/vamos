package db

import (
	"context"
	"database/sql"
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
