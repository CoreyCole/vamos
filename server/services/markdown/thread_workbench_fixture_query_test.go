package markdown

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestAgentMemoryFixtureQueryContract(t *testing.T) {
	t.Parallel()
	e := echo.New()
	cases := []struct {
		raw     string
		density bool
		group   bool
		pair    bool
		history bool
	}{
		{"density_fixture=1", true, false, false, false},
		{"fixture=density", true, false, false, false},
		{"fixture=Density", true, false, false, false},
		{"group_bubble_fixture=1", false, true, false, false},
		{"fixture=group", false, true, false, false},
		{"pairwise_fixture=1", false, false, true, false},
		{"fixture=pairwise", false, false, true, false},
		{"history_fixture=1", false, false, false, true},
		{"fixture=history", false, false, false, true},
		{"fixture=History", false, false, false, true},
		{"", false, false, false, false},
		{"message_thread_fixture=1", false, false, false, false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/threads/x?"+tc.raw, nil)
		c := e.NewContext(req, httptest.NewRecorder())
		if got := densityFixtureRequested(c); got != tc.density {
			t.Fatalf("%q density=%v want %v", tc.raw, got, tc.density)
		}
		if got := groupBubbleFixtureRequested(c); got != tc.group {
			t.Fatalf("%q group=%v want %v", tc.raw, got, tc.group)
		}
		if got := pairwiseFixtureRequested(c); got != tc.pair {
			t.Fatalf("%q pairwise=%v want %v", tc.raw, got, tc.pair)
		}
		if got := historyFixtureRequested(c); got != tc.history {
			t.Fatalf("%q history=%v want %v", tc.raw, got, tc.history)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/threads/x?message_thread_fixture=1", nil)
	c := e.NewContext(req, httptest.NewRecorder())
	if !messageThreadFixtureRequested(c) {
		t.Fatal("message_thread_fixture=1 should request replies fixture")
	}
}
