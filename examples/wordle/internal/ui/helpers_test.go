package ui

import "testing"

func TestTileAttrsAnimateOnlyMarkedTiles(t *testing.T) {
	attrs := tileAttrs(
		TileView{Index: 2, State: "correct", DelayMS: 200, Animation: "flip"},
	)
	if got := attrs["data-wordle-animation"]; got != "flip" {
		t.Fatalf("animation attr = %v, want flip", got)
	}
	if got := attrs["style"]; got != "animation-delay: 200ms" {
		t.Fatalf("style attr = %v, want animation-delay: 200ms", got)
	}

	attrs = tileAttrs(TileView{Index: 2, State: "correct"})
	if _, ok := attrs["data-wordle-animation"]; ok {
		t.Fatalf("old submitted tile unexpectedly has animation attr: %v", attrs)
	}
}

func TestRowAttrsUseTransientAnimation(t *testing.T) {
	attrs := rowAttrs(GuessRow{Current: true, Animation: "shake"})
	if got := attrs["data-wordle-row"]; got != "current" {
		t.Fatalf("row attr = %v, want current", got)
	}
	if got := attrs["data-wordle-animation"]; got != "shake" {
		t.Fatalf("animation attr = %v, want shake", got)
	}
}

func TestKeyAttrsAnimateDelayedKeys(t *testing.T) {
	attrs := keyAttrs(KeyboardKey{Value: "a", DelayMS: 100})
	if got := attrs["data-wordle-animation"]; got != "key" {
		t.Fatalf("animation attr = %v, want key", got)
	}
	if got := attrs["style"]; got != "animation-delay: 100ms" {
		t.Fatalf("style attr = %v, want animation-delay: 100ms", got)
	}
	if _, ok := attrs["data-on:click"]; !ok {
		t.Fatalf("letter key click attr missing: %v", attrs)
	}
}
