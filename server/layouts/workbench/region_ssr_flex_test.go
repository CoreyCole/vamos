package workbench

import (
	"strings"
	"testing"
)

func TestRegionSSRFlexStyle_MatchesVisibleRatios(t *testing.T) {
	t.Parallel()
	state, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen:  true,
		ChatOpen:     true,
		ArtifactOpen: true,
		CommentsOpen: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	threads := state.Regions[0]
	got := RegionSSRFlexStyle(state, threads)
	if !strings.HasPrefix(got, "flex: 0 0 ") || !strings.HasSuffix(got, "%") {
		t.Fatalf("threads SSR flex = %q", got)
	}
	closed := threads
	closed.Visible = false
	if RegionSSRFlexStyle(state, closed) != "" {
		t.Fatal("hidden region should omit SSR flex")
	}
}

func TestRegionSSRFlexStyle_ThreadsWidthIndependentOfChat(t *testing.T) {
	t.Parallel()
	open, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen: true, ChatOpen: true, ArtifactOpen: true, CommentsOpen: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	closed, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen: true, ChatOpen: false, ArtifactOpen: true, CommentsOpen: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	openFlex := RegionSSRFlexStyle(open, open.Regions[0])
	closedFlex := RegionSSRFlexStyle(closed, closed.Regions[0])
	if openFlex == "" || openFlex != closedFlex {
		t.Fatalf("threads flex chat-open %q vs chat-closed %q", openFlex, closedFlex)
	}
	if !strings.HasPrefix(openFlex, "flex: 0 0 ") {
		t.Fatalf("threads should freeze grow-0 basis, got %q", openFlex)
	}
	artifactOpen := RegionSSRFlexStyle(
		open,
		regionByID(t, open, WorkbenchV2ArtifactRegionID),
	)
	artifactClosed := RegionSSRFlexStyle(
		closed,
		regionByID(t, closed, WorkbenchV2ArtifactRegionID),
	)
	if !strings.HasPrefix(artifactOpen, "flex: 0 0 ") ||
		!strings.HasSuffix(artifactOpen, "%") {
		t.Fatalf("open artifact must be sidecar 0 0 ratio, got %q", artifactOpen)
	}
	if artifactClosed != "flex: 1 1 0%" {
		t.Fatalf("chat-closed artifact must fill, got %q", artifactClosed)
	}
	chatOpenFlex := RegionSSRFlexStyle(open, regionByID(t, open, WorkbenchV2ChatRegionID))
	if chatOpenFlex != "flex: 1 1 0%" {
		t.Fatalf("chat must grow, got %q", chatOpenFlex)
	}
}

func TestRegionSSRFlexStyle_ClosedArtifactChatFills(t *testing.T) {
	t.Parallel()
	state, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen: true, ChatOpen: true, ArtifactOpen: false, CommentsOpen: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact := regionByID(t, state, WorkbenchV2ArtifactRegionID)
	if artifact.Visible {
		t.Fatal("artifact should be closed")
	}
	if RegionSSRFlexStyle(state, artifact) != "" {
		t.Fatalf(
			"closed artifact must omit flex, got %q",
			RegionSSRFlexStyle(state, artifact),
		)
	}
	if got := RegionInitialClass(state, artifact); !strings.Contains(got, "md:!hidden") ||
		!strings.Contains(got, "hidden") {
		t.Fatalf("closed artifact class = %q", got)
	}
	chat := regionByID(t, state, WorkbenchV2ChatRegionID)
	if RegionSSRFlexStyle(state, chat) != "flex: 1 1 0%" {
		t.Fatalf(
			"chat should grow when details closed, got %q",
			RegionSSRFlexStyle(state, chat),
		)
	}
	openState, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen: true, ChatOpen: true, ArtifactOpen: true, CommentsOpen: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if RegionSSRFlexStyle(
		openState,
		regionByID(t, openState, WorkbenchV2ChatRegionID),
	) != "flex: 1 1 0%" {
		t.Fatal("open chat must still grow")
	}
	openArt := RegionSSRFlexStyle(
		openState,
		regionByID(t, openState, WorkbenchV2ArtifactRegionID),
	)
	if !strings.HasPrefix(openArt, "flex: 0 0 ") {
		t.Fatalf("open artifact sidecar, got %q", openArt)
	}
}

func TestRegionSSRFlexStyle_ChatFillsWhenDetailsClosed(t *testing.T) {
	t.Parallel()
	state, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen: true, ChatOpen: true, ArtifactOpen: false, CommentsOpen: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	chat := regionByID(t, state, WorkbenchV2ChatRegionID)
	if RegionSSRFlexStyle(state, chat) != "flex: 1 1 0%" {
		t.Fatalf(
			"chat should grow when details closed, got %q",
			RegionSSRFlexStyle(state, chat),
		)
	}
	threads := regionByID(t, state, WorkbenchV2ThreadsRegionID)
	if !strings.HasPrefix(RegionSSRFlexStyle(state, threads), "flex: 0 0 ") {
		t.Fatalf("threads should stay frozen, got %q", RegionSSRFlexStyle(state, threads))
	}
	artifact := regionByID(t, state, WorkbenchV2ArtifactRegionID)
	if RegionSSRFlexStyle(state, artifact) != "" {
		t.Fatal("hidden details should omit SSR flex")
	}

	full, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen: false, ChatOpen: true, ArtifactOpen: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	chat = regionByID(t, full, WorkbenchV2ChatRegionID)
	if RegionSSRFlexStyle(full, chat) != "flex: 1 1 0%" {
		t.Fatalf("solo chat should fill, got %q", RegionSSRFlexStyle(full, chat))
	}
}

func TestRegionSSRFlexStyle_ArtifactFillsWhenChatClosed(t *testing.T) {
	t.Parallel()
	state, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen: false, ChatOpen: false, ArtifactOpen: true, CommentsOpen: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact := regionByID(t, state, WorkbenchV2ArtifactRegionID)
	if RegionSSRFlexStyle(state, artifact) != "flex: 1 1 0%" {
		t.Fatalf(
			"doc should grow when chat closed, got %q",
			RegionSSRFlexStyle(state, artifact),
		)
	}
	chat := regionByID(t, state, WorkbenchV2ChatRegionID)
	if RegionSSRFlexStyle(state, chat) != "" {
		t.Fatal("hidden chat should omit SSR flex")
	}
	if got := RegionInitialClass(state, chat); !strings.Contains(got, "md:!hidden") {
		t.Fatalf("hidden chat class = %q", got)
	}
}

func regionByID(t *testing.T, state WorkbenchState, id string) WorkbenchRegion {
	t.Helper()
	for _, r := range state.Regions {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("missing region %s", id)
	return WorkbenchRegion{}
}

func TestRegionSSRFlexStyle_UsesSavedRatios(t *testing.T) {
	t.Parallel()
	saved := &WorkbenchConfig{
		Version: 1,
		Page:    WorkbenchPageThreads,
		View:    WorkbenchViewSplit,
		Regions: []RegionSpec{
			{
				ID:      WorkbenchV2ThreadsRegionID,
				Slot:    WorkbenchSlotNavigation,
				Kind:    RegionPlanSidebar,
				Ratio:   0.3,
				Visible: true,
			},
			{
				ID:      WorkbenchV2ChatRegionID,
				Slot:    WorkbenchSlotContext,
				Kind:    RegionChat,
				Ratio:   0.35,
				Visible: true,
			},
			{
				ID:      WorkbenchV2ArtifactRegionID,
				Slot:    WorkbenchSlotPrimary,
				Kind:    RegionArtifact,
				Ratio:   0.35,
				Visible: true,
			},
			{
				ID:      WorkbenchV2CommentsRegionID,
				Slot:    WorkbenchSlotContext,
				Kind:    RegionComments,
				Ratio:   0.22,
				Visible: false,
			},
		},
	}
	state, err := BuildWorkbenchV2State(WorkbenchV2Args{
		SavedConfig:  saved,
		ThreadsOpen:  true,
		ChatOpen:     true,
		ArtifactOpen: true,
		CommentsOpen: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := RegionSSRFlexStyle(state, state.Regions[0])
	if got != "flex: 0 0 30.00%" {
		t.Fatalf("expected frozen 30%% threads share, got %q", got)
	}
}
