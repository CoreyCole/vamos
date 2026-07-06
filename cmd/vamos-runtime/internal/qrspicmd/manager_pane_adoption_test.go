package qrspicmd

import (
	"strings"
	"testing"
)

func TestManagerPaneAdoptionExplicitRebindsLiveDifferentParent(t *testing.T) {
	state := ManagerState{
		ManagerPaneID: "%old",
		Delivery:      ManagerDeliveryState{ManagerPaneID: "%old"},
	}
	got, err := ResolveManagerPaneAdoption(t.Context(), state, ManagerPaneAdoptionOptions{
		StateFile:    "state.json",
		Command:      ManagerPaneAdoptionContinue,
		ExplicitPane: "%new",
		CurrentPane:  "%env",
	}, deps{Tmux: &recordingTmux{}})
	if err != nil {
		t.Fatal(err)
	}
	if got.ActionCard != nil || !got.Changed || got.State.ManagerPaneID != "%new" ||
		got.State.Delivery.ManagerPaneID != "%new" ||
		got.Reason != "explicit_manager_pane" {
		t.Fatalf("adoption = %+v", got)
	}
}

func TestManagerPaneAdoptionCurrentPaneWhenStoredBlank(t *testing.T) {
	got, err := ResolveManagerPaneAdoption(
		t.Context(),
		ManagerState{},
		ManagerPaneAdoptionOptions{
			StateFile:   "state.json",
			Command:     ManagerPaneAdoptionContinue,
			CurrentPane: "%parent",
		},
		deps{Tmux: &recordingTmux{}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Changed || got.State.ManagerPaneID != "%parent" || got.ActionCard != nil {
		t.Fatalf("adoption = %+v", got)
	}
}

func TestManagerPaneAdoptionCurrentPaneWhenStoredDead(t *testing.T) {
	state := ManagerState{
		ManagerPaneID: "%dead",
		Delivery:      ManagerDeliveryState{ManagerPaneID: "%dead"},
	}
	tmux := &recordingTmux{missingPanes: map[string]bool{"%dead": true}}
	got, err := ResolveManagerPaneAdoption(t.Context(), state, ManagerPaneAdoptionOptions{
		StateFile:   "state.json",
		Command:     ManagerPaneAdoptionStartNext,
		CurrentPane: "%new",
	}, deps{Tmux: tmux})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Changed || got.State.ManagerPaneID != "%new" ||
		got.State.Delivery.ManagerPaneID != "%new" {
		t.Fatalf("adoption = %+v", got)
	}
}

func TestManagerPaneAdoptionLiveConflictRequiresExplicitPane(t *testing.T) {
	state := ManagerState{
		ManagerPaneID: "%old",
		Delivery:      ManagerDeliveryState{ManagerPaneID: "%old"},
	}
	got, err := ResolveManagerPaneAdoption(t.Context(), state, ManagerPaneAdoptionOptions{
		StateFile:   "state.json",
		Command:     ManagerPaneAdoptionContinue,
		CurrentPane: "%new",
	}, deps{Tmux: &recordingTmux{}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Changed || got.ActionCard == nil ||
		got.ActionCard.Kind != ActionManagerPaneAdoptionRequired ||
		!strings.Contains(got.ActionCard.SafeCommand, "continue --state-file state.json --manager-pane") {
		t.Fatalf("adoption = %+v", got)
	}
}

func TestManagerPaneAdoptionDoesNotAdoptUnavailableCurrentPane(t *testing.T) {
	tmux := &recordingTmux{missingPanes: map[string]bool{"%new": true}}
	got, err := ResolveManagerPaneAdoption(
		t.Context(),
		ManagerState{},
		ManagerPaneAdoptionOptions{
			StateFile:   "state.json",
			Command:     ManagerPaneAdoptionContinue,
			CurrentPane: "%new",
		},
		deps{Tmux: tmux},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Changed || got.State.ManagerPaneID != "" ||
		got.Reason != "current_manager_pane_unavailable" {
		t.Fatalf("adoption = %+v", got)
	}
}
