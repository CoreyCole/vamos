package vamos

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"
	"github.com/playwright-community/playwright-go"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/agentchat"
	serverdb "github.com/CoreyCole/vamos/server/services/db"
)

const (
	A2ARoutingFromSlug        = "alpha"
	A2ARoutingToSlug          = "beta"
	A2ARoutingLeadSlug        = "lead"
	A2ARoutingPairwiseBody    = "A2A_ROUTING_PAIRWISE_MAIL"
	A2ARoutingPlanInboundBody = "A2A_ROUTING_PLAN_INBOUND"
	A2ARoutingPlanDir         = "thoughts/owner/plans/alpha"
	A2ARoutingBotHomeRejectTo = "/rooms/dm/beta"
	a2aRoutingPairwiseEntryID = "a2a-routing-pairwise-mail"
	a2aRoutingPlanEntryID     = "a2a-routing-plan-inbound"
	a2aRoutingHomeQuoteID     = "a2a-routing-home-quote-v2"
	a2aRoutingHomeToolID      = "a2a-routing-home-message-room"
	a2aRoutingFromAgentID     = "a2a-route-alpha"
	a2aRoutingToAgentID       = "a2a-route-beta"
	a2aRoutingLeadAgentID     = "a2a-route-lead"
	a2aRoutingFromHomeID      = "a2a-route-home-alpha"
	a2aRoutingToHomeID        = "a2a-route-home-beta"
)

func SeedA2ARoutingContract() spec.Step {
	return customStep(
		"seed A2A routing contract via resolve",
		func(t testing.TB, ctx *duiruntime.Context) {
			t.Helper()
			ws, err := ReadWorkspaceEnv(ctx.Config.RepoRoot)
			if err != nil {
				t.Fatal(err)
			}
			thoughtsRoot := strings.TrimSpace(os.Getenv("VAMOS_E2E_THOUGHTS_ROOT"))
			if thoughtsRoot == "" {
				thoughtsRoot = filepath.Join(ctx.Config.RepoRoot, "thoughts")
			}
			database, err := serverdb.NewService(ws.DBPath)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = database.Close() })
			q := database.Queries
			svc, err := agentchat.NewServiceWithOptions(
				database.DB(),
				q,
				nil,
				nil,
				nil,
				agentchat.ServiceOptions{
					ProjectRoot:  ctx.Config.RepoRoot,
					ThoughtsRoot: thoughtsRoot,
					DefaultCwd:   thoughtsRoot,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			bg := t.Context()
			alpha := mustAgent(t, q, a2aRoutingFromAgentID, A2ARoutingFromSlug, "Alpha")
			beta := mustAgent(t, q, a2aRoutingToAgentID, A2ARoutingToSlug, "Beta")
			lead := mustAgent(t, q, a2aRoutingLeadAgentID, A2ARoutingLeadSlug, "Lead")
			fromHomeID := ensureBotHome(t, q, alpha, a2aRoutingFromHomeID)
			toHomeID := ensureBotHome(t, q, beta, a2aRoutingToHomeID)

			pair, err := svc.ResolveMessageRoom(bg, agentchat.MessageRoomResolveInput{
				To:          A2ARoutingToSlug,
				FromAgentID: alpha.ID,
			})
			if err != nil {
				t.Fatalf("resolve slug to pairwise: %v", err)
			}
			if pair.RoomKind != agentchat.RoomKindPairwise {
				t.Fatalf("slug resolve kind=%q want pairwise", pair.RoomKind)
			}
			seedMessage(
				t,
				q,
				pair.ThreadID,
				a2aRoutingPairwiseEntryID,
				A2ARoutingPairwiseBody,
			)

			plan, err := svc.ResolveMessageRoom(bg, agentchat.MessageRoomResolveInput{
				To:          A2ARoutingPlanDir,
				FromAgentID: alpha.ID,
			})
			if err != nil {
				t.Fatalf("resolve plan dir: %v", err)
			}
			if plan.RoomKind != agentchat.RoomKindPlan {
				t.Fatalf("plan resolve kind=%q want plan", plan.RoomKind)
			}
			_ = q.SetPlanWorkspaceLeadAgent(bg, db.SetPlanWorkspaceLeadAgentParams{
				LeadAgentID: sql.NullString{String: lead.ID, Valid: true},
				PlanDirRel:  strings.TrimPrefix(A2ARoutingPlanDir, "thoughts/"),
			})
			if again, rerr := svc.ResolveMessageRoom(
				bg,
				agentchat.MessageRoomResolveInput{
					To:          A2ARoutingPlanDir,
					FromAgentID: alpha.ID,
				},
			); rerr == nil {
				plan = again
				if strings.TrimSpace(plan.SpeakerAgentID) != "" &&
					plan.SpeakerAgentID != lead.ID {
					t.Fatalf(
						"plan inbound speaker=%q want lead %q",
						plan.SpeakerAgentID,
						lead.ID,
					)
				}
			}
			seedMessage(
				t,
				q,
				plan.ThreadID,
				a2aRoutingPlanEntryID,
				A2ARoutingPlanInboundBody,
			)
			seedHomeQuoteAndMessageRoom(t, q, fromHomeID)

			_, err = svc.ResolveMessageRoom(bg, agentchat.MessageRoomResolveInput{
				To:          A2ARoutingBotHomeRejectTo,
				FromAgentID: alpha.ID,
			})
			if !errors.Is(err, agentchat.ErrMessageRoomBotHome) {
				t.Fatalf("bot-home destination want ErrMessageRoomBotHome, got %v", err)
			}
			home := mustThread(t, q, toHomeID)
			if err := agentchat.GuardEnqueueDestination(home, agentchat.EnqueueMail{
				FromKind:    agentchat.EnqueueFromAgent,
				FromAgentID: alpha.ID,
			}); !errors.Is(err, agentchat.ErrBotHomeRejectsA2A) {
				t.Fatalf("guard bot home A2A: %v", err)
			}

			if ctx.Memory == nil {
				ctx.Memory = map[string]string{}
			}
			ctx.Memory["a2a.pairwise_thread"] = pair.ThreadID
			ctx.Memory["a2a.plan_thread"] = plan.ThreadID
			ctx.Memory["a2a.from_home"] = fromHomeID
			ctx.Memory["a2a.to_home"] = toHomeID
		},
	)
}

func OpenA2ARoutingPairwiseRoom() spec.Step {
	return customStep(
		"open A2A pairwise room",
		func(t testing.TB, ctx *duiruntime.Context) {
			t.Helper()
			visit(
				t,
				ctx,
				Pages.PairwiseA2ARoom(A2ARoutingFromSlug, A2ARoutingToSlug).Path(),
			)
		},
	)
}

func OpenA2ARoutingBotHome(slug string) spec.Step {
	return customStep(
		"open A2A bot home "+slug,
		func(t testing.TB, ctx *duiruntime.Context) {
			t.Helper()
			key := "a2a.from_home"
			if slug == A2ARoutingToSlug {
				key = "a2a.to_home"
			}
			if id := strings.TrimSpace(ctx.Memory[key]); id != "" {
				visit(t, ctx, "/threads/"+id)
				return
			}
			visit(t, ctx, "/rooms/dm/"+slug)
		},
	)
}

func OpenA2ARoutingPlanThread() spec.Step {
	return customStep(
		"open A2A plan inbound thread",
		func(t testing.TB, ctx *duiruntime.Context) {
			t.Helper()
			id := strings.TrimSpace(ctx.Memory["a2a.plan_thread"])
			if id == "" {
				id = DefaultAgentMemoryThreadID
			}
			visit(t, ctx, "/threads/"+id)
		},
	)
}

func ExpectA2APairwiseContainsRoutedBody() expectation {
	return expectation{
		customStep(
			"pairwise room contains routed body and is view-only",
			func(t testing.TB, ctx *duiruntime.Context) {
				t.Helper()
				waitVisibleText(t, ctx, A2ARoutingPairwiseBody)
				count, err := ctx.Page.Locator("#agent-chat-composer-form").Count()
				if err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatalf("pairwise still has composer (count=%d)", count)
				}
			},
		),
	}
}

func ExpectA2AFromHomeChipNotGroupMail() expectation {
	return expectation{
		customStep(
			"from bot home shows chip not group mail",
			func(t testing.TB, ctx *duiruntime.Context) {
				t.Helper()
				waitVisibleText(t, ctx, "routing quote (not pairwise body)")
				assertNoText(t, ctx, A2ARoutingPairwiseBody)
				href := PairwiseA2AHref(A2ARoutingFromSlug, A2ARoutingToSlug)
				chip := ctx.Page.Locator("[id^='bot-dm-chip-']")
				if n, _ := chip.Count(); n > 0 {
					trigger := chip.First().
						Locator("[data-slot='dropdown-menu-trigger'], button, [role='button']").
						First()
					if tn, _ := trigger.Count(); tn > 0 {
						_ = trigger.Click()
					} else {
						_ = chip.First().Click()
					}
				}
				if err := ctx.Page.Locator("a[href='" + href + "']").
					First().
					WaitFor(); err != nil {
					t.Fatalf("missing pairwise href %s: %v", href, err)
				}
			},
		),
	}
}

func ExpectA2AToHomeHasNoRoutedMail() expectation {
	return expectation{
		customStep(
			"to bot home does not show pairwise mail as group text",
			func(t testing.TB, ctx *duiruntime.Context) {
				t.Helper()
				assertNoText(t, ctx, A2ARoutingPairwiseBody)
			},
		),
	}
}

func ExpectA2APlanInbound() expectation {
	return expectation{
		customStep(
			"plan group contains inbound to lead",
			func(t testing.TB, ctx *duiruntime.Context) {
				t.Helper()
				waitVisibleText(t, ctx, A2ARoutingPlanInboundBody)
			},
		),
	}
}

func PairwiseA2AHref(a, b string) string {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if a > b {
		a, b = b, a
	}
	return "/rooms/a2a/" + a + "/" + b
}

func waitVisibleText(t testing.TB, ctx *duiruntime.Context, text string) {
	t.Helper()
	loc := ctx.Page.GetByText(
		text,
		playwright.PageGetByTextOptions{Exact: playwright.Bool(false)},
	)
	if err := loc.First().WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
	}); err != nil {
		t.Fatalf("wait visible text %q: %v", text, err)
	}
}

func assertNoText(t testing.TB, ctx *duiruntime.Context, text string) {
	t.Helper()
	count, err := ctx.Page.GetByText(text, playwright.PageGetByTextOptions{Exact: playwright.Bool(false)}).
		Count()
	if err != nil || count != 0 {
		t.Fatalf("unexpected text %q count=%d err=%v", text, count, err)
	}
}

func mustAgent(t testing.TB, q *db.Queries, id, slug, name string) db.Agent {
	t.Helper()
	bg := t.Context()
	if row, err := q.GetAgentBySlug(bg, slug); err == nil {
		return row
	}
	row, err := q.CreateAgent(bg, db.CreateAgentParams{
		ID: id, Slug: slug, Name: name, Label: name, Description: name,
	})
	if err != nil {
		if row, gerr := q.GetAgentBySlug(bg, slug); gerr == nil {
			return row
		}
		t.Fatalf("create agent %s: %v", slug, err)
	}
	return row
}

func mustThread(t testing.TB, q *db.Queries, id string) db.AgentThread {
	t.Helper()
	row, err := q.GetAgentThread(t.Context(), id)
	if err != nil {
		t.Fatalf("get thread %s: %v", id, err)
	}
	return row
}

func ensureBotHome(t testing.TB, q *db.Queries, agent db.Agent, threadID string) string {
	t.Helper()
	bg := t.Context()
	agentNS := sql.NullString{String: agent.ID, Valid: true}
	if existing, err := q.GetBotHomeThreadByAgentID(bg, agentNS); err == nil {
		return existing.ID
	}
	cwd := filepath.ToSlash(filepath.Join("thoughts", "agents", agent.Slug))
	if _, err := q.GetAgentThread(bg, threadID); err != nil {
		if _, err := q.CreateAgentThread(bg, db.CreateAgentThreadParams{
			ID:        threadID,
			UserEmail: "shared",
			Title:     agent.Name + " home",
			Cwd:       cwd,
			LineageID: threadID,
		}); err != nil {
			t.Fatalf("create bot home %s: %v", agent.Slug, err)
		}
	}
	if err := q.BindAgentThreadBotHome(bg, db.BindAgentThreadBotHomeParams{
		AgentID: agentNS,
		Cwd:     cwd,
		Title:   agent.Name + " home",
		ID:      threadID,
	}); err != nil {
		t.Fatalf("bind bot home %s: %v", agent.Slug, err)
	}
	if existing, err := q.GetBotHomeThreadByAgentID(bg, agentNS); err == nil {
		return existing.ID
	}
	return threadID
}

func seedMessage(t testing.TB, q *db.Queries, threadID, entryID, body string) {
	t.Helper()
	bg := t.Context()
	thread := mustThread(t, q, threadID)
	lineage := strings.TrimSpace(thread.LineageID)
	if lineage == "" {
		lineage = threadID
	}
	payload, err := json.Marshal(map[string]any{
		"type": "message",
		"id":   entryID,
		"message": map[string]any{
			"role":    "assistant",
			"content": body,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CreateAgentEntry(bg, db.CreateAgentEntryParams{
		LineageID:        lineage,
		EntryID:          entryID,
		EntryType:        "message",
		OriginOrder:      90,
		PayloadJson:      string(payload),
		OriginThreadID:   threadID,
		SessionTimestamp: time.Now().UTC(),
	}); err != nil {
		if !strings.Contains(err.Error(), "UNIQUE") &&
			!strings.Contains(err.Error(), "unique") {
			t.Fatalf("seed message %s: %v", entryID, err)
		}
	}
	_ = q.UpdateAgentThreadHead(bg, db.UpdateAgentThreadHeadParams{
		HeadEntryID: sql.NullString{String: entryID, Valid: true},
		ID:          threadID,
	})
}

func seedHomeQuoteAndMessageRoom(t testing.TB, q *db.Queries, homeThreadID string) {
	t.Helper()
	bg := t.Context()
	thread := mustThread(t, q, homeThreadID)
	lineage := strings.TrimSpace(thread.LineageID)
	if lineage == "" {
		lineage = homeThreadID
	}
	payload, err := json.Marshal(map[string]any{
		"type": "message",
		"id":   a2aRoutingHomeQuoteID,
		"message": map[string]any{
			"role": "assistant",
			"name": A2ARoutingFromSlug,
			"content": []any{
				map[string]any{
					"type": "text",
					"text": "routing quote (not pairwise body)\n\n[pairwise](/rooms/a2a/alpha/beta)",
				},
				map[string]any{
					"type": "toolCall",
					"id":   a2aRoutingHomeToolID,
					"name": "message_room",
					"arguments": map[string]any{
						"to": A2ARoutingToSlug,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CreateAgentEntry(bg, db.CreateAgentEntryParams{
		LineageID:        lineage,
		EntryID:          a2aRoutingHomeQuoteID,
		EntryType:        "message",
		OriginOrder:      9990,
		PayloadJson:      string(payload),
		OriginThreadID:   homeThreadID,
		SessionTimestamp: time.Now().UTC(),
	}); err != nil {
		if !strings.Contains(err.Error(), "UNIQUE") &&
			!strings.Contains(err.Error(), "unique") {
			t.Fatalf("seed home quote: %v", err)
		}
	}
	_ = q.UpdateAgentThreadHead(bg, db.UpdateAgentThreadHeadParams{
		HeadEntryID: sql.NullString{String: a2aRoutingHomeQuoteID, Valid: true},
		ID:          homeThreadID,
	})
}
