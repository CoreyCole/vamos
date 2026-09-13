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
	}{
		{"density_fixture=1", true, false, false},
		{"fixture=density", true, false, false},
		{"fixture=Density", true, false, false},
		{"group_bubble_fixture=1", false, true, false},
		{"fixture=group", false, true, false},
		{"pairwise_fixture=1", false, false, true},
		{"fixture=pairwise", false, false, true},
		{"", false, false, false},
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
	}
}
