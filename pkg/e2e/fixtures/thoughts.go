package fixtures

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func BuildThoughtsWorkbenchBasic(
	ctx context.Context,
	db DBTX,
	input Input,
) (State, error) {
	if input.Workspace.Slug == "" || input.Workspace.Slug == "main" {
		return State{}, fmt.Errorf("fixture requires non-main workspace slug")
	}
	if input.Workspace.CheckoutPath == "" || input.Workspace.DBPath == "" {
		return State{}, fmt.Errorf("fixture requires workspace checkout and DB path")
	}
	if err := db.QueryRowContext(ctx, "select 1").Scan(new(int)); err != nil {
		return State{}, fmt.Errorf("workspace DB ping: %w", err)
	}
	thoughtsRoot := filepath.Join(input.Workspace.CheckoutPath, "thoughts")
	if resolved, err := filepath.EvalSymlinks(thoughtsRoot); err == nil {
		thoughtsRoot = resolved
	}
	if err := os.MkdirAll(thoughtsRoot, 0o755); err != nil {
		return State{}, err
	}
	if err := os.WriteFile(filepath.Join(thoughtsRoot, "example.md"), []byte("# Example\n"), 0o644); err != nil {
		return State{}, err
	}
	if err := seedThoughtsWorkbenchChat(ctx, db, input, thoughtsRoot); err != nil {
		return State{}, err
	}
	return State{
		Name: "thoughts-workbench.basic",
		Data: map[string]any{"workspace_slug": input.Workspace.Slug},
	}, nil
}

func seedThoughtsWorkbenchChat(
	ctx context.Context,
	db DBTX,
	input Input,
	thoughtsRoot string,
) error {
	if _, err := db.ExecContext(ctx, `
INSERT INTO workspaces (id, user_email, title, root_doc_path, workflow_type, source, selected_thread_id, current_session_id)
VALUES ('ws_1', 'tester@example.com', 'E2E Thoughts Workbench', ?, 'qrspi', 'imported', 'th_1', 'th_1')
ON CONFLICT(id) DO UPDATE SET selected_thread_id = 'th_1', current_session_id = 'th_1', updated_at = CURRENT_TIMESTAMP`, thoughtsRoot); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO agent_threads (id, user_email, title, cwd, lineage_id)
VALUES ('th_1', 'tester@example.com', 'E2E Thoughts Thread', ?, 'th_1')
ON CONFLICT(id) DO NOTHING`, input.Workspace.CheckoutPath); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `
INSERT INTO agent_thread_workspaces (thread_id, workspace_id, is_primary, role, adopted_from, adopted_at)
VALUES ('th_1', 'ws_1', 1, 'primary', 'e2e_fixture', CURRENT_TIMESTAMP)
ON CONFLICT(thread_id, workspace_id) DO UPDATE SET is_primary = 1, role = 'primary', adopted_at = CURRENT_TIMESTAMP`)
	return err
}
