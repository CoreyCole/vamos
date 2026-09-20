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
