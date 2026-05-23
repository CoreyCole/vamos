//go:build !integration || unit
// +build !integration unit

package auth

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestLoginPageUsesConfiguredBranding(t *testing.T) {
	SetBranding(Branding{AppName: "ChestnutLLM", AccountLabel: "Chestnut account"})
	t.Cleanup(
		func() { SetBranding(Branding{AppName: "Vamos", AccountLabel: "your account"}) },
	)

	var buf bytes.Buffer
	if err := LoginPage(LoginPageArgs{}).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render LoginPage: %v", err)
	}
	body := buf.String()
	for _, want := range []string{"ChestnutLLM", "Sign in with your Chestnut account"} {
		if !strings.Contains(body, want) {
			t.Fatalf("LoginPage() missing %q: %s", want, body)
		}
	}
}
