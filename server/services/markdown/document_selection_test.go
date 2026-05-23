package markdown

import "testing"

func TestCanonicalThoughtsDocPathRejectsTraversal(t *testing.T) {
	for _, input := range []string{"", "../secret.md", "thoughts/../secret.md", "/etc/passwd"} {
		if got, err := CanonicalThoughtsDocPath(input); err == nil {
			t.Fatalf("CanonicalThoughtsDocPath(%q) = %q, want error", input, got)
		}
	}
}

func TestCanonicalThoughtsDocPathStripsSingleThoughtsPrefix(t *testing.T) {
	got, err := CanonicalThoughtsDocPath("thoughts/CoreyCole/plans/demo.md")
	if err != nil {
		t.Fatal(err)
	}
	if got != "CoreyCole/plans/demo.md" {
		t.Fatalf("CanonicalThoughtsDocPath() = %q", got)
	}
}

func TestThoughtsDocURLAvoidsDoubleThoughtsPrefix(t *testing.T) {
	got := ThoughtsDocURL("thoughts/CoreyCole/plans/demo.md", "intro")
	if got != "/thoughts/CoreyCole/plans/demo.md#intro" {
		t.Fatalf("ThoughtsDocURL() = %q", got)
	}
}

func TestCanonicalThoughtsDirPathRejectsTraversal(t *testing.T) {
	for _, input := range []string{"../secret", "thoughts/../secret", "/etc"} {
		if got, err := CanonicalThoughtsDirPath(input); err == nil {
			t.Fatalf("CanonicalThoughtsDirPath(%q) = %q, want error", input, got)
		}
	}
}

func TestCanonicalThoughtsDirPathAllowsRoot(t *testing.T) {
	for _, input := range []string{"", ".", "/thoughts/"} {
		got, err := CanonicalThoughtsDirPath(input)
		if err != nil {
			t.Fatalf("CanonicalThoughtsDirPath(%q) error = %v", input, err)
		}
		if got != "" {
			t.Fatalf("CanonicalThoughtsDirPath(%q) = %q, want root", input, got)
		}
	}
}

func TestThoughtsDirURLAvoidsDoubleThoughtsPrefix(t *testing.T) {
	got := ThoughtsDirURL("thoughts/CoreyCole/plans")
	if got != "/thoughts/CoreyCole/plans" {
		t.Fatalf("ThoughtsDirURL() = %q", got)
	}
}
