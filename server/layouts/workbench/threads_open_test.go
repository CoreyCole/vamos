package workbench

import (
	"net/http"
	"strings"
	"testing"
)

func TestThreadsOpenFromRequest(t *testing.T) {
	t.Parallel()

	req := &http.Request{Header: http.Header{}}
	if !ThreadsOpenFromRequest(req) {
		t.Fatal("missing cookie should default open")
	}
	req.AddCookie(&http.Cookie{Name: ThreadsOpenCookie, Value: "0"})
	if ThreadsOpenFromRequest(req) {
		t.Fatal("cookie 0 should keep threads closed")
	}
	req2 := &http.Request{Header: http.Header{}}
	req2.AddCookie(&http.Cookie{Name: ThreadsOpenCookie, Value: "1"})
	if !ThreadsOpenFromRequest(req2) {
		t.Fatal("cookie 1 should keep threads open")
	}
}

func TestThreadsToggleClickActionsWriteCookieNotLayoutSave(t *testing.T) {
	t.Parallel()

	hide := ThreadsHideClickAction()
	show := ThreadsShowClickAction("workbenchV2Threads")
	for _, action := range []string{hide, show} {
		if !strings.Contains(action, ThreadsOpenCookie+"=") {
			t.Fatalf("missing cookie write: %q", action)
		}
		if strings.Contains(action, "workbench-layout-save") {
			t.Fatalf("must not persist via layout prefs: %q", action)
		}
	}
	if !strings.Contains(hide, "visible = false") || !strings.Contains(hide, "=0;") {
		t.Fatalf("hide action = %q", hide)
	}
	if !strings.Contains(show, "visible = true") || !strings.Contains(show, "=1;") {
		t.Fatalf("show action = %q", show)
	}
}

func TestThreadsOpenCookieDrivesEncodeWorkbenchSignals(t *testing.T) {
	t.Parallel()

	req := &http.Request{Header: http.Header{}}
	req.AddCookie(&http.Cookie{Name: ThreadsOpenCookie, Value: "0"})
	state, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen:  ThreadsOpenFromRequest(req),
		ChatOpen:     true,
		ArtifactOpen: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.Regions[0].ID != WorkbenchV2ThreadsRegionID || state.Regions[0].Visible {
		t.Fatalf("threads region = %#v, want closed", state.Regions[0])
	}
	signals := EncodeWorkbenchSignals(state)
	idx := strings.Index(signals, "workbenchV2Threads")
	if idx < 0 {
		t.Fatalf("signals missing threads key: %s", signals)
	}
	end := idx + 160
	if end > len(signals) {
		end = len(signals)
	}
	window := signals[idx:end]
	if !strings.Contains(window, `"visible":false`) {
		t.Fatalf("SSR signals must keep threads closed: %s", window)
	}
}

func TestRegionInitialClass_ThreadsClosedLocksHidden(t *testing.T) {
	t.Parallel()
	req := &http.Request{Header: http.Header{}}
	req.AddCookie(&http.Cookie{Name: ThreadsOpenCookie, Value: "0"})
	state, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen:  ThreadsOpenFromRequest(req),
		ChatOpen:     true,
		ArtifactOpen: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	threads := state.Regions[0]
	if threads.Visible {
		t.Fatal("threads should be closed from cookie")
	}
	got := RegionInitialClass(state, threads)
	if !strings.Contains(got, "md:!hidden") || !strings.Contains(got, "hidden") {
		t.Fatalf("closed threads initial class = %q, want hidden md:!hidden lock", got)
	}
	// Open state must use md:!flex so data-class hydrate is a no-op visually.
	stateOpen, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen: true, ChatOpen: true, ArtifactOpen: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	gotOpen := RegionInitialClass(stateOpen, stateOpen.Regions[0])
	if !strings.Contains(gotOpen, "md:!flex") {
		t.Fatalf("open threads initial class = %q, want md:!flex", gotOpen)
	}
}

