package agentchat

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/db"
	serverdb "github.com/CoreyCole/vamos/server/services/db"
)

func setupPlanDirRelTest(
	t *testing.T,
) (projectRoot, thoughtsRoot, planAbs, planRel string, service *Service, database *serverdb.Service) {
	t.Helper()
	projectRoot = t.TempDir()
	thoughtsRoot = filepath.Join(projectRoot, "thoughts")
	planAbs = filepath.Join(thoughtsRoot, "owner", "plans", "alpha")
	if err := os.MkdirAll(planAbs, 0o755); err != nil {
		t.Fatal(err)
	}
	planRel = "owner/plans/alpha"
	var err error
	database, err = serverdb.NewService(filepath.Join(t.TempDir(), "plan-dir-rel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Queries.UpsertDiscoveredPlanWorkspace(
		t.Context(),
		db.UpsertDiscoveredPlanWorkspaceParams{
			PlanDirRel:        planRel,
			ProjectID:         "project-1",
			PlanDir:           planAbs,
			Label:             "alpha",
			ArtifactUpdatedAt: time.Now().UTC(),
			QrspiLifecycle:    "plan",
		},
	); err != nil {
		t.Fatal(err)
	}
	service = &Service{
		projectRoot:  projectRoot,
		thoughtsRoot: thoughtsRoot,
		queries:      database.Queries,
	}
	return projectRoot, thoughtsRoot, planAbs, planRel, service, database
}

func TestBackfillAgentThreadPlanDirRelsSetsFKWhenPlanExists(t *testing.T) {
	_, _, planAbs, planRel, service, database := setupPlanDirRelTest(t)
	threadID := "thread-backfill-hit"
	if _, err := database.Queries.CreateAgentThread(
		t.Context(),
		db.CreateAgentThreadParams{
			ID:        threadID,
			UserEmail: "owner@example.com",
			Title:     "Alpha",
			Cwd:       planAbs,
			LineageID: threadID,
			ProjectID: "project-1",
		},
	); err != nil {
		t.Fatal(err)
	}

	result, err := service.BackfillAgentThreadPlanDirRels(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 {
		t.Fatalf(
			"Updated = %d, want 1 (scanned=%d unresolved=%d)",
			result.Updated,
			result.Scanned,
			result.Unresolved,
		)
	}
	got, err := database.Queries.GetSharedAgentThread(t.Context(), threadID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.PlanDirRel.Valid || got.PlanDirRel.String != planRel {
		t.Fatalf("PlanDirRel = %+v, want %q", got.PlanDirRel, planRel)
	}
}

func TestBackfillAgentThreadPlanDirRelsLeavesUnresolvedNULL(t *testing.T) {
	_, _, _, _, service, database := setupPlanDirRelTest(t)
	threadID := "thread-backfill-miss"
	if _, err := database.Queries.CreateAgentThread(
		t.Context(),
		db.CreateAgentThreadParams{
			ID:        threadID,
			UserEmail: "owner@example.com",
			Title:     "Freeform",
			Cwd:       filepath.Join(t.TempDir(), "somewhere-else"),
			LineageID: threadID,
			ProjectID: "project-1",
		},
	); err != nil {
		t.Fatal(err)
	}

	result, err := service.BackfillAgentThreadPlanDirRels(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.Unresolved < 1 {
		t.Fatalf("Unresolved = %d, want >=1", result.Unresolved)
	}
	got, err := database.Queries.GetSharedAgentThread(t.Context(), threadID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PlanDirRel.Valid {
		t.Fatalf("PlanDirRel = %+v, want NULL", got.PlanDirRel)
	}
}

func TestListWorkbenchThreadsPrefersPlanDirRelFK(t *testing.T) {
	_, _, planAbs, planRel, service, database := setupPlanDirRelTest(t)
	threadID := "thread-fk-prefer"
	// Misleading cwd that would otherwise not group under alpha.
	if _, err := database.Queries.CreateAgentThread(
		t.Context(),
		db.CreateAgentThreadParams{
			ID:         threadID,
			UserEmail:  "owner@example.com",
			Title:      "Linked",
			Cwd:        filepath.Join(t.TempDir(), "not-a-plan"),
			LineageID:  threadID,
			ProjectID:  "project-1",
			PlanDirRel: sql.NullString{String: planRel, Valid: true},
		},
	); err != nil {
		t.Fatal(err)
	}

	groups, err := service.ListWorkbenchThreads(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, group := range groups {
		for _, thread := range group.Threads {
			if thread.ID != threadID {
				continue
			}
			found = true
			if !sameFilesystemPath(thread.PlanDir, planAbs) {
				t.Fatalf("PlanDir = %q, want %q", thread.PlanDir, planAbs)
			}
		}
	}
	if !found {
		t.Fatal("thread missing from workbench groups")
	}
}

func TestListPlanHomeThreadsExcludesFreeformNULL(t *testing.T) {
	_, _, planAbs, planRel, service, database := setupPlanDirRelTest(t)
	linkedID := "thread-linked"
	freeformID := "thread-freeform"
	if _, err := database.Queries.CreateAgentThread(
		t.Context(),
		db.CreateAgentThreadParams{
			ID:         linkedID,
			UserEmail:  "owner@example.com",
			Title:      "Linked",
			Cwd:        planAbs,
			LineageID:  linkedID,
			ProjectID:  "project-1",
			PlanDirRel: sql.NullString{String: planRel, Valid: true},
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Queries.CreateAgentThread(
		t.Context(),
		db.CreateAgentThreadParams{
			ID:        freeformID,
			UserEmail: "owner@example.com",
			Title:     "Freeform but cwd looks like plan",
			Cwd:       planAbs,
			LineageID: freeformID,
			ProjectID: "project-1",
		},
	); err != nil {
		t.Fatal(err)
	}

	rows, err := service.ListPlanHomeThreads(t.Context(), planRel)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != linkedID {
		t.Fatalf("ListPlanHomeThreads = %+v, want only %s", rows, linkedID)
	}
}

func TestMostRecentPlanHomeThreadUsesUpdatedAt(t *testing.T) {
	_, _, planAbs, planRel, service, database := setupPlanDirRelTest(t)
	olderID := "thread-older"
	newerID := "thread-newer"
	for _, id := range []string{olderID, newerID} {
		if _, err := database.Queries.CreateAgentThread(
			t.Context(),
			db.CreateAgentThreadParams{
				ID:         id,
				UserEmail:  "owner@example.com",
				Title:      id,
				Cwd:        planAbs,
				LineageID:  id,
				ProjectID:  "project-1",
				PlanDirRel: sql.NullString{String: planRel, Valid: true},
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	// Force distinct updated_at values (SQLite CURRENT_TIMESTAMP is second-granular).
	if _, err := database.DB().ExecContext(
		t.Context(),
		`UPDATE agent_threads SET updated_at = datetime('2026-01-01 00:00:00') WHERE id = ?`,
		olderID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB().ExecContext(
		t.Context(),
		`UPDATE agent_threads SET updated_at = datetime('2026-01-02 00:00:00'), title = 'newer-title' WHERE id = ?`,
		newerID,
	); err != nil {
		t.Fatal(err)
	}

	got, ok, err := service.MostRecentPlanHomeThread(t.Context(), planRel, planAbs)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected most-recent thread")
	}
	if got.ID != newerID {
		t.Fatalf("MostRecentPlanHomeThread = %q, want %q", got.ID, newerID)
	}
}

func TestEnsureSharedThreadForDocCreatesThenReuses(t *testing.T) {
	_, _, planAbs, _, service, _ := setupPlanDirRelTest(t)
	doc := "thoughts/owner/plans/alpha/design.md"
	if err := os.WriteFile(
		filepath.Join(planAbs, "design.md"),
		[]byte("# Design"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	first, err := service.EnsureSharedThreadForDoc(
		t.Context(), doc, "owner@example.com",
	)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" {
		t.Fatal("expected created thread id")
	}
	second, err := service.EnsureSharedThreadForDoc(
		t.Context(), doc, "owner@example.com",
	)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("EnsureSharedThreadForDoc reused %q, want %q", second, first)
	}
	got, err := service.FindSharedThreadForDoc(t.Context(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if got != first {
		t.Fatalf("FindSharedThreadForDoc = %q, want %q", got, first)
	}
}

func TestEnsureSharedThreadForDocIgnoresNonPlan(t *testing.T) {
	_, _, _, _, service, _ := setupPlanDirRelTest(t)
	got, err := service.EnsureSharedThreadForDoc(
		t.Context(),
		"thoughts/owner/notes.md",
		"owner@example.com",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("EnsureSharedThreadForDoc = %q, want empty", got)
	}
}
