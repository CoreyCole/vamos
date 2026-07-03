package appletruntime

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAliasRegistryMatchesExactAndMethodAliases(t *testing.T) {
	registry := NewAliasRegistry([]string{"/api", "/forms", "/thoughts", "/agent-chat", "/static"})
	if err := registry.Register(AliasRegistration{AppID: "wordle", Aliases: []RouteAlias{
		{Pattern: "/events", Methods: []string{http.MethodGet}},
		{Pattern: "/guesses", Methods: []string{http.MethodPost}},
	}}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	match, ok := registry.Match(httptest.NewRequest(http.MethodGet, "/events", nil))
	if !ok || match.AppID != "wordle" || !match.Alias {
		t.Fatalf("/events match = %+v, %v", match, ok)
	}
	match, ok = registry.Match(httptest.NewRequest(http.MethodPost, "/guesses", nil))
	if !ok || match.AppID != "wordle" || !match.Alias {
		t.Fatalf("/guesses match = %+v, %v", match, ok)
	}
	if _, ok := registry.Match(httptest.NewRequest(http.MethodGet, "/guesses", nil)); ok {
		t.Fatal("GET /guesses unexpectedly matched POST-only alias")
	}
	if _, ok := registry.Match(httptest.NewRequest(http.MethodGet, "/events/sub", nil)); ok {
		t.Fatal("exact /events alias unexpectedly matched child path")
	}
}

func TestAliasRegistryMatchesGlobAlias(t *testing.T) {
	registry := NewAliasRegistry([]string{"/api", "/forms", "/thoughts", "/agent-chat", "/static"})
	if err := registry.Register(AliasRegistration{AppID: "streamlit", Aliases: []RouteAlias{{Pattern: "/_stcore/*"}}}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	match, ok := registry.Match(httptest.NewRequest(http.MethodGet, "/_stcore/stream", nil))
	if !ok || match.AppID != "streamlit" {
		t.Fatalf("/_stcore/stream match = %+v, %v", match, ok)
	}
}

func TestAliasRegistryRejectsDuplicateAlias(t *testing.T) {
	registry := NewAliasRegistry(nil)
	if err := registry.Register(AliasRegistration{AppID: "wordle", Aliases: []RouteAlias{{Pattern: "/events"}}}); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}
	err := registry.Register(AliasRegistration{AppID: "other", Aliases: []RouteAlias{{Pattern: "/events"}}})
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate Register() error = %v", err)
	}
}

func TestValidateAliasConflictsRejectsReservedManagerRoute(t *testing.T) {
	err := ValidateAliasConflicts([]RouteAlias{{Pattern: "/static/*"}}, []string{"/static"})
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("reserved alias error = %v", err)
	}
}

func TestValidateAliasConflictsRejectsUnsafePatterns(t *testing.T) {
	tests := map[string][]RouteAlias{
		"relative":  {{Pattern: "events"}},
		"broad":     {{Pattern: "/*"}},
		"traversal": {{Pattern: "/../events"}},
		"wildcard":  {{Pattern: "/events/*/bad"}},
		"duplicate": {{Pattern: "/events"}, {Pattern: "/events"}},
	}
	for name, aliases := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateAliasConflicts(aliases, nil); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
