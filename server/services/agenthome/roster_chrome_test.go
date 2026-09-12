package agenthome

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRosterIsPinned(t *testing.T) {
	t.Parallel()
	if rosterIsPinned(KindDM, "bot") {
		t.Fatal("SSR must not fixture-pin dm:bot")
	}
	if rosterIsPinned(KindGroup, "vamos-dev") {
		t.Fatal("SSR must not fixture-pin group:vamos-dev")
	}
}

func TestRosterAgentRowClass_SelectedUsesChipClass(t *testing.T) {
	t.Parallel()
	got := rosterAgentRowClass(true)
	if strings.Contains(got, "bg-muted") {
		t.Fatalf("selected row must not use bg-muted (same as --accent): %q", got)
	}
	if !strings.Contains(got, "roster-row-selected") {
		t.Fatalf("selected want roster-row-selected, got %q", got)
	}
	idle := rosterAgentRowClass(false)
	if strings.Contains(idle, "roster-row-selected") {
		t.Fatalf("idle must not be selected: %q", idle)
	}
}

func TestRosterPinClass_SelectedFillNoBorder(t *testing.T) {
	t.Parallel()
	got := rosterPinClass(true)
	if !strings.Contains(got, "roster-pin-selected") {
		t.Fatalf("selected pin want roster-pin-selected: %q", got)
	}
	idle := rosterPinClass(false)
	if strings.Contains(idle, "border") {
		t.Fatalf("unselected pin must not carry a border tile: %q", idle)
	}
}

func TestRosterRail_EmptyBots(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := RosterRail(RosterView{}).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "No bots yet") {
		t.Fatal("empty agents must say No bots yet")
	}
	for _, forbidden := range []string{
		"Research agent",
		"Short path — how do I get read-only access?",
		`href="/rooms/plan/alpha"`,
		`href="/rooms/group/vamos-dev"`,
		"Group chats",
		"Vamos Lead",
		"Ready when you are.",
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("fixture leftover %q in empty roster", forbidden)
		}
	}
}

func TestRosterRail_LiveBotsAndChrome(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	view := RosterView{
		Selection: RosterSelection{Kind: KindDM, ID: "live-bot"},
		Bots: []RosterBotRow{
			{Slug: "live-bot", Title: "Live bot"},
			{Slug: "other", Title: "other"},
		},
	}
	if err := RosterRail(view).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, `id="roster-row-dm-live-bot"`) {
		t.Fatal("live bot list row must exist")
	}
	if !strings.Contains(html, `href="/rooms/dm/live-bot"`) {
		t.Fatal("bot href must be /rooms/dm/{slug}")
	}
	if !strings.Contains(html, `data-roster-id="dm:live-bot"`) {
		t.Fatal("rows must expose data-roster-id for pin JS")
	}
	if !strings.Contains(html, "Live bot") {
		t.Fatal("title must use agent name")
	}
	if !strings.Contains(html, `data-roster-action="pin"`) {
		t.Fatal("context menu must expose pin action")
	}
	if !strings.Contains(html, "#3a3a3a") {
		t.Fatal("pin and row selected fill must be Grok #3a3a3a")
	}
	if !strings.Contains(html, "inset 3px 0 0 0 hsl(var(--primary))") {
		t.Fatal("selected row must keep primary leading edge")
	}
	if strings.Contains(html, "border-b border-border") {
		t.Fatal("search well must sit on the rail without a header rule")
	}
	if !strings.Contains(html, `id="workbench-v2-roster-header"`) ||
		!strings.Contains(html, "h-10 min-h-10 max-h-10") ||
		!strings.Contains(html, "flex h-7 min-w-0 flex-1") {
		t.Fatal("search header must match other h-10 chrome with an h-7 well")
	}
	if !strings.Contains(html, "Hide roster sidebar (Ctrl+B)") {
		t.Fatal("roster hide control must show Ctrl+B")
	}
	if !strings.Contains(html, "rounded-xl bg-white/[0.05]") {
		t.Fatal("search must be a full-width rounded well")
	}
	if strings.Contains(html, "border-dashed") || strings.Contains(html, "Plan threads") {
		t.Fatal("plan band must stay quiet: no dashed rule, label Plan")
	}
	if !strings.Contains(html, ">Plan</h2>") {
		t.Fatal("plan band label must remain Plan")
	}
	if strings.Contains(html, "private DM") || strings.Contains(html, "Private DM") {
		t.Fatal("roster must not call bot homes private DMs")
	}
	if !strings.Contains(html, ">Bots</h2>") {
		t.Fatal("roster agent band must say Bots")
	}
	if strings.Contains(html, ">Group chats</h2>") {
		t.Fatal("ad-hoc Group chats section must be gone")
	}
	if strings.Contains(html, `href="/rooms/group/vamos-dev"`) ||
		strings.Contains(html, `href="/rooms/plan/alpha"`) {
		t.Fatal("fixture room hrefs must not remain")
	}
	if !strings.Contains(html, "roster-row-selected") {
		t.Fatal("selected live bot must use roster-row-selected")
	}
}

func TestRosterRail_LivePlans(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	view := RosterView{
		Selection: RosterSelection{Kind: KindPlan, ID: "plan-one"},
		Plans: []RosterPlanRow{
			{
				ID:    "plan-one",
				Title: "Plan One",
				Href:  "/rooms/plan/plan-one?artifact=thoughts%2Fowner%2Fplans%2Fplan-one%2Fdesign.md",
				Time:  "Fri 1:02 PM",
			},
			{
				ID:    "plan-two",
				Title: "Plan Two",
				Href:  "/rooms/plan/plan-two?artifact=thoughts%2Fowner%2Fplans%2Fplan-two%2Fdesign.md",
				Time:  "Sat 4:05 PM",
			},
		},
	}
	if err := RosterRail(view).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{
		"Plan One",
		"Plan Two",
		`id="roster-row-plan-plan-one"`,
		`href="/rooms/plan/plan-one?artifact=thoughts%2Fowner%2Fplans%2Fplan-one%2Fdesign.md"`,
		`href="/rooms/plan/plan-two?artifact=thoughts%2Fowner%2Fplans%2Fplan-two%2Fdesign.md"`,
		`data-roster-plan-swatch`,
		"roster-row-plan",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q", want)
		}
	}
	for _, ban := range []string{
		"Fri 1:02 PM",
		"Sat 4:05 PM",
		">P</span>", // letter glyph from rosterInitial
	} {
		if strings.Contains(html, ban) {
			t.Fatalf("plan row must not contain %q", ban)
		}
	}
	if strings.Contains(html, `href="/rooms/plan/alpha"`) ||
		strings.Contains(html, ">Alpha<") {
		t.Fatal("Alpha fixture must not appear")
	}
}
