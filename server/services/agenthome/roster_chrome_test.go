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
}
