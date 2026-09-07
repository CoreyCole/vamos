package agenthome

import (
	"strings"
	"testing"
)

func TestRosterAgentRowClass_SelectedElevatesOffMutedRail(t *testing.T) {
	t.Parallel()
	got := rosterAgentRowClass(true)
	if strings.Contains(got, "bg-muted") {
		t.Fatalf("selected row must not use bg-muted on muted rail: %q", got)
	}
	if !strings.Contains(got, "roster-row-selected") {
		t.Fatalf("selected want roster-row-selected, got %q", got)
	}
	idle := rosterAgentRowClass(false)
	if strings.Contains(idle, "roster-row-selected") {
		t.Fatalf("idle must not be selected: %q", idle)
	}
}

func TestRosterPinClass_SelectedDistinct(t *testing.T) {
	t.Parallel()
	got := rosterPinClass(true)
	if !strings.Contains(got, "roster-pin-selected") {
		t.Fatalf("selected pin want roster-pin-selected: %q", got)
	}
}
