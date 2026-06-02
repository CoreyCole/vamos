package vamos

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/playwright-community/playwright-go"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"

	"github.com/CoreyCole/vamos/pkg/e2e/fixtures"
)

const (
	transcriptTextTimeoutMS = 30_000
	piResponseTimeoutMS     = 240_000
	chatPollInterval        = time.Second / 2
	fileChangeTimeout       = 120 * time.Second
	fileChangePollTime      = 500 * time.Millisecond
)

func FollowFirstSidebarDocumentLink() spec.Step {
	return customStep("follow first sidebar document link", func(t testing.TB, ctx *duiruntime.Context) {
		ensureRegionReachable(t, ctx, Thoughts.Sidebar())
		followFirstLink(t, ctx, "#doc-workbench-sidebar-region a[href*='/thoughts/'][href*='.md'], #thoughts-shared-sidebar a[href*='/thoughts/'][href*='.md'], #thoughts-workbench-sidebar a[href*='/thoughts/'][href*='.md'], [data-e2e='thoughts.workbench.sidebar'] a[href*='/thoughts/'][href*='.md']")
	})
}

func FollowFirstBreadcrumbLink() spec.Step {
	return customStep("follow first breadcrumb link", func(t testing.TB, ctx *duiruntime.Context) {
		followFirstLink(t, ctx, "nav[aria-label='Breadcrumb'] a[href], [data-slot='breadcrumb'] a[href], header a[href*='/thoughts/']")
	})
}

func OpenPlanWorkspace(planDir string) spec.Step {
	return customStep("open plan workspace "+planDir, func(t testing.TB, ctx *duiruntime.Context) {
		values := url.Values{}
		values.Set("workflow_type", "qrspi")
		values.Set("plan_dir", planDir)
		visit(t, ctx, "/agent-chat/plan-workspace?"+values.Encode())
		workspaceID, _ := latestPlanWorkspace(t, ctx, planDir)
		threadID := createE2EWorkspaceThread(t, ctx, workspaceID)
		visit(t, ctx, thoughtsChatURL(planDocPath(planDir), workspaceID, threadID))
	})
}

func OpenWorkspaceChat(label string) spec.Step {
	return customStep("open workspace chat "+label, func(t testing.TB, ctx *duiruntime.Context) {
		if err := resolveLocator(t, ctx, AgentChat.Composer()).First().WaitFor(); err != nil {
			t.Fatal(err)
		}
	})
}

func OpenFreeformChatFixture(name string) spec.Step {
	return customStep("open freeform chat fixture "+name, func(t testing.TB, ctx *duiruntime.Context) {
		ensureE2EPlanFixture(t, ctx)
		state := fixtureState(t, ctx, name)
		threadID := fixtureString(t, state, "thread_id")
		workspaceID := fixtureString(t, state, "workspace_id")
		rootDocPath := fixtureString(t, state, "root_doc_path")
		persistE2EFreeformSelection(t, ctx, workspaceID, threadID)
		visit(t, ctx, thoughtsChatURL(rootDocPath, workspaceID, threadID))
	})
}

func OpenThoughtsRootChat(label string) spec.Step {
	return customStep("open thoughts root chat "+label, func(t testing.TB, ctx *duiruntime.Context) {
		ensureE2EPlanFixture(t, ctx)
		clearPlaywrightChatSelection(t, ctx)
		visit(t, ctx, "/thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/plan.md?context=chat")
		if err := resolveLocator(t, ctx, AgentChat.Composer()).First().WaitFor(); err != nil {
			t.Fatal(err)
		}
	})
}

func OpenThoughtsRootChatContext(label string) spec.Step {
	return customStep("open thoughts root chat context "+label, func(t testing.TB, ctx *duiruntime.Context) {
		visit(t, ctx, "/thoughts/?context=chat")
	})
}

func SendFreeformChatPrompt(marker string) spec.Step {
	return customStep("send freeform chat prompt "+marker, func(t testing.TB, ctx *duiruntime.Context) {
		ensureMemory(ctx)
		if selection, _, err := latestFreeformSelection(t, ctx); err == nil {
			ctx.Memory["previous_freeform_run_id"] = selection.runID
		}
		ctx.Memory["last_freeform_marker"] = marker
		startSeededFreeformResumeRun(t, ctx, marker)
	})
}

func WaitForLatestFreeformChatRunCompletion(label string) spec.Step {
	return customStep("wait for latest freeform chat run completion "+label, func(t testing.TB, ctx *duiruntime.Context) {
		selection := waitForLatestChatSelection(t, ctx, false)
		ensureMemory(ctx)
		ctx.Memory["freeform_workspace_id"] = selection.workspaceID
		ctx.Memory["freeform_thread_id"] = selection.threadID
		ctx.Memory["freeform_run_id"] = selection.runID
		seedFreeformTranscriptForSelection(t, ctx, selection, ctx.Memory["last_freeform_marker"])
	})
}

func SeedProjectPlanWorkspaces(projectA, projectB string) spec.Step {
	return customStep("seed project plan workspaces", func(t testing.TB, ctx *duiruntime.Context) {
		ensureE2EPlanFixture(t, ctx)
		database := openDB(t, ctx)
		defer database.Close()
		for _, projectID := range []string{projectA, projectB} {
			seedProjectPlanWorkspace(t, ctx, database, projectID)
		}
	})
}

func SeedLatestFreeformChatQRSPIProjectResult(projectID string) spec.Step {
	return customStep("seed latest freeform chat qrspi project result "+projectID, func(t testing.TB, ctx *duiruntime.Context) {
		ensureE2EPlanFixture(t, ctx)
		database := openDB(t, ctx)
		defer database.Close()
		stamp := time.Now().UTC().Format("20060102T150405.000000000")
		workspaceID := "e2e-freeform-project-ws-" + stamp
		threadID := "e2e-freeform-project-thread-" + stamp
		branchID := "e2e-freeform-project-branch-" + stamp
		lineageID := "e2e-freeform-project-lineage-" + stamp
		entryID := "e2e-freeform-project-entry-" + stamp
		rootDocPath := filepath.Join(thoughtsRoot(ctx), "creative-mode-agent", "plans", "2026-05-20_23-02-59_vamos-e2e-story-playwright-go")
		execSQL(t, database, `INSERT INTO workspaces (id, user_email, title, root_doc_path, workflow_type, source, selected_thread_id, current_session_id, current_branch_id)
VALUES (?, 'playwright@localhost', 'E2E freeform project adoption', ?, 'freeform', 'imported', ?, ?, ?)`, workspaceID, rootDocPath, threadID, threadID, branchID)
		execSQL(t, database, `INSERT INTO agent_threads (id, user_email, title, cwd, lineage_id, head_entry_id, project_id)
VALUES (?, 'playwright@localhost', 'E2E freeform project thread', ?, ?, ?, ?)`, threadID, ctx.Config.RepoRoot, lineageID, entryID, projectID)
		attachE2EThreadWorkspace(t, database, threadID, workspaceID)
		assistantText := fmt.Sprintf(`E2E freeform QRSPI project result

<qrspi-result>
  <project>%s</project>
  <stage>plan</stage>
  <status>complete</status>
  <outcome>complete</outcome>
  <summary>
    <plan-goal>E2E project filtering.</plan-goal>
    <stage-completed>Seeded freeform project adoption.</stage-completed>
    <key-decisions>Project filters plan sidebar.</key-decisions>
  </summary>
  <artifact>thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/plan.md</artifact>
</qrspi-result>`, projectID)
		payload := fmt.Sprintf(`{"type":"message","id":%q,"parentId":null,"message":{"role":"assistant","content":%q}}`, entryID, assistantText)
		execSQL(t, database, `INSERT INTO agent_entries (lineage_id, entry_id, parent_entry_id, entry_type, origin_order, payload_json, origin_thread_id, session_timestamp)
VALUES (?, ?, NULL, 'message', 1, ?, ?, CURRENT_TIMESTAMP)`, lineageID, entryID, payload, threadID)
		execSQL(t, database, `INSERT INTO chat_sessions (id, workspace_id, created_by_user_email, branch_id, topology_kind, current_projection_seq)
VALUES (?, ?, 'playwright@localhost', ?, 'root', 1)`, threadID, workspaceID, branchID)
		messages := fmt.Sprintf(`[{"id":"msg-%s","role":"assistant","content":%q}]`, stamp, assistantText)
		execSQL(t, database, `INSERT INTO chat_session_projections (session_id, last_seq, messages_json, runs_json, participants_json, artifacts_json, topology_json)
VALUES (?, 1, ?, '[]', '[{"id":"assistant","displayName":"Assistant"}]', '[]', '{}')`, threadID, messages)
		execSQL(t, database, `INSERT INTO user_chat_selections (user_email, scope, scope_id, workspace_id, thread_id)
VALUES ('playwright@localhost', 'freeform', '', ?, ?)
ON CONFLICT(user_email, scope, scope_id) DO UPDATE SET workspace_id = excluded.workspace_id, thread_id = excluded.thread_id, run_id = '', updated_at = CURRENT_TIMESTAMP`, workspaceID, threadID)
		ensureMemory(ctx)
		ctx.Memory["freeform_workspace_id"] = workspaceID
		ctx.Memory["freeform_thread_id"] = threadID
		visit(t, ctx, thoughtsChatURL("thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/plan.md", workspaceID, threadID))
	})
}

func SeedLatestWorkspaceChats(markerA, markerB string) spec.Step {
	return customStep("seed latest workspace chats", func(t testing.TB, ctx *duiruntime.Context) {
		ensureE2EPlanFixture(t, ctx)
		database := openDB(t, ctx)
		defer database.Close()
		seedWorkspaceChat(t, ctx, database, "A", markerA)
		seedWorkspaceChat(t, ctx, database, "B", markerB)
	})
}

func OpenSeededWorkspaceChat(label string) spec.Step {
	return customStep("open seeded workspace chat "+label, func(t testing.TB, ctx *duiruntime.Context) {
		workspaceID := ctx.Memory["workspace_"+label]
		threadID := ctx.Memory["workspace_thread_"+label]
		if workspaceID == "" || threadID == "" {
			t.Fatalf("seeded workspace %s not found", label)
		}
		visit(t, ctx, thoughtsChatURL(planDocPath("thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go"), workspaceID, threadID))
	})
}

func SeedQRSPIContinuationWorkspace() spec.Step {
	return customStep("seed qrspi continuation workspace", func(t testing.TB, ctx *duiruntime.Context) {
		ensureE2EPlanFixture(t, ctx)
		database := openDB(t, ctx)
		defer database.Close()
		seedQRSPIContinuationWorkspace(t, ctx, database)
	})
}

func OpenQRSPIContinuationWorkspaceChat() spec.Step {
	return customStep("open qrspi continuation workspace chat", func(t testing.TB, ctx *duiruntime.Context) {
		workspaceID := ctx.Memory["qrspi_continuation_workspace_id"]
		threadID := ctx.Memory["qrspi_continuation_thread_id"]
		rootDocPath := ctx.Memory["qrspi_continuation_root_doc_path"]
		if workspaceID == "" || threadID == "" || rootDocPath == "" {
			t.Fatal("qrspi continuation workspace not seeded")
		}
		visit(t, ctx, thoughtsChatURL(filepath.Join(rootDocPath, "plan.md"), workspaceID, threadID))
	})
}

func SetFirstTranscriptMessageHash() spec.Step {
	return customStep("set first transcript message hash", func(t testing.TB, ctx *duiruntime.Context) {
		id, err := ctx.Page.Locator("#agent-chat-messages [id^='msg-']").First().GetAttribute("id")
		if err != nil || id == "" {
			t.Fatalf("first transcript message has no id: %v", err)
		}
		ensureMemory(ctx)
		ctx.Memory["first_transcript_hash"] = "#" + id
		if _, err := ctx.Page.Evaluate("hash => { window.location.hash = hash }", "#"+id); err != nil {
			t.Fatalf("set transcript hash: %v", err)
		}
	})
}

func WorkflowCardShowsNextSteps() expectation {
	return expectation{customStep("workflow card shows next steps", func(t testing.TB, ctx *duiruntime.Context) {
		if err := ctx.Page.Locator("article").Filter(playwright.LocatorFilterOptions{HasText: "XML next steps"}).WaitFor(); err != nil {
			t.Fatalf("workflow card next steps missing: %v", err)
		}
	})}
}

func WorkflowCardShowsAgentProgress() expectation {
	return expectation{customStep("workflow card shows agent progress", func(t testing.TB, ctx *duiruntime.Context) {
		if err := ctx.Page.Locator("article").Filter(playwright.LocatorFilterOptions{HasText: "Progress"}).WaitFor(); err != nil {
			t.Fatalf("workflow card progress missing: %v", err)
		}
	})}
}

func WorkflowCardHasJumpCurrent() expectation {
	return expectation{customStep("workflow card has jump current", func(t testing.TB, ctx *duiruntime.Context) {
		if err := ctx.Page.Locator("a").Filter(playwright.LocatorFilterOptions{HasText: "Jump to current agent position"}).WaitFor(); err != nil {
			t.Fatalf("jump current link missing: %v", err)
		}
	})}
}

func WorkflowCardHasJumpNextEnd() expectation {
	return expectation{customStep("workflow card has jump next end", func(t testing.TB, ctx *duiruntime.Context) {
		if err := ctx.Page.Locator("a").Filter(playwright.LocatorFilterOptions{HasText: "Jump to end of next step"}).WaitFor(); err != nil {
			t.Fatalf("jump next end link missing: %v", err)
		}
	})}
}

func ExpectHashAnchorPreserved() expectation {
	return expectation{customStep("hash anchor preserved", func(t testing.TB, ctx *duiruntime.Context) {
		want := ctx.Memory["first_transcript_hash"]
		if want == "" {
			t.Fatal("first transcript hash not remembered")
		}
		got, err := ctx.Page.Evaluate("() => window.location.hash")
		if err != nil {
			t.Fatalf("read location hash: %v", err)
		}
		if fmt.Sprint(got) != want {
			t.Fatalf("hash = %v, want %s", got, want)
		}
	})}
}

func MobileTranscriptHasNoHorizontalOverflow() expectation {
	return expectation{customStep("mobile transcript has no horizontal overflow", func(t testing.TB, ctx *duiruntime.Context) {
		result, err := ctx.Page.Locator("#agent-chat-scroll-region").First().Evaluate("el => el.scrollWidth <= el.clientWidth + 1", nil)
		if err != nil {
			t.Fatalf("measure transcript overflow: %v", err)
		}
		if result != true {
			t.Fatalf("transcript has horizontal overflow: %v", result)
		}
	})}
}

func ToolWriteEditRendered() expectation {
	return expectation{customStep("tool write edit rendered", func(t testing.TB, ctx *duiruntime.Context) {
		for _, text := range []string{"bash", "file write", "file edit", "e2e/created.txt", "e2e/updated.txt"} {
			if err := ctx.Page.Locator("body").Filter(playwright.LocatorFilterOptions{HasText: text}).WaitFor(); err != nil {
				t.Fatalf("missing rendered tool/file text %q: %v", text, err)
			}
		}
	})}
}

func OpenWorkspaceDocumentWithoutChatParams(label string) spec.Step {
	return customStep("open workspace document without chat params "+label, func(t testing.TB, ctx *duiruntime.Context) {
		workspaceID := ctx.Memory["workspace_"+label]
		threadID := ctx.Memory["workspace_thread_"+label]
		rootDocPath := ctx.Memory["workspace_root_"+label]
		if workspaceID == "" || threadID == "" || rootDocPath == "" {
			t.Fatalf("seeded workspace %s not found", label)
		}
		database := openDB(t, ctx)
		defer database.Close()
		execSQL(t, database, `UPDATE workspaces SET selected_thread_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, threadID, workspaceID)
		visit(t, ctx, thoughtsChatURL(filepath.Join(rootDocPath, "plan.md"), "", ""))
	})
}

func ExpectThreadMetadataProject(projectID string) expectation {
	return expectation{customStep("thread metadata project "+projectID, func(t testing.TB, ctx *duiruntime.Context) {
		openThreadMetadata(t, ctx)
		content := ctx.Page.Locator("[data-slot='dropdown-menu-content'][data-show='$agent_chat_composer_metadata.open']").First()
		if err := content.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
			t.Fatalf("thread metadata menu did not become visible: %v", err)
		}
		if err := content.GetByText(projectID).First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
			t.Fatalf("thread metadata project %q not visible: %v", projectID, err)
		}
	})}
}

func ExpectThreadMetadataMenuVisible() expectation {
	return expectation{customStep("thread metadata menu visible", func(t testing.TB, ctx *duiruntime.Context) {
		openThreadMetadata(t, ctx)
		content := ctx.Page.Locator("[data-slot='dropdown-menu-content'][data-show='$agent_chat_composer_metadata.open']").First()
		if err := content.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
			t.Fatalf("thread metadata menu did not become visible: %v", err)
		}
		if err := content.GetByText("Chat metadata").First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
			t.Fatalf("thread metadata label not visible: %v", err)
		}
	})}
}

func ExpectAvatarMenuVisible() expectation {
	return expectation{customStep("avatar menu visible", func(t testing.TB, ctx *duiruntime.Context) {
		trigger := ctx.Page.Locator("header [data-slot='dropdown-menu-trigger'][data-on\\:click*='user_profile']").First()
		if err := trigger.Click(); err != nil {
			t.Fatal(err)
		}
		content := ctx.Page.Locator("[data-slot='dropdown-menu-content'][data-show='$user_profile.open']").First()
		if err := content.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
			t.Fatalf("avatar menu did not become visible: %v", err)
		}
		if err := content.GetByText("Log out").First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
			t.Fatalf("avatar menu log out item not visible: %v", err)
		}
	})}
}

func openThreadMetadata(t testing.TB, ctx *duiruntime.Context) {
	t.Helper()
	button := ctx.Page.Locator("button[title='Show chat metadata'], button[aria-label='Show chat metadata']").First()
	if err := button.Click(); err != nil {
		t.Fatal(err)
	}
}

func SwitchTab(key string) spec.Step {
	return customStep("switch tab "+key, func(t testing.TB, ctx *duiruntime.Context) {
		if err := resolveLocator(t, ctx, locatorForKey(key)).First().Click(); err != nil {
			t.Fatal(err)
		}
	})
}

func ToggleRegion(key string) spec.Step {
	return customStep("toggle region "+key, func(t testing.TB, ctx *duiruntime.Context) {
		region := resolveLocator(t, ctx, locatorForKey(key)).First()
		button := region.Locator("button[aria-expanded], button[data-workbench-save-on-click], button").First()
		if err := button.Click(); err != nil {
			t.Fatal(err)
		}
	})
}

func ExpectInactiveTabPanelsHidden() expectation {
	return expectation{customStep("inactive tab panels hidden", func(t testing.TB, ctx *duiruntime.Context) {
		visible, err := ctx.Page.Locator("[data-show][class*='hidden']").First().IsVisible()
		if err != nil {
			return
		}
		if visible {
			t.Fatal("inactive tab panel with hidden class is visible")
		}
	})}
}

func ExpectConsoleClean() expectation { return Console.Clean() }

func RememberFileHash(p string) spec.Step {
	return customStep("remember file hash "+p, func(t testing.TB, ctx *duiruntime.Context) {
		resolved := resolveRepoPath(ctx, p)
		remembered, err := fileHash(resolved)
		if err != nil {
			if !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
				t.Fatal(err)
			}
			seed := "# E2E Pi Plan Docs Review\n\nSeed content before Pi verification.\n"
			if err := os.WriteFile(resolved, []byte(seed), 0o644); err != nil {
				t.Fatal(err)
			}
			remembered, err = fileHash(resolved)
			if err != nil {
				t.Fatal(err)
			}
		}
		ensureMemory(ctx)
		ctx.Memory["file_hash:"+p] = remembered
		if status, err := changedRepoPaths(ctx.Config.RepoRoot); err == nil {
			ctx.Memory["repo_changed_paths_before"] = strings.Join(status, "\n")
		}
	})
}

func SendPiDocsReviewPrompt(marker, outputPath string) spec.Step {
	return customStep("send pi docs review prompt "+marker, func(t testing.TB, ctx *duiruntime.Context) {
		nonce := fmt.Sprintf("%d", time.Now().UnixNano())
		ensureMemory(ctx)
		ctx.Memory["nonce"] = nonce
		prompt := fmt.Sprintf(`Plan: thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/plan.md

You are helping verify the QRSPI implementation workspace. Focus on how tests and docs may need updates after the new plan code lands.

Run marker: %s
Run nonce: %s

Update only this file:
%s

Do not edit source code, Go Story tests, design.md, outline.md, plan.md, or docs. Only update the artifact file above.

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

Also include the marker and nonce in your chat response.`, marker, nonce, outputPath, marker, nonce)
		composer := resolveLocator(t, ctx, AgentChat.Composer()).First()
		if err := composer.Fill(prompt); err != nil {
			t.Fatal(err)
		}
		if err := composer.Press("Enter"); err != nil {
			t.Fatal(err)
		}
	})
}

func WaitForChatMarker(marker string) expectation {
	return expectation{customStep("wait for chat marker "+marker, func(t testing.TB, ctx *duiruntime.Context) {
		if err := ctx.Page.Locator("body").Filter(playwright.LocatorFilterOptions{HasText: marker}).WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(piResponseTimeoutMS)}); err != nil {
			t.Fatal(err)
		}
	})}
}

func TranscriptContains(text string) expectation {
	return expectation{customStep("transcript contains "+text, func(t testing.TB, ctx *duiruntime.Context) {
		if err := ctx.Page.Locator("body").Filter(playwright.LocatorFilterOptions{HasText: text}).WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(transcriptTextTimeoutMS)}); err != nil {
			t.Fatalf("transcript missing %q", text)
		}
		if nonce := ctx.Memory["nonce"]; nonce != "" && text == "VAMOS_E2E_PLAN_DOCS_REVIEW_OK" {
			if err := ctx.Page.Locator("#agent-chat-messages").Filter(playwright.LocatorFilterOptions{HasText: nonce}).WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(piResponseTimeoutMS)}); err != nil {
				t.Fatalf("transcript missing nonce %q", nonce)
			}
		}
	})}
}

func ReloadChat() spec.Step {
	return customStep("reload chat", func(t testing.TB, ctx *duiruntime.Context) {
		if _, err := ctx.Page.Reload(playwright.PageReloadOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded}); err != nil {
			t.Fatal(err)
		}
	})
}

func ReopenCurrentChat() spec.Step {
	return customStep("reopen current chat", func(t testing.TB, ctx *duiruntime.Context) {
		currentURL := ctx.Page.URL()
		visit(t, ctx, "/")
		if _, err := ctx.Page.Goto(currentURL, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded}); err != nil {
			t.Fatal(err)
		}
	})
}

func ExpectFileHashChanged(p string) expectation {
	return expectation{customStep("file hash changed "+p, func(t testing.TB, ctx *duiruntime.Context) {
		remembered := ctx.Memory["file_hash:"+p]
		deadline := time.Now().Add(fileChangeTimeout)
		for {
			got, err := fileHash(resolveRepoPath(ctx, p))
			if err != nil {
				t.Fatal(err)
			}
			if remembered != got {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("file %s hash did not change", p)
			}
			time.Sleep(fileChangePollTime)
		}
	})}
}

func ExpectPiReviewFileSections(p string) expectation {
	return expectation{customStep("pi review file sections "+p, func(t testing.TB, ctx *duiruntime.Context) {
		data, err := os.ReadFile(resolveRepoPath(ctx, p))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, section := range []string{"# E2E Pi Plan Docs Review", "## Latest E2E Pi Review", "### Potential E2E user story updates", "### Potential test implementation updates", "### Potential docs additions/updates/simplifications"} {
			if !strings.Contains(text, section) {
				t.Fatalf("file %s missing section %q", p, section)
			}
		}
		for _, section := range []string{"### Potential E2E user story updates", "### Potential test implementation updates", "### Potential docs additions/updates/simplifications"} {
			if !sectionHasBullet(text, section) {
				t.Fatalf("file %s section %q has no bullet", p, section)
			}
		}
	})}
}

func ExpectOnlyFileChanged(p string) expectation {
	return expectation{customStep("only file changed "+p, func(t testing.TB, ctx *duiruntime.Context) {
		if _, err := os.Stat(resolveRepoPath(ctx, p)); err != nil {
			t.Fatal(err)
		}
		before := splitPathSet(ctx.Memory["repo_changed_paths_before"])
		after, err := changedRepoPaths(ctx.Config.RepoRoot)
		if err != nil {
			t.Fatal(err)
		}
		allowed := filepath.ToSlash(strings.TrimPrefix(resolveRepoPath(ctx, p), ctx.Config.RepoRoot+string(filepath.Separator)))
		for _, changed := range after {
			if changed == allowed || before[changed] {
				continue
			}
			t.Fatalf("unexpected changed file %s; only %s may change", changed, allowed)
		}
	})}
}

func customStep(label string, fn func(testing.TB, *duiruntime.Context)) spec.Step {
	return spec.Custom(label, func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		fn(t, ctx)
	})
}

func visit(t testing.TB, ctx *duiruntime.Context, p string) {
	t.Helper()
	_, err := ctx.Page.Goto(ctx.Config.BaseURL+p, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded})
	if err != nil {
		t.Fatal(err)
	}
}

func resolveLocator(t testing.TB, ctx *duiruntime.Context, locator spec.Locator) playwright.Locator {
	t.Helper()
	resolved, err := locator.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func ensureRegionReachable(t testing.TB, ctx *duiruntime.Context, locator spec.Locator) {
	t.Helper()
	region := resolveLocator(t, ctx, locator).First()
	if err := region.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateAttached}); err != nil {
		t.Fatal(err)
	}
	visible, err := region.IsVisible()
	if err != nil {
		t.Fatal(err)
	}
	if visible {
		return
	}
	id, err := region.GetAttribute("id")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("region is hidden and has no id for reachability toggle")
	}
	signal, _ := region.GetAttribute("data-workbench-signal")
	toggleSelector := "button[aria-controls='" + id + "'], [role='tab'][aria-controls='" + id + "']"
	if signal != "" {
		toggleSelector += ", button[data-on\\:click*='" + signal + ".visible']"
	}
	visibleOnly := true
	toggle := ctx.Page.Locator(toggleSelector).Filter(playwright.LocatorFilterOptions{Visible: &visibleOnly}).First()
	if err := toggle.Click(); err != nil {
		t.Fatalf("hidden region cannot be reached through %s toggle: %v", id, err)
	}
	if err := region.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("region did not become visible after %s toggle: %v", id, err)
	}
}

func followFirstLink(t testing.TB, ctx *duiruntime.Context, selector string) {
	t.Helper()
	link := ctx.Page.Locator(selector).First()
	if err := link.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateAttached}); err != nil {
		t.Fatal(err)
	}
	href, err := link.GetAttribute("href")
	if err != nil || href == "" {
		t.Fatalf("first link %q has no href: %v", selector, err)
	}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		_, err = ctx.Page.Goto(href, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded})
	} else {
		_, err = ctx.Page.Goto(ctx.Config.BaseURL+href, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded})
	}
	if err != nil {
		t.Fatal(err)
	}
}

func latestPlanWorkspace(t testing.TB, ctx *duiruntime.Context, planDir string) (string, string) {
	t.Helper()
	database := openDB(t, ctx)
	defer database.Close()
	rootDocPath := filepath.Join(ctx.Config.RepoRoot, strings.TrimPrefix(planDir, "thoughts/"))
	if strings.HasPrefix(planDir, "thoughts/") {
		rootDocPath = filepath.Join(ctx.Config.RepoRoot, planDir)
	}
	rootDocPath = filepath.Clean(rootDocPath)
	var workspaceID, threadID string
	if err := database.QueryRowContext(t.Context(), `SELECT id, selected_thread_id FROM workspaces WHERE root_doc_path = ? AND workflow_type = 'qrspi' AND archived_at IS NULL ORDER BY updated_at DESC LIMIT 1`, rootDocPath).Scan(&workspaceID, &threadID); err != nil {
		t.Fatal(err)
	}
	if workspaceID == "" || threadID == "" {
		t.Fatalf("plan workspace %s missing workspace or thread", rootDocPath)
	}
	return workspaceID, threadID
}

func createE2EWorkspaceThread(t testing.TB, ctx *duiruntime.Context, workspaceID string) string {
	t.Helper()
	database := openDB(t, ctx)
	defer database.Close()
	threadID := uuid.NewString()
	execSQL(t, database, `INSERT INTO agent_threads (id, user_email, title, cwd, lineage_id) VALUES (?, 'playwright@localhost', ?, ?, ?)`, threadID, "E2E Pi docs review", filepath.Clean(ctx.Config.RepoRoot), threadID)
	attachE2EThreadWorkspace(t, database, threadID, workspaceID)
	execSQL(t, database, `UPDATE workspaces SET selected_thread_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, threadID, workspaceID)
	return threadID
}

func attachE2EThreadWorkspace(t testing.TB, database *sql.DB, threadID, workspaceID string) {
	t.Helper()
	execSQL(t, database, `INSERT INTO agent_thread_workspaces (thread_id, workspace_id, is_primary, role, adopted_from, adopted_at)
VALUES (?, ?, 1, 'primary', 'e2e_fixture', CURRENT_TIMESTAMP)
ON CONFLICT(thread_id, workspace_id) DO UPDATE SET is_primary = 1, role = 'primary', adopted_at = CURRENT_TIMESTAMP`, threadID, workspaceID)
}

func fixtureState(t testing.TB, ctx *duiruntime.Context, name string) fixtures.State {
	t.Helper()
	state, ok := fixtureFromMemory(ctx).(fixtures.State)
	if ok && state.Name == name {
		return state
	}
	WorkspaceFixture(name).Build().Run(t, ctx)
	state, ok = fixtureFromMemory(ctx).(fixtures.State)
	if !ok || state.Name != name {
		t.Fatalf("fixture %s did not load", name)
	}
	return state
}

func fixtureString(t testing.TB, state fixtures.State, key string) string {
	t.Helper()
	value, _ := state.Data[key].(string)
	if value == "" {
		t.Fatalf("fixture %s did not return %s", state.Name, key)
	}
	return value
}

func persistE2EFreeformSelection(t testing.TB, ctx *duiruntime.Context, workspaceID, threadID string) {
	t.Helper()
	database := openDB(t, ctx)
	defer database.Close()
	execSQL(t, database, `INSERT INTO user_chat_selections (user_email, scope, scope_id, workspace_id, thread_id)
VALUES ('playwright@localhost', 'freeform', '', ?, ?)
ON CONFLICT(user_email, scope, scope_id) DO UPDATE SET workspace_id = excluded.workspace_id, thread_id = excluded.thread_id, updated_at = CURRENT_TIMESTAMP`, workspaceID, threadID)
}

func clearPlaywrightChatSelection(t testing.TB, ctx *duiruntime.Context) {
	t.Helper()
	database := openDB(t, ctx)
	defer database.Close()
	execSQL(t, database, `DELETE FROM user_chat_selections WHERE user_email IN ('playwright@localhost', 'playwright@chestnutfi.com')`)
}

func startSeededFreeformResumeRun(t testing.TB, ctx *duiruntime.Context, marker string) {
	t.Helper()
	database := openDB(t, ctx)
	defer database.Close()
	runID := "e2e-freeform-resume-run-" + time.Now().UTC().Format("20060102T150405.000000000")
	workspaceID := ctx.Memory["freeform_workspace_id"]
	threadID := ctx.Memory["freeform_thread_id"]
	if workspaceID == "" || threadID == "" {
		stamp := time.Now().UTC().Format("20060102T150405.000000000")
		workspaceID = "e2e-freeform-refresh-ws-" + stamp
		threadID = "e2e-freeform-refresh-thread-" + stamp
		branchID := "e2e-freeform-refresh-branch-" + stamp
		lineageID := "e2e-freeform-refresh-lineage-" + stamp
		rootDocPath := filepath.Join(thoughtsRoot(ctx), "creative-mode-agent", "plans", "2026-05-20_23-02-59_vamos-e2e-story-playwright-go")
		execSQL(t, database, `INSERT INTO workspaces (id, user_email, title, root_doc_path, workflow_type, source, selected_thread_id, current_session_id, current_branch_id) VALUES (?, 'playwright@localhost', 'E2E freeform refresh chat', ?, 'freeform', 'imported', ?, ?, ?)`, workspaceID, rootDocPath, threadID, threadID, branchID)
		execSQL(t, database, `INSERT INTO agent_threads (id, user_email, title, cwd, lineage_id) VALUES (?, 'playwright@localhost', 'E2E freeform refresh thread', ?, ?)`, threadID, ctx.Config.RepoRoot, lineageID)
		attachE2EThreadWorkspace(t, database, threadID, workspaceID)
		ctx.Memory["freeform_workspace_id"] = workspaceID
		ctx.Memory["freeform_thread_id"] = threadID
	}
	execSQL(t, database, `INSERT INTO agent_runs (id, workspace_id, thread_id, trigger, status, prompt_text, workflow_id, root_doc_path) VALUES (?, ?, ?, 'resume', 'running', ?, ?, ?)`, runID, workspaceID, threadID, "E2E durable freeform refresh check. Reply with marker "+marker, "e2e-freeform-refresh", ctx.Config.RepoRoot)
	execSQL(t, database, `INSERT INTO user_chat_selections (user_email, scope, scope_id, workspace_id, thread_id, run_id)
VALUES ('playwright@localhost', 'freeform', '', ?, ?, ?)
ON CONFLICT(user_email, scope, scope_id) DO UPDATE SET workspace_id = excluded.workspace_id, thread_id = excluded.thread_id, run_id = excluded.run_id, updated_at = CURRENT_TIMESTAMP`, workspaceID, threadID, runID)
	visit(t, ctx, thoughtsChatURL("thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/plan.md", workspaceID, threadID))
}

type latestChatSelection struct{ workspaceID, threadID, runID string }

func waitForLatestChatSelection(t testing.TB, ctx *duiruntime.Context, complete bool) latestChatSelection {
	t.Helper()
	deadline := time.Now().Add(piResponseTimeoutMS * time.Millisecond)
	var lastErr error
	for time.Now().Before(deadline) {
		selection, status, err := latestFreeformSelection(t, ctx)
		if err == nil && selection.workspaceID != "" && selection.threadID != "" && selection.runID != "" && selection.runID != ctx.Memory["previous_freeform_run_id"] {
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

func latestFreeformSelection(t testing.TB, ctx *duiruntime.Context) (latestChatSelection, string, error) {
	t.Helper()
	database, err := OpenWorkspaceDB(t.Context(), ctx.Config)
	if err != nil {
		return latestChatSelection{}, "", err
	}
	defer database.Close()
	var selection latestChatSelection
	var status string
	err = database.QueryRowContext(t.Context(), `SELECT s.workspace_id, s.thread_id, s.run_id, r.status FROM user_chat_selections s JOIN workspaces w ON w.id = s.workspace_id LEFT JOIN agent_runs r ON r.id = s.run_id WHERE w.workflow_type = 'freeform' ORDER BY s.updated_at DESC LIMIT 1`).Scan(&selection.workspaceID, &selection.threadID, &selection.runID, &status)
	return selection, status, err
}

func seedFreeformTranscriptForSelection(t testing.TB, ctx *duiruntime.Context, selection latestChatSelection, marker string) {
	t.Helper()
	if marker == "" {
		t.Fatal("freeform marker missing")
	}
	database := openDB(t, ctx)
	defer database.Close()
	lineageID := ""
	_ = database.QueryRowContext(t.Context(), `SELECT lineage_id FROM agent_threads WHERE id = ?`, selection.threadID).Scan(&lineageID)
	if lineageID == "" {
		lineageID = "e2e-freeform-refresh-lineage-" + selection.threadID
	}
	entryID := "e2e-freeform-refresh-entry-" + selection.runID
	payload := fmt.Sprintf(`{"type":"message","id":%q,"parentId":null,"message":{"role":"assistant","content":%q}}`, entryID, "Durable freeform refresh response "+marker)
	execSQL(t, database, `UPDATE agent_threads SET lineage_id = COALESCE(NULLIF(lineage_id, ''), ?), head_entry_id = ? WHERE id = ?`, lineageID, entryID, selection.threadID)
	execSQL(t, database, `INSERT INTO chat_sessions (id, workspace_id, created_by_user_email, branch_id, topology_kind, current_projection_seq) VALUES (?, ?, 'playwright@localhost', ?, 'root', 1) ON CONFLICT(id) DO NOTHING`, selection.threadID, selection.workspaceID, selection.threadID)
	execSQL(t, database, `INSERT OR REPLACE INTO agent_entries (lineage_id, entry_id, parent_entry_id, entry_type, origin_order, payload_json, origin_thread_id, origin_run_id, session_timestamp) VALUES (?, ?, NULL, 'message', 1, ?, ?, ?, CURRENT_TIMESTAMP)`, lineageID, entryID, payload, selection.threadID, selection.runID)
	messages := fmt.Sprintf(`[{"id":"msg-%s","role":"assistant","content":%q}]`, selection.runID, "Durable freeform refresh response "+marker)
	execSQL(t, database, `INSERT OR REPLACE INTO chat_session_projections (session_id, last_seq, messages_json, runs_json, participants_json, artifacts_json, topology_json) VALUES (?, 1, ?, '[]', '[{"id":"assistant","displayName":"Assistant"}]', '[]', '{}')`, selection.threadID, messages)
	execSQL(t, database, `UPDATE agent_runs SET status = 'complete', completed_at = CURRENT_TIMESTAMP WHERE id = ?`, selection.runID)
}

func seedProjectPlanWorkspace(t testing.TB, ctx *duiruntime.Context, database *sql.DB, projectID string) {
	t.Helper()
	stamp := time.Now().UTC().Format("20060102T150405.000000000")
	slug := e2eProjectSlug(projectID)
	rel := path.Join("creative-mode-agent", "plans", "e2e-project-filter-"+slug)
	dir := filepath.Join(thoughtsRoot(ctx), rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	frontmatter := fmt.Sprintf("---\nproject: %s\nstage: plan\n---\n# %s\n", projectID, projectPlanLabel(projectID))
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(frontmatter), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plan.md"), []byte(frontmatter), 0o644); err != nil {
		t.Fatal(err)
	}
	execSQL(t, database, `INSERT INTO plan_workspaces (plan_dir_rel, project_id, plan_dir, label, workspace_slug, artifact_updated_at, qrspi_lifecycle, last_discovered_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, 'plan', CURRENT_TIMESTAMP)
ON CONFLICT(plan_dir_rel) DO UPDATE SET project_id = excluded.project_id, plan_dir = excluded.plan_dir, label = excluded.label, workspace_slug = excluded.workspace_slug, artifact_updated_at = CURRENT_TIMESTAMP, qrspi_lifecycle = excluded.qrspi_lifecycle, archived_at = NULL, last_discovered_at = CURRENT_TIMESTAMP`, rel, projectID, dir, projectPlanLabel(projectID), "e2e-project-filter-"+slug+"-"+stamp)
}

func seedQRSPIContinuationWorkspace(t testing.TB, ctx *duiruntime.Context, database *sql.DB) {
	t.Helper()
	stamp := time.Now().UTC().Format("20060102T150405.000000000")
	workspaceID := "e2e-qrspi-continuation-ws-" + stamp
	threadID := "e2e-qrspi-continuation-thread-" + stamp
	branchID := "e2e-qrspi-continuation-branch-" + stamp
	rootDocPath := filepath.Join(thoughtsRoot(ctx), "creative-mode-agent", "plans", "e2e-qrspi-continuation-"+stamp)
	if err := os.MkdirAll(rootDocPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDocPath, "plan.md"), []byte("# E2E QRSPI Continuation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workflowState := `{"type":"qrspi","version":"v1","current_node_id":"review-plan","status":"idle","policy":{"advanceMode":"guided","enablePlanReviews":true,"invalidResultRetryLimit":1},"executionCwd":"` + ctx.Config.RepoRoot + `","attempts":{"plan":1},"nodes":{"plan":{"status":"complete","attempts":1,"last_run_id":"run-` + stamp + `"},"review-plan":{"status":"pending"}},"last_result":{"source_node_id":"plan","status":"complete","summary":"E2E QRSPI plan complete.","primary_artifact":"thoughts/creative-mode-agent/plans/e2e-qrspi-continuation-` + stamp + `/plan.md","display_next":"Read ~/.agents/skills/qrspi-planning/SKILL.md.\nRead ~/.agents/skills/q-review/SKILL.md.\nRead thoughts/creative-mode-agent/plans/e2e-qrspi-continuation-` + stamp + `/plan.md.\nStart /q-review immediately unless blocked.","outcome":"complete"},"pending_next_node_id":"review-plan"}`
	execSQL(t, database, `INSERT INTO workspaces (id, user_email, title, root_doc_path, workflow_type, source, selected_thread_id, current_session_id, current_branch_id, workflow_state_json)
VALUES (?, 'playwright@localhost', 'E2E QRSPI continuation', ?, 'qrspi', 'imported', ?, ?, ?, ?)`, workspaceID, rootDocPath, threadID, threadID, branchID, workflowState)
	execSQL(t, database, `INSERT INTO agent_threads (id, user_email, title, cwd, lineage_id, head_entry_id) VALUES (?, 'playwright@localhost', 'E2E QRSPI continuation thread', ?, ?, ?)`, threadID, ctx.Config.RepoRoot, threadID, "e2e-qrspi-assistant-"+stamp)
	attachE2EThreadWorkspace(t, database, threadID, workspaceID)
	execSQL(t, database, `INSERT INTO chat_sessions (id, workspace_id, created_by_user_email, branch_id, topology_kind, current_projection_seq) VALUES (?, ?, 'playwright@localhost', ?, 'root', 0)`, threadID, workspaceID, branchID)
	execSQL(t, database, `INSERT INTO chat_session_events (session_id, seq, event_type, actor_participant_id, payload_json) VALUES (?, 1, 'message.completed', 'user', ?)`, threadID, fmt.Sprintf(`{"id":"e2e-prompt-%s","role":"user","content":"VAMOS_E2E_QRSPI_CONTINUATION_PROMPT"}`, stamp))
	execSQL(t, database, `INSERT INTO chat_session_events (session_id, seq, event_type, actor_participant_id, payload_json) VALUES (?, 2, 'message.completed', 'assistant', ?)`, threadID, fmt.Sprintf(`{"id":"e2e-assistant-%s","role":"assistant","content":"QRSPI continuation ready"}`, stamp))
	execSQL(t, database, `INSERT INTO chat_session_events (session_id, seq, event_type, payload_json) VALUES (?, 3, 'tool.completed', ?)`, threadID, fmt.Sprintf(`{"tool_call_id":"tool-%s","tool_name":"bash","summary":"ran continuation smoke check"}`, stamp))
	execSQL(t, database, `INSERT INTO chat_session_events (session_id, seq, event_type, payload_json) VALUES (?, 4, 'file.written', '{"path":"e2e/created.txt"}')`, threadID)
	execSQL(t, database, `INSERT INTO chat_session_events (session_id, seq, event_type, payload_json) VALUES (?, 5, 'file.edited', '{"path":"e2e/updated.txt"}')`, threadID)
	execSQL(t, database, `UPDATE workspaces SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, workspaceID)
	ensureMemory(ctx)
	ctx.Memory["qrspi_continuation_workspace_id"] = workspaceID
	ctx.Memory["qrspi_continuation_thread_id"] = threadID
	ctx.Memory["qrspi_continuation_root_doc_path"] = rootDocPath
}

func seedWorkspaceChat(t testing.TB, ctx *duiruntime.Context, database *sql.DB, label, marker string) {
	t.Helper()
	stamp := time.Now().UTC().Format("20060102T150405.000000000")
	safeLabel := strings.ToLower(label)
	workspaceID := "e2e-workspace-" + safeLabel + "-" + stamp
	threadID := "e2e-workspace-thread-" + safeLabel + "-" + stamp
	branchID := "e2e-workspace-branch-" + safeLabel + "-" + stamp
	lineageID := "e2e-workspace-lineage-" + safeLabel + "-" + stamp
	entryID := "e2e-workspace-entry-" + safeLabel + "-" + stamp
	root := thoughtsRoot(ctx)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	rootDocPath := filepath.Join(root, "creative-mode-agent", "plans", "2026-05-20_23-02-59_vamos-e2e-story-playwright-go", "workspace-"+safeLabel+"-"+stamp)
	if err := os.MkdirAll(rootDocPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDocPath, "plan.md"), []byte("# E2E Workspace "+label+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	execSQL(t, database, `INSERT INTO workspaces (id, user_email, title, root_doc_path, workflow_type, source, selected_thread_id, current_session_id, current_branch_id) VALUES (?, 'playwright@localhost', ?, ?, 'qrspi', 'imported', ?, ?, ?)`, workspaceID, "E2E workspace "+label, rootDocPath, threadID, threadID, branchID)
	execSQL(t, database, `INSERT INTO agent_threads (id, user_email, title, cwd, lineage_id, head_entry_id) VALUES (?, 'playwright@localhost', ?, ?, ?, ?)`, threadID, "E2E workspace thread "+label, ctx.Config.RepoRoot, lineageID, entryID)
	attachE2EThreadWorkspace(t, database, threadID, workspaceID)
	payload := fmt.Sprintf(`{"type":"message","id":%q,"parentId":null,"message":{"role":"assistant","content":%q}}`, entryID, "Latest workspace chat "+label+" "+marker)
	execSQL(t, database, `INSERT INTO agent_entries (lineage_id, entry_id, parent_entry_id, entry_type, origin_order, payload_json, origin_thread_id, session_timestamp) VALUES (?, ?, NULL, 'message', 1, ?, ?, CURRENT_TIMESTAMP)`, lineageID, entryID, payload, threadID)
	execSQL(t, database, `INSERT INTO chat_sessions (id, workspace_id, created_by_user_email, branch_id, topology_kind, current_projection_seq) VALUES (?, ?, 'playwright@localhost', ?, 'root', 1)`, threadID, workspaceID, branchID)
	messages := fmt.Sprintf(`[{"id":"msg-%s","role":"assistant","content":%q}]`, safeLabel, "Latest workspace chat "+label+" "+marker)
	execSQL(t, database, `INSERT INTO chat_session_projections (session_id, last_seq, messages_json, runs_json, participants_json, artifacts_json, topology_json) VALUES (?, 1, ?, '[]', '[{"id":"assistant","displayName":"Assistant"}]', '[]', '{}')`, threadID, messages)
	ensureMemory(ctx)
	ctx.Memory["workspace_"+label] = workspaceID
	ctx.Memory["workspace_thread_"+label] = threadID
	ctx.Memory["workspace_root_"+label] = rootDocPath
}

func ensureE2EPlanFixture(t testing.TB, ctx *duiruntime.Context) {
	t.Helper()
	planPath := filepath.Join(thoughtsRoot(ctx), "creative-mode-agent", "plans", "2026-05-20_23-02-59_vamos-e2e-story-playwright-go", "plan.md")
	if err := os.MkdirAll(filepath.Dir(planPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planPath, []byte("# Vamos E2E Story Fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func projectPlanLabel(projectID string) string {
	return "E2E Project Plan " + strings.TrimSpace(projectID)
}
func e2eProjectSlug(projectID string) string {
	slug := strings.ToLower(strings.TrimSpace(projectID))
	slug = strings.NewReplacer("/", "-", ".", "-", "_", "-", " ", "-").Replace(slug)
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	return strings.Trim(slug, "-")
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
	if threadID != "" {
		values.Set("thread", threadID)
	} else if workspaceID != "" {
		values.Set("chat_workspace", workspaceID)
	}
	return "/thoughts/" + strings.Join(parts, "/") + "?" + values.Encode()
}

func openDB(t testing.TB, ctx *duiruntime.Context) *sql.DB {
	t.Helper()
	database, err := OpenWorkspaceDB(t.Context(), ctx.Config)
	if err != nil {
		t.Fatal(err)
	}
	return database
}

func execSQL(t testing.TB, database *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := database.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func thoughtsRoot(ctx *duiruntime.Context) string {
	if root := strings.TrimSpace(os.Getenv("VAMOS_E2E_THOUGHTS_ROOT")); root != "" {
		return root
	}
	return filepath.Join(ctx.Config.RepoRoot, "thoughts")
}

func ensureMemory(ctx *duiruntime.Context) {
	if ctx.Memory == nil {
		ctx.Memory = map[string]string{}
	}
}

func resolveRepoPath(ctx *duiruntime.Context, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(ctx.Config.RepoRoot, p)
}

func fileHash(p string) (string, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:]), nil
}

func changedRepoPaths(repoRoot string) ([]string, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return nil, nil
	}
	cmd := exec.Command("git", "status", "--porcelain=v1")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status changed files: %w", err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\r\n"), "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		p := strings.TrimSpace(line[3:])
		if renamed := strings.Split(p, " -> "); len(renamed) == 2 {
			p = renamed[1]
		}
		paths = append(paths, filepath.ToSlash(p))
	}
	return paths, nil
}

func splitPathSet(value string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(value, "\n") {
		if p := strings.TrimSpace(line); p != "" {
			out[p] = true
		}
	}
	return out
}

func sectionHasBullet(text, heading string) bool {
	idx := strings.Index(text, heading)
	if idx < 0 {
		return false
	}
	rest := text[idx+len(heading):]
	if next := strings.Index(rest, "\n### "); next >= 0 {
		rest = rest[:next]
	}
	for _, line := range strings.Split(rest, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			return true
		}
	}
	return false
}
