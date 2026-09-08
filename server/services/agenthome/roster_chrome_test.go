package agenthome

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRosterIsPinned(t *testing.T) {
	t.Parallel()
	if !rosterIsPinned(KindDM, "bot") {
		t.Fatal("Bot pin must hide the Bot agent row")
	}
	if !rosterIsPinned(KindGroup, "vamos-dev") {
		t.Fatal("Vamos Lead pin must hide the Vamos dev group row")
	}
	if rosterIsPinned(KindDM, "research") {
		t.Fatal("Research is not pinned")
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

func TestRosterRail_PinActionAndHrefs(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := RosterRail(
		RosterSelection{Kind: KindGroup, ID: "vamos-dev"},
	).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, `id="roster-row-dm-bot"`) {
		t.Fatal("Bot list row must exist so Unpin can restore it")
	}
	if !strings.Contains(html, `id="roster-row-group-vamos-dev"`) {
		t.Fatal("Vamos dev list row must exist so Unpin can restore it")
	}
	if !strings.Contains(html, `href="/rooms/dm/bot"`) ||
		!strings.Contains(html, `href="/rooms/group/vamos-dev"`) {
		t.Fatal("pin tiles must keep room hrefs")
	}
	if !strings.Contains(html, `data-roster-action="pin"`) {
		t.Fatal("context menu must expose pin action")
	}
	if !strings.Contains(html, "roster-pin-selected") {
		t.Fatal("Vamos Lead pin must be selected")
	}
	if !strings.Contains(html, "#313131") {
		t.Fatal("pin and row selected fill must be Grok #313131")
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
}
