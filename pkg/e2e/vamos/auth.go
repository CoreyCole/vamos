package vamos

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/playwright-community/playwright-go"

	"github.com/CoreyCole/vamos/pkg/auth/agentbrowser"
	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"
)

type User struct{ Email string }

type robotActor struct{}

var Robot robotActor

func (robotActor) AuthStep() spec.Step {
	return AuthenticatedAs(User{Email: "playwright@localhost"})
}

func AuthenticatedAs(user any) spec.Step {
	email := userEmail(user)
	return spec.Custom("authenticated as "+email, func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		if err := Authenticate(context.Background(), ctx.Page, ctx.Config, email); err != nil {
			t.Fatal(err)
		}
	})
}

func userEmail(user any) string {
	switch value := user.(type) {
	case User:
		return value.Email
	case string:
		return value
	default:
		return fmt.Sprint(value)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func BuildAuthURL(cfg duiruntime.Config, redirect string) (string, error) {
	if redirect == "" {
		redirect = "/"
	}
	token := strings.TrimSpace(firstNonEmpty(os.Getenv("VAMOS_E2E_AUTH_TOKEN"), os.Getenv("VAMOS_PLAYWRIGHT_AUTH_TOKEN")))
	if token == "" {
		return "", errors.New("VAMOS_E2E_AUTH_TOKEN missing; run eval \"$(vamos auth playwright-env --slug <slug>)\"")
	}
	authURL, err := url.Parse(
		strings.TrimRight(cfg.BaseURL, "/") + "/internal/agent-auth/browser-login",
	)
	if err != nil {
		return "", err
	}
	q := authURL.Query()
	q.Set("purpose", string(agentbrowser.PurposeE2EPlaywright))
	q.Set("token", token)
	q.Set("redirect", redirect)
	authURL.RawQuery = q.Encode()
	return authURL.String(), nil
}

func Authenticate(
	ctx context.Context,
	page playwright.Page,
	cfg duiruntime.Config,
	email string,
) error {
	_ = email
	authURL, err := BuildAuthURL(cfg, "/")
	if err != nil {
		return err
	}
	_, err = page.Goto(
		authURL,
		playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded},
	)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	finalURL := page.URL()
	if strings.Contains(finalURL, "/login") ||
		strings.Contains(finalURL, "/internal/agent-auth/browser-login") {
		return fmt.Errorf("playwright auth failed; final URL: %s", finalURL)
	}
	return nil
}
