package workbench

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestArtifactOpenFromRequest(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if !ArtifactOpenFromRequest(req) {
		t.Fatal("missing cookie should default open")
	}
	req.AddCookie(&http.Cookie{Name: ArtifactOpenCookie, Value: "0"})
	if ArtifactOpenFromRequest(req) {
		t.Fatal("cookie=0 should close")
	}
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(&http.Cookie{Name: ArtifactOpenCookie, Value: "1"})
	if !ArtifactOpenFromRequest(req2) {
		t.Fatal("cookie=1 should open")
	}
}

func TestArtifactHideShowActionsWriteCookie(t *testing.T) {
	t.Parallel()
	hide := ArtifactHideClickAction()
	show := ArtifactShowClickAction()
	for _, want := range []string{
		"workbenchV2Artifact.visible = false",
		ArtifactOpenCookie + "=0",
	} {
		if !strings.Contains(hide, want) {
			t.Fatalf("hide missing %q in %s", want, hide)
		}
	}
	for _, want := range []string{
		"workbenchV2Artifact.visible = true",
		ArtifactOpenCookie + "=1",
	} {
		if !strings.Contains(show, want) {
			t.Fatalf("show missing %q in %s", want, show)
		}
	}
}
