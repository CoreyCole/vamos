package fixtures

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

const DurableFreeformFixture = "freeform-chat.durable"

func BuildFreeformDurableChat(
	ctx context.Context,
	db DBTX,
	input Input,
) (State, error) {
	if input.Workspace.Slug == "" || input.Workspace.Slug == "main" {
		return State{}, fmt.Errorf("fixture requires non-main workspace slug")
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000000")
	workspaceID := "e2e-freeform-ws-" + stamp
	threadID := "e2e-freeform-session-" + stamp
	branchID := "e2e-freeform-branch-" + stamp
	rootDocPath := filepath.Join(
		input.Workspace.CheckoutPath,
		"thoughts",
		"creative-mode-agent",
		"plans",
		"2026-05-20_23-02-59_vamos-e2e-story-playwright-go",
	)
	docPath := "creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/plan.md"
	lineageID := "e2e-freeform-lineage-" + stamp
	userEntryID := "e2e-freeform-user-" + stamp
	assistantEntryID := "e2e-freeform-assistant-" + stamp

	if _, err := db.ExecContext(ctx, `
INSERT INTO workspaces (id, user_email, title, root_doc_path, workflow_type, source, selected_thread_id, current_session_id, current_branch_id)
VALUES (?, 'playwright@localhost', 'E2E durable freeform chat', ?, 'freeform', 'imported', ?, ?, ?)`, workspaceID, rootDocPath, threadID, threadID, branchID); err != nil {
		return State{}, fmt.Errorf("insert freeform workspace: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO agent_threads (id, user_email, title, cwd, lineage_id, head_entry_id)
VALUES (?, 'playwright@localhost', 'E2E durable freeform thread', ?, ?, ?)`, threadID, input.Workspace.CheckoutPath, lineageID, assistantEntryID); err != nil {
		return State{}, fmt.Errorf("insert freeform agent thread: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO agent_thread_workspaces (thread_id, workspace_id, is_primary, role, adopted_from, adopted_at)
VALUES (?, ?, 1, 'primary', 'e2e_fixture', CURRENT_TIMESTAMP)
ON CONFLICT(thread_id, workspace_id) DO UPDATE SET
is_primary = 1,
role = 'primary',
adopted_at = CURRENT_TIMESTAMP`, threadID, workspaceID); err != nil {
		return State{}, fmt.Errorf("attach freeform agent thread to workspace: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO agent_entries (lineage_id, entry_id, parent_entry_id, entry_type, origin_order, payload_json, origin_thread_id, session_timestamp)
VALUES (?, ?, NULL, 'message', 1, ?, ?, CURRENT_TIMESTAMP)`, lineageID, userEntryID, `{"type":"message","id":"`+userEntryID+`","parentId":null,"message":{"role":"user","content":"Seeded durable user message VAMOS_E2E_FREEFORM_REPLAY_OK"}}`, threadID); err != nil {
		return State{}, fmt.Errorf("insert freeform user entry: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO agent_entries (lineage_id, entry_id, parent_entry_id, entry_type, origin_order, payload_json, origin_thread_id, session_timestamp)
VALUES (?, ?, ?, 'message', 2, ?, ?, CURRENT_TIMESTAMP)`, lineageID, assistantEntryID, userEntryID, `{"type":"message","id":"`+assistantEntryID+`","parentId":"`+userEntryID+`","message":{"role":"assistant","content":"Seeded durable assistant response VAMOS_E2E_FREEFORM_REPLAY_OK"}}`, threadID); err != nil {
		return State{}, fmt.Errorf("insert freeform assistant entry: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO chat_sessions (id, workspace_id, created_by_user_email, branch_id, topology_kind, current_projection_seq)
VALUES (?, ?, 'playwright@localhost', ?, 'root', 2)`, threadID, workspaceID, branchID); err != nil {
		return State{}, fmt.Errorf("insert freeform chat session: %w", err)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO chat_session_sequences (session_id, next_seq) VALUES (?, 3)`,
		threadID,
	); err != nil {
		return State{}, fmt.Errorf("insert freeform sequence: %w", err)
	}
	messages := `[{"id":"msg-user","role":"user","content":"Seeded durable user message VAMOS_E2E_FREEFORM_REPLAY_OK"},{"id":"msg-assistant","role":"assistant","content":"Seeded durable assistant response VAMOS_E2E_FREEFORM_REPLAY_OK"}]`
	participants := `[{"id":"playwright","displayName":"Playwright"},{"id":"assistant","displayName":"Assistant"}]`
	if _, err := db.ExecContext(ctx, `
INSERT INTO chat_session_projections (session_id, last_seq, messages_json, runs_json, participants_json, artifacts_json, topology_json)
VALUES (?, 2, ?, '[]', ?, '[]', '{}')`, threadID, messages, participants); err != nil {
		return State{}, fmt.Errorf("insert freeform projection: %w", err)
	}
	return State{
		Name: DurableFreeformFixture,
		Data: map[string]any{
			"workspace_id":  workspaceID,
			"thread_id":     threadID,
			"root_doc_path": docPath,
		},
	}, nil
}
