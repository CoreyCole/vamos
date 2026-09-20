package vamos

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"
	"github.com/playwright-community/playwright-go"
)

// Agent-memory VA fixture query contract (dual form). Prefer *_fixture=1.
const (
	DensityFixtureQueryKey       = "density_fixture"
	GroupBubbleFixtureQueryKey   = "group_bubble_fixture"
	PairwiseFixtureQueryKey      = "pairwise_fixture"
	HistoryFixtureQueryKey       = "history_fixture"
	MessageThreadFixtureQueryKey = "message_thread_fixture"
	FixtureAliasQueryKey         = "fixture"
	DensityFixtureQueryValue     = "1"
	GroupBubbleFixtureAlias      = "group"
	PairwiseFixtureAlias         = "pairwise"
	HistoryFixtureAlias          = "history"
	MessageThreadFixtureAlias    = "replies"
	DensityFixtureAlias          = "density"
	DefaultAgentMemoryThreadID   = "wb2_alpha"
	// Dogfood tip-host thread (set VAMOS_E2E_AGENT_MEMORY_THREAD_ID).
	DogfoodAgentMemoryThreadID  = "65f8ec8e-02c0-43bd-8cd4-fe3638178221"
	envAgentMemoryFixtureThread = "VAMOS_E2E_AGENT_MEMORY_THREAD_ID"
)

// DOM contract (rendered ids use msg- prefix; chip id does not).
const (
	DensityReasoningDOMID      = "msg-ai470-density-fixture-reasoning"
	DensityTool0DOMID          = "msg-ai470-density-fixture-tool-0"
	DensityTool1DOMID          = "msg-ai470-density-fixture-tool-1"
	GroupPeerDOMID             = "msg-ai470-group-peer-fixture"
	GroupQuoteDOMID            = "msg-ai470-group-quote-fixture"
	GroupUserDOMID             = "msg-ai470-group-user-fixture"
	GroupBotDMChipDOMID        = "bot-dm-chip-ai470-group-quote-fixture"
	ChromaHighlightDOMID       = "msg-ai470-chroma-highlight-fixture"
	PairwisePeerDOMID          = "msg-ai470-pairwise-peer-fixture"
	PairwiseSelfDOMID          = "msg-ai470-pairwise-self-fixture"
	HistoryScrollSentinelDOMID = "agent-chat-scroll-sentinel-above"
	// First-paint window after windowStableTranscript (limit=50, extra=5).
	HistoryFixtureFirstPaintStart = 5
	HistoryFixtureFirstPaintEnd   = 54
	HistoryFixtureOlderBefore     = "ai470-history-fixture-5"
	MessageThreadParentDOMID      = "msg-ai470-message-thread-parent"
	MessageThreadSummaryDOMID     = "msg-ai470-message-thread-parent-thread-summary"
	MessageThreadPanelDOMID       = "agent-chat-message-thread"
	MessageThreadReply1Body       = "First reply from Corey."
	MessageThreadReply2Body       = "Second reply from Lead."
	PairwiseRoomInfraLead         = "/rooms/a2a/infra/lead"
	PairwiseRoomLeadResearch      = "/rooms/a2a/lead/research"
)

var AgentMemory agentMemoryFeature

type agentMemoryFeature struct{}

func AgentMemoryFixtureThreadID() string {
	if v := strings.TrimSpace(os.Getenv(envAgentMemoryFixtureThread)); v != "" {
		return v
	}
	return DefaultAgentMemoryThreadID
}

func (pages) ThreadWithFixture(threadID, fixtureQuery string) page {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		threadID = DefaultAgentMemoryThreadID
	}
	q := strings.TrimSpace(fixtureQuery)
	path := "/threads/" + url.PathEscape(threadID)
	if q != "" {
		if strings.HasPrefix(q, "?") {
			path += q
		} else {
			path += "?" + q
		}
	}
	return page{label: "thread fixture " + threadID, path: path}
}

func (pages) DensityFixtureThread(threadID string) page {
	return Pages.ThreadWithFixture(
		threadID,
		DensityFixtureQueryKey+"="+DensityFixtureQueryValue,
	)
}

func (pages) GroupBubbleFixtureThread(threadID string) page {
	return Pages.ThreadWithFixture(
		threadID,
		GroupBubbleFixtureQueryKey+"="+DensityFixtureQueryValue,
	)
}

func (pages) PairwiseFixtureThread(threadID string) page {
	return Pages.ThreadWithFixture(
		threadID,
		PairwiseFixtureQueryKey+"="+DensityFixtureQueryValue,
	)
}

func (pages) HistoryFixtureThread(threadID string) page {
	return Pages.ThreadWithFixture(
		threadID,
		HistoryFixtureQueryKey+"="+DensityFixtureQueryValue,
	)
}

func (pages) PairwiseA2ARoom(a, b string) page {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a > b {
		a, b = b, a
	}
	return page{
		label: "pairwise room " + a + "/" + b,
		path:  "/rooms/a2a/" + url.PathEscape(a) + "/" + url.PathEscape(b),
	}
}

func (agentMemoryFeature) DensityReasoning() spec.Locator {
	return spec.CSS("#" + DensityReasoningDOMID)
}

func (agentMemoryFeature) DensityTool0() spec.Locator {
	return spec.CSS("#" + DensityTool0DOMID)
}

func (agentMemoryFeature) DensityTool1() spec.Locator {
	return spec.CSS("#" + DensityTool1DOMID)
}

func (agentMemoryFeature) DensityReasoningDetails() spec.Locator {
	return spec.CSS("#" + DensityReasoningDOMID + "[data-chat-density='reasoning']")
}

func (agentMemoryFeature) DensityToolDetails() spec.Locator {
	return spec.CSS("details[data-chat-density='tool']")
}

func (agentMemoryFeature) GroupPeer() spec.Locator {
	return spec.CSS("#" + GroupPeerDOMID)
}

func (agentMemoryFeature) GroupQuote() spec.Locator {
	return spec.CSS("#" + GroupQuoteDOMID)
}

func (agentMemoryFeature) GroupUser() spec.Locator {
	return spec.CSS("#" + GroupUserDOMID)
}

func (agentMemoryFeature) BotDMChip() spec.Locator {
	return spec.CSS("#" + GroupBotDMChipDOMID)
}

func (agentMemoryFeature) ChromaHighlight() spec.Locator {
	return spec.CSS("#" + ChromaHighlightDOMID)
}

func (agentMemoryFeature) PairwisePeer() spec.Locator {
	return spec.CSS("#" + PairwisePeerDOMID)
}

func (agentMemoryFeature) PairwiseSelf() spec.Locator {
	return spec.CSS("#" + PairwiseSelfDOMID)
}

func (agentMemoryFeature) HistoryScrollSentinel() spec.Locator {
	return spec.CSS("#" + HistoryScrollSentinelDOMID)
}

func (agentMemoryFeature) HistoryFixtureMsg(n int) spec.Locator {
	return spec.CSS(fmt.Sprintf("#msg-ai470-history-fixture-%d", n))
}

func (agentMemoryFeature) ComposerForm() spec.Locator {
	return spec.CSS(
		"#workbench-v2-chat-body #agent-chat-composer-form, #agent-chat-composer-form",
	)
}

func OpenAgentMemoryDensityFixture() spec.Step {
	return OpenAgentMemoryThreadFixture(
		"density",
		DensityFixtureQueryKey+"="+DensityFixtureQueryValue,
	)
}

func OpenAgentMemoryGroupBubbleFixture() spec.Step {
	return OpenAgentMemoryThreadFixture(
		"group_bubble",
		GroupBubbleFixtureQueryKey+"="+DensityFixtureQueryValue,
	)
}

func OpenAgentMemoryPairwiseFixture() spec.Step {
	return OpenAgentMemoryThreadFixture(
		"pairwise",
		PairwiseFixtureQueryKey+"="+DensityFixtureQueryValue,
	)
}

func OpenAgentMemoryHistoryFixture() spec.Step {
	return OpenAgentMemoryThreadFixture(
		"history",
		HistoryFixtureQueryKey+"="+DensityFixtureQueryValue,
	)
}

func OpenAgentMemoryMessageThreadFixture() spec.Step {
	return OpenAgentMemoryThreadFixture(
		"message_thread",
		MessageThreadFixtureQueryKey+"="+DensityFixtureQueryValue,
	)
}

func OpenAgentMemoryThreadFixture(label, fixtureQuery string) spec.Step {
	return customStep(
		"open agent-memory "+label+" fixture thread",
		func(t testing.TB, ctx *duiruntime.Context) {
			t.Helper()
			visit(
				t,
				ctx,
				Pages.ThreadWithFixture(AgentMemoryFixtureThreadID(), fixtureQuery).
					Path(),
			)
		},
	)
}

func ExpectChatDensityFixtureSeeded() expectation {
	return expectation{
		customStep(
			"chat density fixture seeded",
			func(t testing.TB, ctx *duiruntime.Context) {
				t.Helper()
				waitVisible(t, ctx, "#"+DensityReasoningDOMID)
				waitVisible(t, ctx, "#"+DensityTool0DOMID)
				waitVisible(t, ctx, "#"+DensityTool1DOMID)
				assertCount(
					t,
					ctx,
					"#"+DensityReasoningDOMID+"[data-chat-density='reasoning']",
					1,
				)
				assertCount(t, ctx, "#"+DensityTool0DOMID+"[data-chat-density='tool']", 1)
				assertCount(t, ctx, "#"+DensityTool1DOMID+"[data-chat-density='tool']", 1)
				assertCount(t, ctx, "details[data-chat-density='reasoning']", 1)
				toolCount, err := ctx.Page.Locator("details[data-chat-density='tool']").
					Count()
				if err != nil || toolCount < 2 {
					t.Fatalf(
						"expected >=2 tool density details, got %d err=%v",
						toolCount,
						err,
					)
				}
				// Class A: native details — not signal-toggled remorph chrome.
				for _, id := range []string{DensityReasoningDOMID, DensityTool0DOMID, DensityTool1DOMID} {
					tag, err := ctx.Page.Locator("#"+id).
						First().
						Evaluate("el => el.tagName", nil)
					if err != nil {
						t.Fatal(err)
					}
					if strings.ToUpper(fmt.Sprint(tag)) != "DETAILS" {
						t.Fatalf("%s tag=%v want DETAILS", id, tag)
					}
				}
			},
		),
	}
}

func ExpectGroupBubbleFixtureSeeded() expectation {
	return expectation{
		customStep(
			"group bubble fixture seeded",
			func(t testing.TB, ctx *duiruntime.Context) {
				t.Helper()
				waitVisible(t, ctx, "#"+GroupPeerDOMID)
				waitVisible(t, ctx, "#"+GroupQuoteDOMID)
				waitVisible(t, ctx, "#"+GroupUserDOMID)
				waitVisible(t, ctx, "#"+GroupBotDMChipDOMID)
				waitVisible(t, ctx, "#"+ChromaHighlightDOMID)
			},
		),
	}
}

func ExpectBotDMChipPairwiseLinks() expectation {
	return expectation{
		customStep(
			"BotDMChip exposes pairwise room links",
			func(t testing.TB, ctx *duiruntime.Context) {
				t.Helper()
				chip := ctx.Page.Locator("#" + GroupBotDMChipDOMID)
				if err := chip.First().WaitFor(); err != nil {
					t.Fatal(err)
				}
				trigger := chip.Locator("[data-slot='dropdown-menu-trigger'], button, [role='button']").
					First()
				if count, _ := trigger.Count(); count > 0 {
					if err := trigger.Click(); err != nil {
						t.Fatalf("open BotDMChip popover: %v", err)
					}
				} else if err := chip.First().Click(); err != nil {
					t.Fatalf("click BotDMChip: %v", err)
				}
				for _, href := range []string{PairwiseRoomInfraLead, PairwiseRoomLeadResearch} {
					loc := ctx.Page.Locator("a[href='" + href + "']")
					if err := loc.First().WaitFor(); err != nil {
						t.Fatalf("missing chip link %s: %v", href, err)
					}
				}
			},
		),
	}
}

func ExpectMessageThreadFixtureSeeded() expectation {
	return expectation{
		customStep(
			"message thread fixture seeded",
			func(t testing.TB, ctx *duiruntime.Context) {
				t.Helper()
				waitVisible(t, ctx, "#"+MessageThreadParentDOMID)
				waitVisible(t, ctx, "#"+MessageThreadSummaryDOMID)
				waitVisible(t, ctx, "#agent-chat-scroll-region")
				summary, err := ctx.Page.Locator("#" + MessageThreadSummaryDOMID).
					First().
					InnerText()
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(summary, "2 replies") {
					t.Fatalf("summary copy=%q want 2 replies", summary)
				}
				if err := ctx.Page.Locator("#" + MessageThreadSummaryDOMID).
					First().
					Click(); err != nil {
					t.Fatalf("open thread summary: %v", err)
				}
				waitVisible(t, ctx, "#"+MessageThreadPanelDOMID)
				waitVisible(t, ctx, "#agent-chat-scroll-region")
				body, berr := ctx.Page.Locator("#" + MessageThreadPanelDOMID).
					First().
					InnerText()
				if berr != nil {
					t.Fatal(berr)
				}
				for _, want := range []string{MessageThreadReply1Body, MessageThreadReply2Body} {
					if !strings.Contains(body, want) {
						t.Fatalf("thread panel missing %q; got %q", want, body)
					}
				}
				family, _ := ctx.Page.Locator("#agent-chat-thread-family").Count()
				if family != 0 {
					t.Fatalf("family bar still visible (count=%d)", family)
				}
			},
		),
	}
}

func ExpectPairwiseFixtureViewOnly() expectation {
	return expectation{
		customStep(
			"pairwise fixture is view-only",
			func(t testing.TB, ctx *duiruntime.Context) {
				t.Helper()
				waitVisible(t, ctx, "#"+PairwisePeerDOMID)
				waitVisible(t, ctx, "#"+PairwiseSelfDOMID)
				waitVisible(t, ctx, "#"+ChromaHighlightDOMID)
				count, err := ctx.Page.Locator("#agent-chat-composer-form").Count()
				if err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatalf("pairwise fixture still has composer form (count=%d)", count)
				}
				// Soft: BotDMChip remains on group fixture for now; do not assert on pairwise yet.
			},
		),
	}
}

func ExpectHistoryFixtureInfiniteScroll() expectation {
	return expectation{
		customStep(
			"history fixture InfiniteScroll first paint",
			func(t testing.TB, ctx *duiruntime.Context) {
				t.Helper()
				waitVisible(t, ctx, "#"+HistoryScrollSentinelDOMID)
				waitVisible(
					t,
					ctx,
					fmt.Sprintf(
						"#msg-ai470-history-fixture-%d",
						HistoryFixtureFirstPaintStart,
					),
				)
				waitVisible(
					t,
					ctx,
					fmt.Sprintf(
						"#msg-ai470-history-fixture-%d",
						HistoryFixtureFirstPaintEnd,
					),
				)
				// Sample: windowed first-paint should include [5..54] (50 msgs).
				painted, err := ctx.Page.Locator(`[id^="msg-ai470-history-fixture-"]`).
					Count()
				if err != nil || painted < 40 {
					t.Fatalf(
						"history fixture painted count=%d want >=40 err=%v",
						painted,
						err,
					)
				}
				// Oldest pre-window ids must not be on first paint.
				assertCount(t, ctx, "#msg-ai470-history-fixture-0", 0)
				// Clear hook from PatchAboveExpr on the sentinel (unit-tested contract).
				html, herr := ctx.Page.Locator("#"+HistoryScrollSentinelDOMID).
					First().
					Evaluate("el => el.outerHTML", nil)
				if herr != nil {
					t.Fatalf("read sentinel outerHTML: %v", herr)
				}
				s := fmt.Sprint(html)
				if !strings.Contains(s, "before="+HistoryFixtureOlderBefore) &&
					!strings.Contains(
						s,
						"before="+url.QueryEscape(HistoryFixtureOlderBefore),
					) {
					t.Fatalf(
						"sentinel missing before-cursor %s; html=%s",
						HistoryFixtureOlderBefore,
						s,
					)
				}
			},
		),
	}
}

func waitVisible(t testing.TB, ctx *duiruntime.Context, selector string) {
	t.Helper()
	if err := ctx.Page.Locator(selector).First().WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
	}); err != nil {
		t.Fatalf("wait visible %s: %v", selector, err)
	}
}

func assertCount(t testing.TB, ctx *duiruntime.Context, selector string, want int) {
	t.Helper()
	got, err := ctx.Page.Locator(selector).Count()
	if err != nil || got != want {
		t.Fatalf("selector %s count=%d want=%d err=%v", selector, got, want, err)
	}
}
