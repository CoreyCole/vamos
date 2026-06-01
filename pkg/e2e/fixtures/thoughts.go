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
	thoughtsRoot := strings.TrimSpace(input.ThoughtsRoot)
	if thoughtsRoot == "" {
		thoughtsRoot = filepath.Join(input.Workspace.CheckoutPath, "thoughts")
	}
	if err := os.MkdirAll(thoughtsRoot, 0o755); err != nil {
		return State{}, err
	}
	if err := os.WriteFile(filepath.Join(thoughtsRoot, "example.md"), []byte("# Example\n"), 0o644); err != nil {
		return State{}, err
	}
	outlinePath := filepath.Join(thoughtsRoot, "owner", "plans", "demo", "outline.md")
	if err := os.MkdirAll(filepath.Dir(outlinePath), 0o755); err != nil {
		return State{}, err
	}
	if err := os.WriteFile(outlinePath, []byte("# Demo Outline\n"), 0o644); err != nil {
		return State{}, err
	}
	if err := resetThoughtsWorkbenchLayout(ctx, db); err != nil {
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

func BuildThoughtsWorkbenchQRSPILifecycle(ctx context.Context, db DBTX, input Input) (State, error) {
	state, err := BuildThoughtsWorkbenchBasic(ctx, db, input)
	if err != nil {
		return State{}, err
	}
	thoughtsRoot := strings.TrimSpace(input.ThoughtsRoot)
	if thoughtsRoot == "" {
		thoughtsRoot = filepath.Join(input.Workspace.CheckoutPath, "thoughts")
	}
	plans := []struct {
		name  string
		stage string
	}{
		{name: "question-plan", stage: "question"},
		{name: "merged-plan", stage: "merged"},
		{name: "closed-plan", stage: "closed"},
	}
	for _, plan := range plans {
		dir := filepath.Join(thoughtsRoot, "creative-mode-agent", "plans", plan.name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return State{}, err
		}
		if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("---\nqrspi_lifecycle: "+plan.stage+"\n---\n# "+plan.name+"\n"), 0o644); err != nil {
			return State{}, err
		}
		rel := filepath.ToSlash(filepath.Join("creative-mode-agent", "plans", plan.name))
		if _, err := db.ExecContext(ctx, `
INSERT INTO plan_workspaces (plan_dir_rel, plan_dir, label, artifact_updated_at, qrspi_lifecycle, qrspi_closed_reason, last_discovered_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP, ?, '', CURRENT_TIMESTAMP)
ON CONFLICT(plan_dir_rel) DO UPDATE SET plan_dir = excluded.plan_dir, label = excluded.label, artifact_updated_at = CURRENT_TIMESTAMP, qrspi_lifecycle = excluded.qrspi_lifecycle, archived_at = NULL, last_discovered_at = CURRENT_TIMESTAMP`, rel, dir, plan.name, plan.stage); err != nil {
			if strings.Contains(err.Error(), "no such table") {
				continue
			}
			return State{}, err
		}
	}
	state.Name = "thoughts-workbench.qrspi-lifecycle"
	return state, nil
}

func resetThoughtsWorkbenchLayout(ctx context.Context, db DBTX) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM layout_preferences WHERE user_email = 'playwright@localhost' AND page = 'thoughts'`); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	return nil
}

func seedThoughtsWorkbenchChat(
	ctx context.Context,
	db DBTX,
	input Input,
	thoughtsRoot string,
) error {
	if _, err := db.ExecContext(ctx, `
INSERT INTO workspaces (id, user_email, title, root_doc_path, workflow_type, source, selected_thread_id, current_session_id)
VALUES ('ws_1', 'playwright@localhost', 'E2E Thoughts Workbench', ?, 'qrspi', 'imported', 'th_1', 'th_1')
ON CONFLICT(id) DO UPDATE SET user_email = excluded.user_email, root_doc_path = excluded.root_doc_path, selected_thread_id = 'th_1', current_session_id = 'th_1', updated_at = CURRENT_TIMESTAMP`, thoughtsRoot); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO agent_threads (id, user_email, title, cwd, lineage_id)
VALUES ('th_1', 'playwright@localhost', 'E2E Thoughts Thread', ?, 'th_1')
ON CONFLICT(id) DO UPDATE SET user_email = excluded.user_email, cwd = excluded.cwd`, input.Workspace.CheckoutPath); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `
INSERT INTO agent_thread_workspaces (thread_id, workspace_id, is_primary, role, adopted_from, adopted_at)
VALUES ('th_1', 'ws_1', 1, 'primary', 'e2e_fixture', CURRENT_TIMESTAMP)
ON CONFLICT(thread_id, workspace_id) DO UPDATE SET is_primary = 1, role = 'primary', adopted_at = CURRENT_TIMESTAMP`)
	return err
}
