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

func TestResolveViewportsRejectsUnknown(t *testing.T) {
	if _, err := ResolveViewports([]string{"watch"}); err == nil {
		t.Fatal("ResolveViewports() error = nil, want unknown viewport")
	}
}
