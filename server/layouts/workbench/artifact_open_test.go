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

func TestWriteArtifactOpenCookie(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	WriteArtifactOpenCookie(rec, true)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != ArtifactOpenCookie ||
		cookies[0].Value != "1" {
		t.Fatalf("cookies = %#v", cookies)
	}
}

func TestArtifactHideShowActionsWriteCookie(t *testing.T) {
	t.Parallel()
	hide := ArtifactHideClickAction()
	show := ArtifactShowClickAction()
	toggle := ArtifactToggleClickAction()
	for _, want := range []string{
		"workbenchV2Artifact.visible = false",
		ArtifactOpenCookie + "=0",
		"workbench-layout-reflow",
	} {
		if !strings.Contains(hide, want) {
			t.Fatalf("hide missing %q in %s", want, hide)
		}
	}
	for _, want := range []string{
		"workbenchV2Artifact.visible = true",
		ArtifactOpenCookie + "=1",
		"workbench-layout-reflow",
	} {
		if !strings.Contains(show, want) {
			t.Fatalf("show missing %q in %s", want, show)
		}
	}
	for _, action := range []string{hide, show} {
		if !strings.Contains(
			action,
			"workbenchApplyRegionVisible('workbench-v2-artifact'",
		) ||
			!strings.Contains(action, "workbenchReflow()") {
			t.Fatalf("details toggle must paint then reflow: %s", action)
		}
	}
	if !strings.Contains(toggle, "visible === false") {
		t.Fatalf("toggle = %q", toggle)
	}
}
