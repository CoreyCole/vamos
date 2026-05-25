package runtime

import "testing"

func TestResolveViewports(t *testing.T) {
	vps, err := ResolveViewports([]string{"mobile", "desktop-half", "desktop-full"})
	if err != nil {
		t.Fatalf("ResolveViewports() error = %v", err)
	}
	if len(vps) != 3 {
		t.Fatalf("viewports=%d want 3", len(vps))
	}
	if got, want := vps[0].Width, 390; got != want {
		t.Fatalf("mobile width=%d want %d", got, want)
	}
	if got, want := vps[1].Width, 900; got != want {
		t.Fatalf("desktop-half width=%d want %d", got, want)
	}
	if got, want := vps[2].Width, 1440; got != want {
		t.Fatalf("desktop-full width=%d want %d", got, want)
	}
}

func TestNewPageOptionsForViewportSendsViewportClassHeader(t *testing.T) {
	options := newPageOptionsForViewport(Viewport{Class: ViewportDesktopHalf, Width: 900, Height: 900})
	if options.Viewport == nil || options.Viewport.Width != 900 || options.Viewport.Height != 900 {
		t.Fatalf("viewport options = %#v", options.Viewport)
	}
	if got := options.ExtraHttpHeaders["X-Vamos-Viewport-Class"]; got != "desktop-half" {
		t.Fatalf("X-Vamos-Viewport-Class = %q, want desktop-half", got)
	}
}

func TestResolveViewportsRejectsUnknown(t *testing.T) {
	if _, err := ResolveViewports([]string{"watch"}); err == nil {
		t.Fatal("ResolveViewports() error = nil, want unknown viewport")
	}
}
