package fixtures

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const WorkbenchV2Fixture = "workbench-v2.complete"

func BuildWorkbenchV2(ctx context.Context, db DBTX, input Input) (State, error) {
	state, err := BuildThoughtsWorkbenchBasic(ctx, db, input)
	if err != nil {
		return State{}, err
	}
	thoughtsRoot := strings.TrimSpace(input.ThoughtsRoot)
	if thoughtsRoot == "" {
		thoughtsRoot = filepath.Join(input.Workspace.CheckoutPath, "thoughts")
	}
	escapeTarget := thoughtsRoot + "-sibling"
	if err := os.MkdirAll(escapeTarget, 0o755); err != nil {
		return State{}, err
	}
	if err := os.WriteFile(
		filepath.Join(escapeTarget, "secret.md"),
		[]byte("outside thoughts root\n"),
		0o644,
	); err != nil {
		return State{}, err
	}
	files := map[string]string{
		"workbench-v2/root.md":                           "# Workbench v2 root\n",
		"workbench-v2/nested/needle.md":                  "# Nested needle\n",
		"workbench-v2/nested/source.go":                  "package fixture\n",
		"workbench-v2/nested/data.csv":                   "name,value\nAda,1\n",
		"workbench-v2/static.html":                       "<h1>Opaque fixture</h1><p>Comments cross the bridge.</p>",
		"owner/plans/alpha/design.md":                    "# Alpha design\n",
		"owner/plans/alpha/notes.md":                     "# Alpha notes\n\nSibling artifact for chat-jank Story.\n",
		"owner/plans/beta/design.md":                     "# Beta design\n",
		"owner/plans/beta/cross-plan-artifact.md":        "# Cross-plan artifact\n\nCross-plan selected text.\n\n[Open root artifact](/thoughts/workbench-v2/root.md)\n",
		"owner/plans/beta/cross-plan-directory/entry.md": "# Cross-plan directory entry\n",
	}
	for relative, contents := range files {
		path := filepath.Join(thoughtsRoot, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return State{}, err
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			return State{}, err
		}
	}
	symlink := filepath.Join(thoughtsRoot, "workbench-v2", "symlink-escape")
	_ = os.Remove(symlink)
	if err := os.Symlink(escapeTarget, symlink); err != nil && !os.IsExist(err) {
		return State{}, err
	}
	if err := seedWorkbenchV2Applets(
		thoughtsRoot,
		input.Workspace.CheckoutPath,
	); err != nil {
		return State{}, err
	}
	if err := cleanupWorkbenchV2State(ctx, db); err != nil {
		return State{}, err
	}
	if err := seedWorkbenchV2Threads(ctx, db, input, thoughtsRoot); err != nil {
		return State{}, err
	}
	state.Name = WorkbenchV2Fixture
	state.Data = map[string]any{
		"alpha_thread": "wb2_alpha",
		"beta_thread":  "wb2_beta",
		"alpha_design": "thoughts/owner/plans/alpha/design.md",
		"cross_file":   "thoughts/owner/plans/beta/cross-plan-artifact.md",
		"cross_dir":    "thoughts/owner/plans/beta/cross-plan-directory",
		"alpha_notes":  "thoughts/owner/plans/alpha/notes.md",
		"jank_early":   "WB2_CHAT_JANK_USER_01",
		"jank_late":    "WB2_CHAT_JANK_ASSIST_12",
	}
	return state, nil
}

func cleanupWorkbenchV2State(ctx context.Context, db DBTX) error {
	statements := []string{
		`DELETE FROM layout_preferences WHERE user_email IN ('playwright@localhost', 'playwright-secondary@localhost') AND page = 'threads'`,
		`DELETE FROM document_comment_replies WHERE comment_id IN (SELECT id FROM document_comments WHERE doc_path IN ('thoughts/workbench-v2/root.md', 'thoughts/workbench-v2/nested/needle.md', 'thoughts/workbench-v2/nested/source.go', 'thoughts/workbench-v2/nested/data.csv', 'thoughts/workbench-v2/static.html', 'thoughts/owner/plans/alpha/design.md', 'thoughts/owner/plans/alpha/notes.md', 'thoughts/owner/plans/beta/design.md', 'thoughts/owner/plans/beta/cross-plan-artifact.md', 'thoughts/owner/plans/beta/cross-plan-directory/entry.md', 'thoughts/v2-wordle/AGENTS.md', 'thoughts/v2-streamlit/AGENTS.md'))`,
		`DELETE FROM document_comments WHERE doc_path IN ('thoughts/workbench-v2/root.md', 'thoughts/workbench-v2/nested/needle.md', 'thoughts/workbench-v2/nested/source.go', 'thoughts/workbench-v2/nested/data.csv', 'thoughts/workbench-v2/static.html', 'thoughts/owner/plans/alpha/design.md', 'thoughts/owner/plans/alpha/notes.md', 'thoughts/owner/plans/beta/design.md', 'thoughts/owner/plans/beta/cross-plan-artifact.md', 'thoughts/owner/plans/beta/cross-plan-directory/entry.md', 'thoughts/v2-wordle/AGENTS.md', 'thoughts/v2-streamlit/AGENTS.md')`,
		`DELETE FROM workspace_events WHERE thread_id IN ('wb2_alpha', 'wb2_beta', 'wb2_rejected') OR run_id IN (SELECT id FROM agent_runs WHERE thread_id IN ('wb2_alpha', 'wb2_beta', 'wb2_rejected'))`,
		`DELETE FROM agent_surface_attachments WHERE run_id IN (SELECT id FROM agent_runs WHERE thread_id IN ('wb2_alpha', 'wb2_beta', 'wb2_rejected'))`,
		`DELETE FROM chat_session_events WHERE run_id IN (SELECT id FROM agent_runs WHERE thread_id IN ('wb2_alpha', 'wb2_beta', 'wb2_rejected'))`,
		`DELETE FROM agent_run_attachments WHERE thread_id IN ('wb2_alpha', 'wb2_beta', 'wb2_rejected')`,
		`DELETE FROM agent_entries WHERE origin_thread_id IN ('wb2_alpha', 'wb2_beta', 'wb2_rejected')`,
		`DELETE FROM agent_thread_drafts WHERE thread_id IN ('wb2_alpha', 'wb2_beta', 'wb2_rejected')`,
		`DELETE FROM agent_runs WHERE thread_id IN ('wb2_alpha', 'wb2_beta', 'wb2_rejected')`,
		`DELETE FROM agent_sessions WHERE projected_thread_id IN ('wb2_alpha', 'wb2_beta', 'wb2_rejected')`,
		`DELETE FROM agent_thread_workspaces WHERE thread_id IN ('wb2_alpha', 'wb2_beta', 'wb2_rejected')`,
		`DELETE FROM agent_threads WHERE id IN ('wb2_alpha', 'wb2_beta', 'wb2_rejected')`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(
			ctx,
			statement,
		); err != nil &&
			!strings.Contains(err.Error(), "no such table") {
			return fmt.Errorf("clean workbench v2 fixture: %w", err)
		}
	}
	return nil
}

func seedWorkbenchV2Applets(thoughtsRoot, checkout string) error {
	for _, applet := range []struct{ name, id, kind, source, command, healthPath string }{
		{"v2-wordle", "e2e-v2-wordle", "datastar", "examples/wordle", "[just, build]", ""},
		// The fixture exercises Streamlit through its scoped app route. It must not
		// claim Streamlit's global aliases, which the repository example owns.
		{"v2-streamlit", "e2e-v2-streamlit", "http", "examples/streamlit", "[./start.sh]", "/_stcore/health"},
	} {
		directory := filepath.Join(thoughtsRoot, applet.name)
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
		healthPath := ""
		if applet.healthPath != "" {
			healthPath = "  health_path: " + applet.healthPath + "\n"
		}
		manifest := fmt.Sprintf(
			"---\nvamos_artifact: applet\napplet:\n  id: %s\n  title: E2E %s\n  kind: %s\n  source_dir: %s\n  files_root: files\n%s  start_command: %s\n---\n# E2E %s\n",
			applet.id,
			applet.name,
			applet.kind,
			filepath.ToSlash(filepath.Join(checkout, applet.source)),
			healthPath,
			applet.command,
			applet.name,
		)
		if err := os.WriteFile(
			filepath.Join(directory, "AGENTS.md"),
			[]byte(manifest),
			0o644,
		); err != nil {
			return err
		}
	}
	return nil
}

func seedWorkbenchV2Threads(
	ctx context.Context,
	db DBTX,
	input Input,
	thoughtsRoot string,
) error {
	for _, thread := range []struct {
		id, title, plan string
	}{
		{"wb2_alpha", "Workbench Alpha", "owner/plans/alpha"},
		{"wb2_beta", "Workbench Beta", "owner/plans/beta"},
		{"wb2_rejected", "Workbench Rejected", "owner/plans/alpha"},
	} {
		cwd := filepath.ToSlash(filepath.Join("thoughts", thread.plan))
		if _, err := db.ExecContext(ctx, `
INSERT INTO agent_threads (id, user_email, title, cwd, lineage_id)
VALUES (?, 'playwright@localhost', ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET title = excluded.title, cwd = excluded.cwd`, thread.id, thread.title, cwd, thread.id); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `
INSERT INTO agent_thread_workspaces (thread_id, workspace_id, is_primary, role, adopted_from, adopted_at)
VALUES (?, 'ws_1', 1, 'primary', 'e2e_fixture', CURRENT_TIMESTAMP)
ON CONFLICT(thread_id, workspace_id) DO UPDATE SET is_primary = 1, role = 'primary', adopted_at = CURRENT_TIMESTAMP`, thread.id); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO agent_thread_drafts (user_email, thread_id, content, operation_order)
VALUES ('playwright@localhost', 'wb2_alpha', 'completed alpha draft', 1),
       ('playwright@localhost', 'wb2_beta', 'completed beta draft', 1),
       ('playwright@localhost', 'wb2_rejected', 'rejected thread draft', 1)
ON CONFLICT(user_email, thread_id) DO UPDATE SET content = excluded.content, operation_order = excluded.operation_order`); err != nil && !strings.Contains(err.Error(), "no such table") {
		return fmt.Errorf("seed workbench v2 drafts: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO agent_runs (id, workspace_id, thread_id, trigger, status, prompt_text, workflow_id, root_doc_path)
VALUES ('wb2_rejected_active_run', 'ws_1', 'wb2_rejected', 'resume', 'running', 'fixture active run', 'wb2-rejected-fixture', ?)
ON CONFLICT(id) DO UPDATE SET status = 'running', thread_id = excluded.thread_id`, thoughtsRoot); err != nil && !strings.Contains(err.Error(), "no such table") {
		return fmt.Errorf("seed rejected workbench v2 run: %w", err)
	}
	if err := seedWorkbenchV2LongTranscript(ctx, db); err != nil && !strings.Contains(err.Error(), "no such table") {
		return err
	}
	return nil
}

func seedWorkbenchV2LongTranscript(ctx context.Context, db DBTX) error {
	const threadID = "wb2_alpha"
	const pairs = 12
	var parent any
	parent = nil
	headID := ""
	order := 0
	for turn := 1; turn <= pairs; turn++ {
		userID := fmt.Sprintf("wb2_alpha_jank_user_%02d", turn)
		assistantID := fmt.Sprintf("wb2_alpha_jank_assist_%02d", turn)
		userContent := fmt.Sprintf(
			"WB2_CHAT_JANK_USER_%02d\nfixture user turn %d\npad-a\npad-b\npad-c\npad-d\npad-e",
			turn,
			turn,
		)
		assistantContent := fmt.Sprintf(
			"WB2_CHAT_JANK_ASSIST_%02d\nfixture assistant turn %d\npad-a\npad-b\npad-c\npad-d\npad-e\npad-f\npad-g",
			turn,
			turn,
		)
		order++
		userPayload := fmt.Sprintf(
			`{"type":"message","id":"%s","message":{"role":"user","content":%q}}`,
			userID,
			userContent,
		)
		if _, err := db.ExecContext(ctx, `
INSERT INTO agent_entries (lineage_id, entry_id, parent_entry_id, entry_type, origin_order, payload_json, origin_thread_id, session_timestamp)
VALUES (?, ?, ?, 'message', ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(lineage_id, entry_id) DO UPDATE SET
parent_entry_id = excluded.parent_entry_id,
origin_order = excluded.origin_order,
payload_json = excluded.payload_json,
origin_thread_id = excluded.origin_thread_id`,
			threadID, userID, parent, order, userPayload, threadID,
		); err != nil {
			return fmt.Errorf("seed workbench v2 jank user entry %d: %w", turn, err)
		}
		order++
		assistantPayload := fmt.Sprintf(
			`{"type":"message","id":"%s","parentId":"%s","message":{"role":"assistant","content":%q}}`,
			assistantID,
			userID,
			assistantContent,
		)
		if _, err := db.ExecContext(ctx, `
INSERT INTO agent_entries (lineage_id, entry_id, parent_entry_id, entry_type, origin_order, payload_json, origin_thread_id, session_timestamp)
VALUES (?, ?, ?, 'message', ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(lineage_id, entry_id) DO UPDATE SET
parent_entry_id = excluded.parent_entry_id,
origin_order = excluded.origin_order,
payload_json = excluded.payload_json,
origin_thread_id = excluded.origin_thread_id`,
			threadID, assistantID, userID, order, assistantPayload, threadID,
		); err != nil {
			return fmt.Errorf("seed workbench v2 jank assistant entry %d: %w", turn, err)
		}
		parent = assistantID
		headID = assistantID
	}
	if _, err := db.ExecContext(ctx, `
UPDATE agent_threads SET head_entry_id = ?, lineage_id = ? WHERE id = ?`,
		headID, threadID, threadID,
	); err != nil {
		return fmt.Errorf("set workbench v2 jank head entry: %w", err)
	}
	return nil
}
