package runtime

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/playwright-community/playwright-go"
)

func BuildAuthURL(cfg Config, redirect string) (string, error) {
	if redirect == "" {
		redirect = "/"
	}
	authURL, err := url.Parse(
		strings.TrimRight(cfg.BaseURL, "/") + "/internal/playwright-auth",
	)
	if err != nil {
		return "", err
	}
	q := authURL.Query()
	q.Set("redirect", redirect)
	if cfg.AuthToken != "" {
		q.Set("token", cfg.AuthToken)
	}
	authURL.RawQuery = q.Encode()
	return authURL.String(), nil
}

func Authenticate(
	ctx context.Context,
	page playwright.Page,
	cfg Config,
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
		strings.Contains(finalURL, "/internal/playwright-auth") {
		return fmt.Errorf("playwright auth failed; final URL: %s", finalURL)
	}
	return nil
}
