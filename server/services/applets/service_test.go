package applets

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
)

func TestBuildWorkbenchStateUsesOnlyFilesAppAndChatRegions(t *testing.T) {
	state := BuildWorkbenchState(t.Context(), WorkbenchState{
		Config: AppletConfig{ID: "pickleball"},
		Files:  FilesViewModel{Title: "Files", Component: EmptyRegion("files")},
		Chat:   ChatViewModel{Title: "Chat", Component: EmptyRegion("chat")},
	})

	if len(state.Regions) != 2 {
		t.Fatalf("regions = %d, want 2", len(state.Regions))
	}

	files := state.Regions[0]
	if files.ID != "pickleball-files-app" || files.Slot != workbench.WorkbenchSlotPrimary || files.Title != "Files" {
		t.Fatalf("files region = %#v", files)
	}
	if files.TargetID != "pickleball-files-app-region" {
		t.Fatalf("files target = %q", files.TargetID)
	}

	chat := state.Regions[1]
	if chat.ID != "pickleball-chat" || chat.Slot != workbench.WorkbenchSlotContext || chat.Title != "Chat" {
		t.Fatalf("chat region = %#v", chat)
	}
	if chat.TargetID != "pickleball-chat-region" {
		t.Fatalf("chat target = %q", chat.TargetID)
	}

	for _, region := range state.Regions {
		if region.Slot == workbench.WorkbenchSlotNavigation || region.Kind == workbench.RegionWorkspaceTopology {
			t.Fatalf("technical/navigation region leaked: %#v", region)
		}
	}
}

func TestBuildWorkbenchStateDefaultsMobileToFilesApp(t *testing.T) {
	state := BuildWorkbenchState(t.Context(), WorkbenchState{Config: AppletConfig{ID: "pickleball"}})

	if got := state.Config.Mobile.ActiveRegionID; got != "pickleball-files-app" {
		t.Fatalf("mobile active region = %q, want files/app", got)
	}
}

func TestEmptyRegionRendersFriendlyPlaceholder(t *testing.T) {
	var body bytes.Buffer
	if err := EmptyRegion("Files will appear here.").Render(t.Context(), &body); err != nil {
		t.Fatalf("render empty region: %v", err)
	}
	if !strings.Contains(body.String(), "Files will appear here.") {
		t.Fatalf("placeholder body = %q", body.String())
	}
}
