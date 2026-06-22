package qrspicmd

import "testing"

func TestOutputTailFiltersCobraUsage(t *testing.T) {
	input := []byte("starting child\nError: unknown option --session-id\nUsage:\n  pi [flags]\n\nFlags:\n  --session string\n  --session-dir string\nGlobal Flags:\n  --help\n")
	got := FilterChildOutputTail(input, 8)
	if !containsLine(got, "starting child") || !containsLine(got, "Error: unknown option --session-id") {
		t.Fatalf("tail = %#v, want useful lines", got)
	}
	for _, forbidden := range []string{"Usage:", "pi [flags]", "Flags:", "--session string", "Global Flags:", "--help"} {
		if containsLine(got, forbidden) {
			t.Fatalf("tail = %#v contains usage line %q", got, forbidden)
		}
	}
}

func TestOutputTailLimitsUsefulLines(t *testing.T) {
	got := FilterChildOutputTail([]byte("one\ntwo\nthree\nfour\n"), 2)
	want := []string{"three", "four"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("tail = %#v, want %#v", got, want)
	}
}
