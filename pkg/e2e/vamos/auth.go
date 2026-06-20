package vamos

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/playwright-community/playwright-go"

	"github.com/CoreyCole/vamos/pkg/auth/agentbrowser"
	"github.com/CoreyCole/vamos/pkg/ctl/verifycmd"
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

func authTokenForConfig(ctx context.Context, cfg duiruntime.Config) (string, error) {
	staticToken := strings.TrimSpace(firstNonEmpty(os.Getenv("VAMOS_E2E_AUTH_TOKEN"), os.Getenv("VAMOS_PLAYWRIGHT_AUTH_TOKEN")))
	if shouldMintFreshBrowserToken(cfg) {
		if token, err := verifycmd.MintFreshE2EAuthToken(ctx, workspaceVerifyConfigForAuth(cfg)); err == nil {
			return token, nil
		} else if staticToken == "" {
			return "", fmt.Errorf("fresh playwright auth token mint failed: %w", err)
		}
	}
	if staticToken != "" {
		return staticToken, nil
	}
	return "", errors.New("VAMOS_E2E_AUTH_TOKEN missing; run eval \"$(vamos auth playwright-env --slug <slug>)\"")
}

func shouldMintFreshBrowserToken(cfg duiruntime.Config) bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("VAMOS_E2E_AUTH_TOKEN_REFRESH")), "0") {
		return false
	}
	base, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	return err == nil && strings.EqualFold(base.Scheme, "https")
}

func workspaceVerifyConfigForAuth(cfg duiruntime.Config) verifycmd.WorkspaceVerifyConfig {
	return verifycmd.WorkspaceVerifyConfig{
		BaseURL:    strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		ManagerURL: strings.TrimSpace(os.Getenv("VAMOS_WORKSPACE_MANAGER_URL")),
		Slug:       firstNonEmpty(os.Getenv("VAMOS_E2E_WORKSPACE_SLUG"), os.Getenv("VAMOS_WORKSPACE_SLUG"), slugFromBaseURL(cfg.BaseURL), workspaceEnvValue("VAMOS_WORKSPACE_SLUG")),
		BrowserEmail: firstNonEmpty(
			os.Getenv("VAMOS_E2E_AUTH_EMAIL"),
			os.Getenv("VAMOS_PLAYWRIGHT_AUTH_EMAIL"),
			"playwright@localhost",
		),
	}
}

func slugFromBaseURL(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	if host == "" || host == "localhost" || strings.HasPrefix(host, "127.") {
		return ""
	}
	slug, _, _ := strings.Cut(host, ".")
	return strings.TrimSpace(slug)
}

func workspaceEnvValue(key string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		path := filepath.Join(cwd, ".vamos", "run", "workspace.env")
		if value := envFileValue(path, key); value != "" {
			return value
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return ""
		}
		cwd = parent
	}
}

func envFileValue(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	prefix := key + "="
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, prefix)), "'\"")
		}
	}
	return ""
}

func BuildAuthURL(cfg duiruntime.Config, redirect string) (string, error) {
	if redirect == "" {
		redirect = "/"
	}
	token, err := authTokenForConfig(context.Background(), cfg)
	if err != nil {
		return "", err
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
	response, err := page.Goto(
		authURL,
		playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded},
	)
	if err != nil {
		return err
	}
	if response == nil {
		return fmt.Errorf("playwright auth failed; no response from %s", authURL)
	}
	status := response.Status()
	if status < 200 || status >= 400 {
		hint := ""
		if strings.HasPrefix(strings.TrimSpace(cfg.BaseURL), "https://") && strings.TrimSpace(os.Getenv("VAMOS_E2E_AUTH_TOKEN")) == "" {
			hint = "; set VAMOS_E2E_AUTH_TOKEN for public workspace URLs"
		}
		return fmt.Errorf("playwright auth failed; %s returned HTTP %d%s", authURL, status, hint)
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
