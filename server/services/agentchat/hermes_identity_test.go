package agentchat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveHermesPlanIdentityConvergesLegacyAndUnicodeFilesystemHints(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "Renée", "plans", "über")
	if err := os.MkdirAll(plan, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, hint := range []string{
		"Renée/plans/über",
		"thoughts/Renée/plans/über",
		"Rene\u0301e/plans/u\u0308ber",
	} {
		identity, resolved, err := ResolveHermesPlanIdentity(root, plan, hint)
		if err != nil {
			t.Fatalf("hint %q: %v", hint, err)
		}
		resolvedInfo, statErr := os.Stat(resolved)
		if statErr != nil {
			t.Fatal(statErr)
		}
		planInfo, statErr := os.Stat(plan)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if identity != "Renée/plans/über" || !os.SameFile(resolvedInfo, planInfo) {
			t.Fatalf("hint %q produced %q, %q", hint, identity, resolved)
		}
	}
}

func TestResolveHermesPlanIdentityRejectsDifferentContainedHint(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "owner", "plans", "first")
	second := filepath.Join(root, "owner", "plans", "second")
	if err := os.MkdirAll(first, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(second, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ResolveHermesPlanIdentity(root, first, second); err == nil {
		t.Fatal("different contained hint was accepted")
	}
}

func TestValidateHermesPlanIdentityRejectsEveryNoncanonicalWireForm(t *testing.T) {
	invalid := []string{
		"thoughts/CoreyCole/plans/alpha", "/CoreyCole/plans/alpha",
		"C:/CoreyCole/plans/alpha", "CoreyCole//plans/alpha",
		"CoreyCole/plans/alpha/", "CoreyCole/./plans/alpha",
		"CoreyCole/../plans/alpha", `CoreyCole\plans\alpha`,
		"Rene\u0301e/plans/u\u0308ber", "CoreyCole/plans/a\x00b",
		"CoreyCole/plans/a\nb", "CoreyCole/plans/a\x7fb", string([]byte{0xff}),
	}
	for _, value := range invalid {
		if err := ValidateHermesPlanIdentity(HermesPlanIdentity(value)); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}

func TestHermesConversationReferenceGoldenVectors(t *testing.T) {
	tests := []struct{ plan, thread, want string }{
		{"CoreyCole/plans/alpha", "0123456789abcdef0123456789abcdef", "vhc1_4a52229d0cc15725147136839692036a31e760c3a4b2f1fb9c6cc5b6280ff89b"},
		{"CoreyCole/plans/beta", "0123456789abcdef0123456789abcdef", "vhc1_ae93205129efceb16945a0a47126651724065016825a89ca2db47111b4519111"},
		{"Renée/plans/über", "thread_1", "vhc1_c1807b711125f62fbc87fe666d4b1db3224886c5552be11450422e7a27f84470"},
	}
	for _, test := range tests {
		got, err := HermesConversationReference(HermesPlanIdentity(test.plan), test.thread)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("reference(%q, %q) = %q, want %q", test.plan, test.thread, got, test.want)
		}
		if err := ValidateHermesConversationReference(got); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHermesConversationReferenceSeparatesEqualThreadIDsAcrossPlans(t *testing.T) {
	first, _ := HermesConversationReference("owner/plans/first", "same")
	second, _ := HermesConversationReference("owner/plans/second", "same")
	if first == second {
		t.Fatal("equal thread IDs in different plans produced the same reference")
	}
}

func TestResolveHermesPlanIdentityRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ResolveHermesPlanIdentity(root, link, ""); err == nil {
		t.Fatal("symlink escape was accepted")
	}
}
