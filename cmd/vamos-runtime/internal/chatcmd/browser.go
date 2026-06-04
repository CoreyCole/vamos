package chatcmd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/playwright-community/playwright-go"
)

type BrowserAutomation interface {
	Login(ctx context.Context, loginURL string) error
	SubmitComposerPrompt(ctx context.Context, prompt string) (ChatRunRef, error)
	Cookies(ctx context.Context, baseURL string) ([]*http.Cookie, error)
	Close(ctx context.Context) error
}

type ChatRunRef struct {
	WorkspaceID   string
	ThreadID      string
	RunID         string
	ChatSessionID string
}

type PlaywrightBrowser struct {
	pw      *playwright.Playwright
	browser playwright.Browser
	context playwright.BrowserContext
	page    playwright.Page
}

func NewPlaywrightBrowser(opts Options) (BrowserAutomation, error) {
	pw, err := playwright.Run()
	if err != nil {
		return nil, err
	}
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(opts.Headless)})
	if err != nil {
		_ = pw.Stop()
		return nil, err
	}
	ctx, err := browser.NewContext()
	if err != nil {
		_ = browser.Close()
		_ = pw.Stop()
		return nil, err
	}
	page, err := ctx.NewPage()
	if err != nil {
		_ = ctx.Close()
		_ = browser.Close()
		_ = pw.Stop()
		return nil, err
	}
	return &PlaywrightBrowser{pw: pw, browser: browser, context: ctx, page: page}, nil
}

func (b *PlaywrightBrowser) Login(_ context.Context, loginURL string) error {
	_, err := b.page.Goto(loginURL, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded})
	return err
}

func (b *PlaywrightBrowser) SubmitComposerPrompt(_ context.Context, prompt string) (ChatRunRef, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ChatRunRef{}, fmt.Errorf("prompt is required")
	}
	input := b.page.Locator("#agent-chat-composer-input")
	if err := input.Fill(prompt); err != nil {
		return ChatRunRef{}, err
	}
	if _, err := b.page.Locator("#agent-chat-composer-form").Evaluate("el => el.requestSubmit()", nil); err != nil {
		return ChatRunRef{}, err
	}
	_ = b.page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{State: playwright.LoadStateDomcontentloaded})
	return chatRunRefFromURL(b.page.URL()), nil
}

func (b *PlaywrightBrowser) Cookies(_ context.Context, baseURL string) ([]*http.Cookie, error) {
	cookies, err := b.context.Cookies(baseURL)
	if err != nil {
		return nil, err
	}
	out := make([]*http.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		out = append(out, &http.Cookie{Name: cookie.Name, Value: cookie.Value, Path: cookie.Path, Domain: cookie.Domain})
	}
	return out, nil
}

func (b *PlaywrightBrowser) Close(_ context.Context) error {
	var err error
	if b.context != nil {
		err = b.context.Close()
	}
	if b.browser != nil {
		if closeErr := b.browser.Close(); err == nil {
			err = closeErr
		}
	}
	if b.pw != nil {
		if stopErr := b.pw.Stop(); err == nil {
			err = stopErr
		}
	}
	return err
}

func chatRunRefFromURL(raw string) ChatRunRef {
	u, err := url.Parse(raw)
	if err != nil {
		return ChatRunRef{}
	}
	ref := ChatRunRef{
		RunID:         strings.TrimSpace(u.Query().Get("run")),
		ChatSessionID: strings.TrimSpace(u.Query().Get("chat_session_id")),
		WorkspaceID:   strings.TrimSpace(u.Query().Get("workspace")),
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "thread" {
			ref.ThreadID = strings.TrimSpace(parts[i+1])
		}
	}
	if ref.ChatSessionID == "" {
		ref.ChatSessionID = ref.ThreadID
	}
	return ref
}
