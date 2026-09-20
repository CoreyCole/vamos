package markdown

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
)

func TestChatCommentsOpenHonorsChatCookie(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	chatOpen, commentsOpen := chatCommentsOpen(req, true)
	if !chatOpen || commentsOpen {
		t.Fatalf("missing cookie: chatOpen=%v commentsOpen=%v", chatOpen, commentsOpen)
	}

	req0 := httptest.NewRequest(http.MethodGet, "/", nil)
	req0.AddCookie(&http.Cookie{Name: workbench.ChatOpenCookie, Value: "0"})
	chatOpen, commentsOpen = chatCommentsOpen(req0, true)
	if chatOpen || commentsOpen {
		t.Fatalf("cookie=0: chatOpen=%v commentsOpen=%v", chatOpen, commentsOpen)
	}

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.AddCookie(&http.Cookie{Name: workbench.ChatOpenCookie, Value: "1"})
	chatOpen, commentsOpen = chatCommentsOpen(req1, true)
	if !chatOpen || commentsOpen {
		t.Fatalf("cookie=1: chatOpen=%v commentsOpen=%v", chatOpen, commentsOpen)
	}

	reqComments := httptest.NewRequest(http.MethodGet, "/", nil)
	reqComments.AddCookie(&http.Cookie{Name: workbench.ChatOpenCookie, Value: "1"})
	reqComments.AddCookie(&http.Cookie{Name: workbench.CommentsOpenCookie, Value: "1"})
	chatOpen, commentsOpen = chatCommentsOpen(reqComments, true)
	if chatOpen || !commentsOpen {
		t.Fatalf("comments win: chatOpen=%v commentsOpen=%v", chatOpen, commentsOpen)
	}

	reqRoute := httptest.NewRequest(http.MethodGet, "/", nil)
	chatOpen, _ = chatCommentsOpen(reqRoute, false)
	if chatOpen {
		t.Fatal("routeChatOpen false must keep chat closed")
	}
}
