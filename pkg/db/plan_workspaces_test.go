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
		WorkspaceSlug:     strings.ReplaceAll(rel, "/", "-"),
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
