package safecomponent

import (
	"strings"
	"testing"
)

func TestValidateBoundedAcceptsOnlyNewComponentGrammar(t *testing.T) {
	for _, value := range []string{"a", "ABC_123-x", strings.Repeat("a", 128)} {
		if err := ValidateBounded(value); err != nil {
			t.Fatalf("ValidateBounded(%q): %v", value, err)
		}
	}
}

func TestValidateBoundedRejectsDotSeparatorsUnicodeAndOverlength(t *testing.T) {
	for _, value := range []string{"", ".", "..", "a/b", `a\\b`, "a b", "über", strings.Repeat("a", 129)} {
		if err := ValidateBounded(value); err == nil {
			t.Fatalf("ValidateBounded(%q) succeeded", value)
		}
	}
}
