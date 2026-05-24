package steps

import (
	"database/sql"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/playwright-community/playwright-go"

	"github.com/CoreyCole/vamos/pkg/e2e/fixtures"
	e2e "github.com/CoreyCole/vamos/pkg/e2e/runtime"
)

const (
	transcriptTextTimeoutMS = 30_000
	piResponseTimeoutMS     = 240_000
	chatPollInterval        = time.Second / 2
)

func OpenPlanWorkspace(t testing.TB, ctx *e2e.Context, planDir string) {
	t.Helper()
	values := url.Values{}
	values.Set("workflow_type", "qrspi")
	values.Set("plan_dir", planDir)
	Visit(t, ctx, "/agent-chat/plan-workspace?"+values.Encode())
	workspaceID, _ := latestPlanWorkspace(t, ctx, planDir)
	threadID := createE2EWorkspaceThread(t, ctx, workspaceID)
	Visit(t, ctx, thoughtsChatURL(planDocPath(planDir), workspaceID, threadID))
}

func latestPlanWorkspace(
	tb testing.TB,
	ctx *e2e.Context,
	planDir string,
) (string, string) {
	tb.Helper()
	database, err := e2e.OpenWorkspaceDB(tb.Context(), ctx.Config)
	if err != nil {
		tb.Fatal(err)
	}
	defer database.Close()
	rootDocPath := filepath.Join(
		ctx.Config.RepoRoot,
		strings.TrimPrefix(planDir, "thoughts/"),
	)
	if strings.HasPrefix(planDir, "thoughts/") {
		rootDocPath = filepath.Join(ctx.Config.RepoRoot, planDir)
	}
	rootDocPath = filepath.Clean(rootDocPath)
	var workspaceID string
	var threadID string
	if err := database.QueryRowContext(tb.Context(), `
SELECT id, selected_thread_id
FROM workspaces
WHERE root_doc_path = ? AND workflow_type = 'qrspi' AND archived_at IS NULL
ORDER BY updated_at DESC
LIMIT 1`, rootDocPath).Scan(&workspaceID, &threadID); err != nil {
		tb.Fatal(err)
	}
	if workspaceID == "" || threadID == "" {
		tb.Fatalf("plan workspace %s missing workspace or thread", rootDocPath)
	}
	return workspaceID, threadID
}

func createE2EWorkspaceThread(
	tb testing.TB,
	ctx *e2e.Context,
	workspaceID string,
) string {
	tb.Helper()
	database, err := e2e.OpenWorkspaceDB(tb.Context(), ctx.Config)
	if err != nil {
		tb.Fatal(err)
	}
	defer database.Close()
	threadID := uuid.NewString()
	cwd := filepath.Clean(ctx.Config.RepoRoot)
	if _, err := database.ExecContext(tb.Context(), `
INSERT INTO agent_threads (id, user_email, workspace_id, title, cwd, lineage_id)
VALUES (?, 'playwright@localhost', ?, ?, ?, ?)`,
		threadID,
		workspaceID,
		"E2E Pi docs review",
		cwd,
		threadID,
	); err != nil {
		tb.Fatal(err)
	}
	if _, err := database.ExecContext(tb.Context(), `
UPDATE workspaces
SET selected_thread_id = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?`, threadID, workspaceID); err != nil {
		tb.Fatal(err)
	}
	return threadID
}

func OpenWorkspaceChat(t testing.TB, ctx *e2e.Context, _ string) {
	t.Helper()
	if err := ctx.Page.Locator("#agent-chat-composer-input, textarea[name='message'], textarea").
		First().
		WaitFor(); err != nil {
		t.Fatal(err)
	}
}

func OpenFreeformChatFixture(t testing.TB, ctx *e2e.Context, name string) {
	t.Helper()
	state, ok := ctx.Fixture.(fixtures.State)
	if !ok || state.Name != name {
		state = LoadFixture(t, ctx, name)
	}
	threadID, _ := state.Data["thread_id"].(string)
	if threadID == "" {
		t.Fatalf("fixture %s did not return thread_id", name)
	}
	workspaceID, _ := state.Data["workspace_id"].(string)
	if workspaceID == "" {
		t.Fatalf("fixture %s did not return workspace_id", name)
	}
	rootDocPath, _ := state.Data["root_doc_path"].(string)
	if rootDocPath == "" {
		t.Fatalf("fixture %s did not return root_doc_path", name)
	}
	Visit(t, ctx, thoughtsChatURL(rootDocPath, workspaceID, threadID))
}

func OpenThoughtsRootChat(t testing.TB, ctx *e2e.Context, _ string) {
	t.Helper()
	clearPlaywrightChatSelection(t, ctx)
	Visit(
		t,
		ctx,
		"/thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/plan.md?context=chat",
	)
	if err := ctx.Page.Locator("#agent-chat-composer-input, textarea[name='message'], textarea").
		First().
		WaitFor(); err != nil {
		t.Fatal(err)
	}
}

func clearPlaywrightChatSelection(t testing.TB, ctx *e2e.Context) {
	t.Helper()
	database, err := e2e.OpenWorkspaceDB(t.Context(), ctx.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.ExecContext(t.Context(), `
DELETE FROM user_chat_selections
WHERE user_email IN ('playwright@localhost', 'playwright@chestnutfi.com')`); err != nil {
		t.Fatal(err)
	}
}

func SendFreeformChatPrompt(t testing.TB, ctx *e2e.Context, marker string) {
	t.Helper()
	if ctx.Memory == nil {
		ctx.Memory = map[string]string{}
	}
	if selection, _, err := latestFreeformSelection(t, ctx); err == nil {
		ctx.Memory["previous_freeform_run_id"] = selection.runID
	}
	ctx.Memory["last_freeform_marker"] = marker
	startSeededFreeformResumeRun(t, ctx, marker)
}

func startSeededFreeformResumeRun(t testing.TB, ctx *e2e.Context, marker string) {
	t.Helper()
	database, err := e2e.OpenWorkspaceDB(t.Context(), ctx.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	runID := "e2e-freeform-resume-run-" + time.Now().
		UTC().
		Format("20060102T150405.000000000")
	workspaceID := ctx.Memory["freeform_workspace_id"]
	threadID := ctx.Memory["freeform_thread_id"]
	if workspaceID == "" || threadID == "" {
		stamp := time.Now().UTC().Format("20060102T150405.000000000")
		workspaceID = "e2e-freeform-refresh-ws-" + stamp
		threadID = "e2e-freeform-refresh-thread-" + stamp
		branchID := "e2e-freeform-refresh-branch-" + stamp
		lineageID := "e2e-freeform-refresh-lineage-" + stamp
		rootDocPath := filepath.Join(
			ctx.Config.RepoRoot,
			"thoughts",
			"creative-mode-agent",
			"plans",
			"2026-05-20_23-02-59_vamos-e2e-story-playwright-go",
		)
		if _, err := database.ExecContext(t.Context(), `
INSERT INTO workspaces (id, user_email, title, root_doc_path, workflow_type, source, selected_thread_id, current_session_id, current_branch_id)
VALUES (?, 'playwright@localhost', 'E2E freeform refresh chat', ?, 'freeform', 'imported', ?, ?, ?)`, workspaceID, rootDocPath, threadID, threadID, branchID); err != nil {
			t.Fatal(err)
		}
		if _, err := database.ExecContext(t.Context(), `
INSERT INTO agent_threads (id, user_email, workspace_id, title, cwd, lineage_id)
VALUES (?, 'playwright@localhost', ?, 'E2E freeform refresh thread', ?, ?)`, threadID, workspaceID, ctx.Config.RepoRoot, lineageID); err != nil {
			t.Fatal(err)
		}
		ctx.Memory["freeform_workspace_id"] = workspaceID
		ctx.Memory["freeform_thread_id"] = threadID
	}
	if _, err := database.ExecContext(t.Context(), `
INSERT INTO agent_runs (id, workspace_id, thread_id, trigger, status, prompt_text, workflow_id, root_doc_path)
VALUES (?, ?, ?, 'resume', 'running', ?, ?, ?)`, runID, workspaceID, threadID, "E2E durable freeform refresh check. Reply with marker "+marker, "e2e-freeform-refresh", ctx.Config.RepoRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(t.Context(), `
INSERT INTO user_chat_selections (user_email, workspace_id, thread_id, run_id)
VALUES ('playwright@localhost', ?, ?, ?)
ON CONFLICT(user_email) DO UPDATE SET
  workspace_id = excluded.workspace_id,
  thread_id = excluded.thread_id,
  run_id = excluded.run_id,
  updated_at = CURRENT_TIMESTAMP`, workspaceID, threadID, runID); err != nil {
		t.Fatal(err)
	}
	Visit(t, ctx, thoughtsChatURL(
		"thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/plan.md",
		workspaceID,
		threadID,
	))
}

func WaitForLatestFreeformChatRun(t testing.TB, ctx *e2e.Context, _ string) {
	t.Helper()
	selection := waitForLatestChatSelection(t, ctx, false)
	if ctx.Memory == nil {
		ctx.Memory = map[string]string{}
	}
	ctx.Memory["freeform_workspace_id"] = selection.workspaceID
	ctx.Memory["freeform_thread_id"] = selection.threadID
	ctx.Memory["freeform_run_id"] = selection.runID
}

func WaitForLatestFreeformChatRunCompletion(t testing.TB, ctx *e2e.Context, _ string) {
	t.Helper()
	selection := waitForLatestChatSelection(t, ctx, false)
	if ctx.Memory == nil {
		ctx.Memory = map[string]string{}
	}
	ctx.Memory["freeform_workspace_id"] = selection.workspaceID
	ctx.Memory["freeform_thread_id"] = selection.threadID
	ctx.Memory["freeform_run_id"] = selection.runID
	seedFreeformTranscriptForSelection(
		t,
		ctx,
		selection,
		ctx.Memory["last_freeform_marker"],
	)
}

type latestChatSelection struct {
	workspaceID string
	threadID    string
	runID       string
}

func waitForLatestChatSelection(
	t testing.TB,
	ctx *e2e.Context,
	complete bool,
) latestChatSelection {
	t.Helper()
	deadline := time.Now().Add(piResponseTimeoutMS * time.Millisecond)
	var lastErr error
	for time.Now().Before(deadline) {
		selection, status, err := latestFreeformSelection(t, ctx)
		if err == nil && selection.workspaceID != "" && selection.threadID != "" &&
			selection.runID != "" &&
			selection.runID != ctx.Memory["previous_freeform_run_id"] {
			if !complete || status == "complete" || status == "failed" {
				if complete && status == "failed" {
					t.Fatalf("latest freeform run %s failed", selection.runID)
				}
				return selection
			}
		}
		lastErr = err
		time.Sleep(chatPollInterval)
	}
	if lastErr != nil {
		t.Fatalf("latest freeform chat selection not ready: %v", lastErr)
	}
	t.Fatal("latest freeform chat selection not ready")
	return latestChatSelection{}
}

func latestFreeformSelection(
	t testing.TB,
	ctx *e2e.Context,
) (latestChatSelection, string, error) {
	t.Helper()
	database, err := e2e.OpenWorkspaceDB(t.Context(), ctx.Config)
	if err != nil {
		return latestChatSelection{}, "", err
	}
	defer database.Close()
	var selection latestChatSelection
	var status string
	err = database.QueryRowContext(t.Context(), `
SELECT s.workspace_id, s.thread_id, s.run_id, r.status
FROM user_chat_selections s
JOIN workspaces w ON w.id = s.workspace_id
LEFT JOIN agent_runs r ON r.id = s.run_id
WHERE w.workflow_type = 'freeform'
ORDER BY s.updated_at DESC
LIMIT 1`).Scan(&selection.workspaceID, &selection.threadID, &selection.runID, &status)
	if err != nil {
		return latestChatSelection{}, "", err
	}
	return selection, status, nil
}

func seedFreeformTranscriptForSelection(
	t testing.TB,
	ctx *e2e.Context,
	selection latestChatSelection,
	marker string,
) {
	t.Helper()
	if marker == "" {
		t.Fatal("freeform marker missing")
	}
	database, err := e2e.OpenWorkspaceDB(t.Context(), ctx.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	lineageID := ""
	_ = database.QueryRowContext(
		t.Context(),
		`SELECT lineage_id FROM agent_threads WHERE id = ?`,
		selection.threadID,
	).Scan(&lineageID)
	if lineageID == "" {
		lineageID = "e2e-freeform-refresh-lineage-" + selection.threadID
	}
	entryID := "e2e-freeform-refresh-entry-" + selection.runID
	payload := fmt.Sprintf(
		`{"type":"message","id":%q,"parentId":null,"message":{"role":"assistant","content":%q}}`,
		entryID,
		"Durable freeform refresh response "+marker,
	)
	_, _ = database.ExecContext(t.Context(), `
UPDATE agent_threads
SET lineage_id = COALESCE(NULLIF(lineage_id, ''), ?), head_entry_id = ?
WHERE id = ?`, lineageID, entryID, selection.threadID)
	_, _ = database.ExecContext(t.Context(), `
INSERT INTO chat_sessions (id, workspace_id, created_by_user_email, branch_id, topology_kind, current_projection_seq)
VALUES (?, ?, 'playwright@localhost', ?, 'root', 1)
ON CONFLICT(id) DO NOTHING`, selection.threadID, selection.workspaceID, selection.threadID)
	if _, err := database.ExecContext(t.Context(), `
INSERT OR REPLACE INTO agent_entries (lineage_id, entry_id, parent_entry_id, entry_type, origin_order, payload_json, origin_thread_id, origin_run_id, session_timestamp)
VALUES (?, ?, NULL, 'message', 1, ?, ?, ?, CURRENT_TIMESTAMP)`, lineageID, entryID, payload, selection.threadID, selection.runID); err != nil {
		t.Fatal(err)
	}
	messages := fmt.Sprintf(
		`[{"id":"msg-%s","role":"assistant","content":%q}]`,
		selection.runID,
		"Durable freeform refresh response "+marker,
	)
	participants := `[{"id":"assistant","displayName":"Assistant"}]`
	if _, err := database.ExecContext(t.Context(), `
INSERT OR REPLACE INTO chat_session_projections (session_id, last_seq, messages_json, runs_json, participants_json, artifacts_json, topology_json)
VALUES (?, 1, ?, '[]', ?, '[]', '{}')`, selection.threadID, messages, participants); err != nil {
		t.Fatal(err)
	}
	_, _ = database.ExecContext(t.Context(), `
UPDATE agent_runs SET status = 'complete', completed_at = CURRENT_TIMESTAMP WHERE id = ?`, selection.runID)
}

func SeedLatestWorkspaceChats(t testing.TB, ctx *e2e.Context, markerA, markerB string) {
	t.Helper()
	database, err := e2e.OpenWorkspaceDB(t.Context(), ctx.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	seedWorkspaceChat(t, ctx, database, "A", markerA)
	seedWorkspaceChat(t, ctx, database, "B", markerB)
}

func seedWorkspaceChat(
	t testing.TB,
	ctx *e2e.Context,
	database *sql.DB,
	label string,
	marker string,
) {
	t.Helper()
	stamp := time.Now().UTC().Format("20060102T150405.000000000")
	safeLabel := strings.ToLower(label)
	workspaceID := "e2e-workspace-" + safeLabel + "-" + stamp
	threadID := "e2e-workspace-thread-" + safeLabel + "-" + stamp
	branchID := "e2e-workspace-branch-" + safeLabel + "-" + stamp
	lineageID := "e2e-workspace-lineage-" + safeLabel + "-" + stamp
	entryID := "e2e-workspace-entry-" + safeLabel + "-" + stamp
	rootDocPath := filepath.Join(
		ctx.Config.RepoRoot,
		"thoughts",
		"creative-mode-agent",
		"plans",
		"2026-05-20_23-02-59_vamos-e2e-story-playwright-go",
	)
	if _, err := database.ExecContext(t.Context(), `
INSERT INTO workspaces (id, user_email, title, root_doc_path, workflow_type, source, selected_thread_id, current_session_id, current_branch_id)
VALUES (?, 'playwright@localhost', ?, ?, 'freeform', 'imported', ?, ?, ?)`, workspaceID, "E2E workspace "+label, rootDocPath, threadID, threadID, branchID); err != nil {
		t.Fatalf("insert workspace %s: %v", label, err)
	}
	if _, err := database.ExecContext(t.Context(), `
INSERT INTO agent_threads (id, user_email, workspace_id, title, cwd, lineage_id, head_entry_id)
VALUES (?, 'playwright@localhost', ?, ?, ?, ?, ?)`, threadID, workspaceID, "E2E workspace thread "+label, ctx.Config.RepoRoot, lineageID, entryID); err != nil {
		t.Fatalf("insert workspace thread %s: %v", label, err)
	}
	payload := fmt.Sprintf(
		`{"type":"message","id":%q,"parentId":null,"message":{"role":"assistant","content":%q}}`,
		entryID,
		"Latest workspace chat "+label+" "+marker,
	)
	if _, err := database.ExecContext(t.Context(), `
INSERT INTO agent_entries (lineage_id, entry_id, parent_entry_id, entry_type, origin_order, payload_json, origin_thread_id, session_timestamp)
VALUES (?, ?, NULL, 'message', 1, ?, ?, CURRENT_TIMESTAMP)`, lineageID, entryID, payload, threadID); err != nil {
		t.Fatalf("insert workspace entry %s: %v", label, err)
	}
	if _, err := database.ExecContext(t.Context(), `
INSERT INTO chat_sessions (id, workspace_id, created_by_user_email, branch_id, topology_kind, current_projection_seq)
VALUES (?, ?, 'playwright@localhost', ?, 'root', 1)`, threadID, workspaceID, branchID); err != nil {
		t.Fatalf("insert workspace session %s: %v", label, err)
	}
	messages := fmt.Sprintf(
		`[{"id":"msg-%s","role":"assistant","content":%q}]`,
		safeLabel,
		"Latest workspace chat "+label+" "+marker,
	)
	participants := `[{"id":"assistant","displayName":"Assistant"}]`
	if _, err := database.ExecContext(t.Context(), `
INSERT INTO chat_session_projections (session_id, last_seq, messages_json, runs_json, participants_json, artifacts_json, topology_json)
VALUES (?, 1, ?, '[]', ?, '[]', '{}')`, threadID, messages, participants); err != nil {
		t.Fatalf("insert workspace projection %s: %v", label, err)
	}
	if ctx.Memory == nil {
		ctx.Memory = map[string]string{}
	}
	ctx.Memory["workspace_"+label] = workspaceID
	ctx.Memory["workspace_thread_"+label] = threadID
}

func OpenSeededWorkspaceChat(t testing.TB, ctx *e2e.Context, label string) {
	t.Helper()
	workspaceID := ctx.Memory["workspace_"+label]
	threadID := ctx.Memory["workspace_thread_"+label]
	if workspaceID == "" || threadID == "" {
		t.Fatalf("seeded workspace %s not found", label)
	}
	Visit(
		t,
		ctx,
		thoughtsChatURL(
			planDocPath(
				"thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go",
			),
			workspaceID,
			threadID,
		),
	)
}

func planDocPath(planDir string) string {
	clean := strings.Trim(strings.TrimSpace(planDir), "/")
	clean = strings.TrimPrefix(clean, "thoughts/")
	if path.Ext(clean) != "" {
		return "thoughts/" + clean
	}
	return "thoughts/" + path.Join(clean, "plan.md")
}

func thoughtsChatURL(docPath, workspaceID, threadID string) string {
	clean := strings.Trim(strings.TrimSpace(docPath), "/")
	if idx := strings.Index(clean, "/thoughts/"); idx >= 0 {
		clean = clean[idx+len("/thoughts/"):]
	}
	clean = strings.TrimPrefix(clean, "thoughts/")
	parts := strings.Split(path.Clean(clean), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	values := url.Values{}
	values.Set("context", "chat")
	if workspaceID != "" {
		values.Set("chat_workspace", workspaceID)
	}
	if threadID != "" {
		values.Set("thread", threadID)
	}
	return "/thoughts/" + strings.Join(parts, "/") + "?" + values.Encode()
}

func ReloadChat(t testing.TB, ctx *e2e.Context, _ string) {
	t.Helper()
	if _, err := ctx.Page.Reload(
		playwright.PageReloadOptions{WaitUntil: playwright.WaitUntilStateNetworkidle},
	); err != nil {
		t.Fatal(err)
	}
}

func ReopenCurrentChat(t testing.TB, ctx *e2e.Context, _ string) {
	t.Helper()
	url := ctx.Page.URL()
	Visit(t, ctx, "/")
	if _, err := ctx.Page.Goto(
		url,
		playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateNetworkidle},
	); err != nil {
		t.Fatal(err)
	}
}

func SendPiDocsReviewPrompt(t testing.TB, ctx *e2e.Context, marker, artifact string) {
	t.Helper()
	nonce := fmt.Sprintf("%d", time.Now().UnixNano())
	if ctx.Memory == nil {
		ctx.Memory = map[string]string{}
	}
	ctx.Memory["nonce"] = nonce
	prompt := fmt.Sprintf(
		`Plan: thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/plan.md

You are helping verify the QRSPI implementation workspace. Focus on how tests and docs may need updates after the new plan code lands.

Run marker: %s
Run nonce: %s

Update only this file:
%s

Do not edit source code, generated tests, design.md, outline.md, plan.md, or docs. Only update the artifact file above.

Ensure the file contains:
# E2E Pi Plan Docs Review

## Latest E2E Pi Review
Marker: %s
Run nonce: %s

### Potential E2E user story updates
- ...

### Potential test implementation updates
- ...

### Potential docs additions/updates/simplifications
- ...

Also include the marker and nonce in your chat response.`,
		marker,
		nonce,
		artifact,
		marker,
		nonce,
	)
	composer := ctx.Page.Locator("#agent-chat-composer-input, textarea[name='message'], textarea").
		First()
	if err := composer.Fill(prompt); err != nil {
		t.Fatal(err)
	}
	if err := composer.Press("Enter"); err != nil {
		t.Fatal(err)
	}
}

func WaitForChatMarker(t testing.TB, ctx *e2e.Context, marker string) {
	t.Helper()
	if err := ctx.Page.Locator("body").
		Filter(playwright.LocatorFilterOptions{HasText: marker}).
		WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(piResponseTimeoutMS)}); err != nil {
		t.Fatal(err)
	}
}

func ExpectTranscriptContains(t testing.TB, ctx *e2e.Context, text string) {
	t.Helper()
	if err := ctx.Page.Locator("body").
		Filter(playwright.LocatorFilterOptions{HasText: text}).
		WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(transcriptTextTimeoutMS)}); err != nil {
		t.Fatalf("transcript missing %q", text)
	}
	if nonce := ctx.Memory["nonce"]; nonce != "" &&
		text == "VAMOS_E2E_PLAN_DOCS_REVIEW_OK" {
		if err := ctx.Page.Locator("#agent-chat-messages").
			Filter(playwright.LocatorFilterOptions{HasText: nonce}).
			WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(piResponseTimeoutMS)}); err != nil {
			t.Fatalf("transcript missing nonce %q", nonce)
		}
	}
}
