package workbench

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatOpenFromRequest(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if !ChatOpenFromRequest(req) {
		t.Fatal("missing cookie should default open")
	}
	req.AddCookie(&http.Cookie{Name: ChatOpenCookie, Value: "0"})
	if ChatOpenFromRequest(req) {
		t.Fatal("cookie=0 should keep chat minimized")
	}
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(&http.Cookie{Name: ChatOpenCookie, Value: "1"})
	if !ChatOpenFromRequest(req2) {
		t.Fatal("cookie=1 should keep chat open")
	}
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.AddCookie(&http.Cookie{Name: ChatOpenCookie, Value: "nope"})
	if !ChatOpenFromRequest(req3) {
		t.Fatal("invalid cookie should default open")
	}
}

func TestChatOpenFromRequestDefaultClosed(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if ChatOpenFromRequestDefault(req, false) {
		t.Fatal("missing cookie should honor default closed")
	}
	req.AddCookie(&http.Cookie{Name: ChatOpenCookie, Value: "1"})
	if !ChatOpenFromRequestDefault(req, false) {
		t.Fatal("cookie=1 should open even when default closed")
	}
}

func TestChatToggleClickActionWritesChatCookieBothWays(t *testing.T) {
	t.Parallel()
	js := ChatToggleClickAction()
	if !strings.Contains(js, ChatOpenCookie+"=0") {
		t.Fatalf("close must write cookie 0: %s", js)
	}
	if !strings.Contains(js, ChatOpenCookie+"=1") {
		t.Fatalf("open must write cookie 1: %s", js)
	}
	if !strings.Contains(js, CommentsOpenCookie+"=0") {
		t.Fatalf("opening chat must clear comments cookie: %s", js)
	}
	if strings.Contains(js, "ExecuteScript") {
		t.Fatalf("must not use ExecuteScript: %s", js)
	}
	if strings.Contains(js, "workbench-layout-save") {
		t.Fatalf("must not persist via layout prefs: %s", js)
	}
	if strings.Contains(js, "workbenchV2Threads.visible = false") {
		t.Fatalf("closing chat must not hide threads: %s", js)
	}
}

func TestChatVisibleCoalesceUsesSSRBool(t *testing.T) {
	t.Parallel()
	if got := ChatVisibleCoalesce(
		true,
	); got != "$workbench.regions.workbenchV2Chat.visible ?? true" {
		t.Fatalf("open coalesce = %q", got)
	}
	if got := ChatToggleDataClass(true); !strings.Contains(got, "?? true") {
		t.Fatalf("data-class missing ?? true: %s", got)
	}
	if got := ChatToggleAriaPressed(
		false,
	); got != "$workbench.regions.workbenchV2Chat.visible ?? false ? 'true' : 'false'" {
		t.Fatalf("closed aria = %q", got)
	}
}

func TestChatOpenCookieDrivesEncodeWorkbenchSignals(t *testing.T) {
	t.Parallel()

	missing := httptest.NewRequest(http.MethodGet, "/", nil)
	state, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen:  true,
		ChatOpen:     ChatOpenFromRequest(missing),
		ArtifactOpen: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	signals := EncodeWorkbenchSignals(state)
	idx := strings.Index(signals, "workbenchV2Chat")
	if idx < 0 {
		t.Fatalf("signals missing chat key: %s", signals)
	}
	end := idx + 160
	if end > len(signals) {
		end = len(signals)
	}
	window := signals[idx:end]
	if !strings.Contains(window, `"visible":true`) {
		t.Fatalf("missing cookie must SSR chat visible true: %s", window)
	}

	req0 := httptest.NewRequest(http.MethodGet, "/", nil)
	req0.AddCookie(&http.Cookie{Name: ChatOpenCookie, Value: "0"})
	closed, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen:  true,
		ChatOpen:     ChatOpenFromRequest(req0),
		ArtifactOpen: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	signals = EncodeWorkbenchSignals(closed)
	idx = strings.Index(signals, "workbenchV2Chat")
	end = idx + 160
	if end > len(signals) {
		end = len(signals)
	}
	window = signals[idx:end]
	if !strings.Contains(window, `"visible":false`) {
		t.Fatalf("cookie 0 must SSR chat visible false: %s", window)
	}
}
