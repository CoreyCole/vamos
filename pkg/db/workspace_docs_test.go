package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func openWorkspaceDocsTestDB(t *testing.T) (*sql.DB, *Queries) {
	t.Helper()
	dbConn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	schema, err := os.ReadFile(filepath.Join("migrations", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if _, err := dbConn.Exec(string(schema)); err != nil {
		t.Fatalf("exec schema: %v", err)
	}
	return dbConn, New(dbConn)
}

func TestResolveWorkspaceForDocPathPrefersCurrentUserThenSharedFallback(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)

	insertWorkspaceDoc := func(id, userEmail, updatedAt string) {
		t.Helper()
		if _, err := q.db.ExecContext(ctx, `
INSERT INTO workspaces (id, user_email, title, root_doc_path, workflow_type, source, created_at, updated_at)
VALUES (?, ?, ?, ?, 'freeform', 'web', ?, ?)
`, id, userEmail, id, "thoughts/shared/workspace", updatedAt, updatedAt); err != nil {
			t.Fatalf("insert workspace %s: %v", id, err)
		}
		if err := q.UpsertWorkspaceDoc(ctx, UpsertWorkspaceDocParams{
			WorkspaceID: id,
			DocPath:     "thoughts/shared/workspace/design.md",
			RelPath:     "design.md",
			Kind:        "file",
			Title:       "design.md",
		}); err != nil {
			t.Fatalf("upsert workspace doc %s: %v", id, err)
		}
	}

	insertWorkspaceDoc("coworker-new", "coworker@example.com", "2026-05-17T10:00:00Z")
	insertWorkspaceDoc("current-old", "user@example.com", "2026-05-17T09:00:00Z")

	row, err := q.ResolveWorkspaceForDocPath(ctx, ResolveWorkspaceForDocPathParams{
		DocPath:   "thoughts/shared/workspace/design.md",
		UserEmail: "user@example.com",
	})
	if err != nil {
		t.Fatalf("ResolveWorkspaceForDocPath(current user): %v", err)
	}
	if row.ID != "current-old" {
		t.Fatalf("resolved workspace = %q, want current user's workspace", row.ID)
	}

	row, err = q.ResolveWorkspaceForDocPath(ctx, ResolveWorkspaceForDocPathParams{
		DocPath:   "thoughts/shared/workspace/design.md",
		UserEmail: "new-viewer@example.com",
	})
	if err != nil {
		t.Fatalf("ResolveWorkspaceForDocPath(fallback): %v", err)
	}
	if row.ID != "coworker-new" {
		t.Fatalf("fallback workspace = %q, want latest coworker workspace", row.ID)
	}
}

func TestListWorkspaceDocsUsesWorkspaceRelPathIndex(t *testing.T) {
	ctx := context.Background()
	dbConn, q := openWorkspaceDocsTestDB(t)
	if _, err := q.db.ExecContext(ctx, `
INSERT INTO workspaces (id, user_email, title, root_doc_path, workflow_type, source)
VALUES ('workspace-1', 'user@example.com', 'Workspace', 'thoughts/demo', 'freeform', 'web')
`); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	for _, rel := range []string{"design.md", "research/notes.md"} {
		if err := q.UpsertWorkspaceDoc(ctx, UpsertWorkspaceDocParams{
			WorkspaceID: "workspace-1",
			DocPath:     "thoughts/demo/" + rel,
			RelPath:     rel,
			Kind:        "file",
			Title:       rel,
		}); err != nil {
			t.Fatalf("upsert %s: %v", rel, err)
		}
	}
	rows, err := dbConn.QueryContext(ctx, `EXPLAIN QUERY PLAN
SELECT * FROM workspace_docs
WHERE workspace_id = ? AND deleted_at IS NULL
ORDER BY rel_path ASC`, "workspace-1")
	if err != nil {
		t.Fatalf("explain query plan: %v", err)
	}
	defer rows.Close()
	plan := ""
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan explain: %v", err)
		}
		plan += detail + "\n"
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("explain rows: %v", err)
	}
	if !strings.Contains(plan, "idx_workspace_docs_workspace_rel_path") {
		t.Fatalf("query plan = %q, want idx_workspace_docs_workspace_rel_path", plan)
	}
}
