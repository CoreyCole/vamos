package tests

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"
	"github.com/playwright-community/playwright-go"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/pkg/e2e/fixtures"
	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

func TestWorkbenchV2_CompletedDraftSaveReturnsAfterThoughtsBack(t *testing.T) {
	draft := "workbench v2 completed draft"
	spec.Story(t, "workbench v2 completed draft save returns after thoughts back").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Path("/threads/wb2_alpha")).Expect(vamos.WorkbenchV2.Ready()).
		Do(fillAndAwaitDraft(draft)).
		Do(followWorkbenchLink("#workbench-v2-artifact-body a[href^='/thoughts/']")).
		Do(browserBack()).Expect(vamos.WorkbenchV2.Ready()).
		Expect(spec.InputValue(vamos.WorkbenchV2.Composer(), draft)).
		Expect(vamos.Console.Clean()).Run()
}

func TestWorkbenchV2_IncompleteDraftSaveRestoresPriorServerValue(t *testing.T) {
	prior := "completed alpha draft"
	spec.Story(t, "workbench v2 incomplete draft save restores prior server value").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Path("/threads/wb2_alpha")).Expect(vamos.WorkbenchV2.Ready()).
		Do(interruptDraftSaveOffline("uncompleted draft")).
		Do(followWorkbenchLink("#workbench-v2-threads-body a[href='/threads/wb2_beta']")).
		Visit(vamos.Pages.Path("/threads/wb2_alpha")).Expect(vamos.WorkbenchV2.Ready()).
		Expect(spec.InputValue(vamos.WorkbenchV2.Composer(), prior)).Run()
}

func TestWorkbenchV2_SendOutcomesKeepServerDraftTruth(t *testing.T) {
	accepted := "workbench v2 accepted send"
	rejected := "rejected thread draft"
	spec.Story(t, "workbench v2 send outcomes keep server draft truth").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Path("/threads/wb2_alpha")).Expect(vamos.WorkbenchV2.Ready()).
		Do(fillAndAwaitDraft(accepted)).
		Do(submitV2ChatMessage("/thoughts/chat/thread/wb2_alpha/resume", 200)).
		Expect(spec.InputValue(vamos.WorkbenchV2.Composer(), "")).
		Do(reloadPage()).Expect(spec.InputValue(vamos.WorkbenchV2.Composer(), "")).
		Visit(vamos.Pages.Path("/threads/wb2_rejected")).
		Expect(vamos.WorkbenchV2.Ready()).
		Expect(spec.InputValue(vamos.WorkbenchV2.Composer(), rejected)).
		Do(submitV2ChatMessage("/thoughts/chat/thread/wb2_rejected/resume", 409)).
		Expect(spec.InputValue(vamos.WorkbenchV2.Composer(), rejected)).
		Do(reloadPage()).Expect(spec.InputValue(vamos.WorkbenchV2.Composer(), rejected)).
		Do(expectRejectedSendConsoleOnly("/thoughts/chat/thread/wb2_rejected/resume")).
		Run()
}

func TestWorkbenchV2_OlderAdmittedDraftCannotOverwriteAcceptedClear(t *testing.T) {
	old := "delayed browser draft"
	spec.Story(t, "workbench v2 older admitted draft cannot overwrite accepted clear").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Path("/threads/wb2_alpha")).Expect(vamos.WorkbenchV2.Ready()).
		Do(fillAndAwaitDraft(old)).
		Do(submitV2ChatMessage("/thoughts/chat/thread/wb2_alpha/resume", 200)).
		Expect(spec.InputValue(vamos.WorkbenchV2.Composer(), "")).
		Do(upsertOlderDraftAfterAcceptedClear(old)).Do(reloadPage()).
		Expect(spec.InputValue(vamos.WorkbenchV2.Composer(), "")).Run()
}

func TestWorkbenchV2_DraftsAreIsolatedAcrossThreadsAndActors(t *testing.T) {
	spec.Story(t, "workbench v2 drafts are isolated across threads and actors").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Visit(vamos.Pages.Path("/threads/wb2_alpha")).Expect(vamos.WorkbenchV2.Ready()).
		Expect(spec.InputValue(vamos.WorkbenchV2.Composer(), "completed alpha draft")).
		Visit(vamos.Pages.Path("/threads/wb2_beta")).Expect(vamos.WorkbenchV2.Ready()).
		Expect(spec.InputValue(vamos.WorkbenchV2.Composer(), "completed beta draft")).
		Do(assertSecondaryActorDraftIsolationAndSharedSend()).Run()
}

func upsertOlderDraftAfterAcceptedClear(content string) spec.Step {
	return spec.Custom(
		"older admitted draft cannot overwrite accepted clear",
		func(t testing.TB, ctx *duiruntime.Context) {
			actor := os.Getenv("VAMOS_E2E_AUTH_EMAIL")
			if actor == "" {
				actor = "playwright@localhost"
			}
			database, err := vamos.OpenWorkspaceDB(t.Context(), ctx.Config)
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			var clearedOrder int64
			if err := database.QueryRowContext(t.Context(), `SELECT operation_order FROM agent_thread_drafts WHERE user_email = ? AND thread_id = ?`, actor, "wb2_alpha").
				Scan(&clearedOrder); err != nil {
				t.Fatal(err)
			}
			if clearedOrder <= 1 {
				t.Fatalf(
					"accepted clear did not produce a tombstone order: %d",
					clearedOrder,
				)
			}
			queries := db.New(database)
			if err := queries.UpsertAgentThreadDraft(
				t.Context(),
				db.UpsertAgentThreadDraftParams{
					UserEmail:      actor,
					ThreadID:       "wb2_alpha",
					Content:        content,
					OperationOrder: clearedOrder - 1,
				},
			); err != nil {
				t.Fatal(err)
			}
			got, err := queries.GetAgentThreadDraft(
				t.Context(),
				db.GetAgentThreadDraftParams{
					UserEmail: actor,
					ThreadID:  "wb2_alpha",
				},
			)
			if err != nil || got != "" {
				t.Fatalf("older draft replaced accepted clear: %q %v", got, err)
			}
		},
	)
}

func submitV2ChatMessage(path string, wantStatus int) spec.Step {
	return spec.Custom(
		"submit v2 chat message",
		func(t testing.TB, ctx *duiruntime.Context) {
			click := func() error {
				return ctx.Page.Locator("#workbench-v2-chat-body #agent-chat-composer-form").
					GetByRole(*playwright.AriaRoleButton, playwright.LocatorGetByRoleOptions{Name: "Send message"}).
					Click()
			}
			response, err := ctx.Page.ExpectResponse("**"+path+"*", click)
			if err != nil {
				t.Fatalf("chat response not observed: %v", err)
			}
			if response.Status() != wantStatus {
				t.Fatalf(
					"chat submit status = %d, want %d",
					response.Status(),
					wantStatus,
				)
			}
		},
	)
}

func expectRejectedSendConsoleOnly(path string) spec.Step {
	return spec.Custom(
		"expected rejected send is the only console problem",
		func(t testing.TB, ctx *duiruntime.Context) {
			for _, problem := range ctx.Console.Problems() {
				if problem.Type == "error" &&
					strings.Contains(problem.Text, "Failed to load resource") &&
					strings.Contains(problem.URL, path) {
					continue
				}
				t.Fatalf(
					"unexpected console problem:\n%s",
					duiruntime.FormatConsoleProblems([]duiruntime.ConsoleEntry{problem}),
				)
			}
		},
	)
}

func reloadPage() spec.Step {
	return spec.Custom("reload page", func(t testing.TB, ctx *duiruntime.Context) {
		if _, err := ctx.Page.Reload(
			playwright.PageReloadOptions{
				WaitUntil: playwright.WaitUntilStateDomcontentloaded,
			},
		); err != nil {
			t.Fatal(err)
		}
	})
}

func fillDraft(
	value string,
) spec.Step {
	return spec.Fill(vamos.WorkbenchV2.Composer(), value)
}

func fillAndAwaitDraft(value string) spec.Step {
	return spec.Custom("save draft", func(t testing.TB, ctx *duiruntime.Context) {
		if err := ctx.Page.Locator("#workbench-v2-chat-body #agent-chat-composer-input").
			First().
			Fill(value); err != nil {
			t.Fatal(err)
		}
		actor := os.Getenv("VAMOS_E2E_AUTH_EMAIL")
		if actor == "" {
			actor = "playwright@localhost"
		}
		database, err := vamos.OpenWorkspaceDB(t.Context(), ctx.Config)
		if err != nil {
			t.Fatal(err)
		}
		defer database.Close()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			var stored string
			err = database.QueryRowContext(t.Context(), `SELECT content FROM agent_thread_drafts WHERE user_email = ? AND thread_id = ?`, actor, "wb2_alpha").
				Scan(&stored)
			if err == nil && stored == value {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("draft was not persisted as %q before timeout: %v", value, err)
	})
}

func interruptDraftSaveOffline(value string) spec.Step {
	return spec.Custom(
		"interrupt draft save while offline",
		func(t testing.TB, ctx *duiruntime.Context) {
			if err := ctx.Page.Context().SetOffline(true); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := ctx.Page.Context().SetOffline(false); err != nil {
					t.Errorf("restore browser network: %v", err)
				}
			}()
			if err := ctx.Page.Locator("#workbench-v2-chat-body #agent-chat-composer-input").
				First().
				Fill(value); err != nil {
				t.Fatal(err)
			}
			time.Sleep(700 * time.Millisecond)
			if err := ctx.Page.Context().SetOffline(false); err != nil {
				t.Fatal(err)
			}
		},
	)
}

func assertSecondaryActorDraftIsolationAndSharedSend() spec.Step {
	return spec.Custom(
		"secondary actor has an isolated draft and can send to a shared thread",
		func(t testing.TB, ctx *duiruntime.Context) {
			if strings.TrimSpace(ctx.Config.BaseURL) == "" {
				t.Fatal("missing base URL")
			}
			browserContext, err := ctx.Browser.NewContext()
			if err != nil {
				t.Fatal(err)
			}
			defer browserContext.Close()
			page, err := browserContext.NewPage()
			if err != nil {
				t.Fatal(err)
			}
			if err := vamos.AuthenticateSecondary(
				context.Background(),
				page,
				ctx.Config,
			); err != nil {
				t.Fatal(err)
			}
			if _, err := page.Goto(
				ctx.Config.BaseURL+"/threads/wb2_alpha",
				playwright.PageGotoOptions{
					WaitUntil: playwright.WaitUntilStateDomcontentloaded,
				},
			); err != nil {
				t.Fatal(err)
			}
			value, err := page.Locator("#workbench-v2-chat-body #agent-chat-composer-input").
				First().
				InputValue()
			if err != nil {
				t.Fatal(err)
			}
			if value == "completed alpha draft" {
				t.Fatal("secondary actor received primary draft")
			}
			if _, err := page.Goto(
				ctx.Config.BaseURL+"/threads/wb2_beta",
				playwright.PageGotoOptions{
					WaitUntil: playwright.WaitUntilStateDomcontentloaded,
				},
			); err != nil {
				t.Fatal(err)
			}
			composer := page.Locator("#workbench-v2-chat-body #agent-chat-composer-input").
				First()
			if err := composer.Fill("secondary actor shared send"); err != nil {
				t.Fatal(err)
			}
			response, err := page.ExpectResponse(
				"**/thoughts/chat/thread/wb2_beta/resume?*",
				func() error {
					return page.Locator("#workbench-v2-chat-body #agent-chat-composer-form").
						GetByRole(*playwright.AriaRoleButton, playwright.LocatorGetByRoleOptions{Name: "Send message"}).
						Click()
				},
			)
			if err != nil {
				t.Fatalf("secondary actor shared send response not observed: %v", err)
			}
			if response.Status() != http.StatusOK {
				t.Fatalf("secondary actor shared send status = %d", response.Status())
			}
		},
	)
}
