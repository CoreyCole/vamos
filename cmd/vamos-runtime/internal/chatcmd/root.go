package chatcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/CoreyCole/vamos/cmd/vamos-runtime/internal/authcmd"
)

const purposeHermesChat = "hermes_chat"

type Options struct {
	ManagerURL string
	Slug       string
	Email      string
	Prompt     string
	Timeout    time.Duration
	Headless   bool
	Profile    string
	BaseURL    string
	Cwd        string
}

type deps struct {
	Store      authcmd.CredentialStore
	Client     authcmd.AgentAuthClient
	BrowserNew func(Options) (BrowserAutomation, error)
	WatcherNew func(*http.Client) CompletionWatcher
}

type CompletionWatcher interface {
	WatchUntilComplete(ctx context.Context, baseURL, sessionID string, afterSeq int64) (ChatCompletion, error)
}

func NewCommand() *cobra.Command { return newCommand(deps{}) }

func newCommand(d deps) *cobra.Command {
	opts := Options{Timeout: 10 * time.Minute, Headless: true, Profile: "default"}
	cmd := &cobra.Command{
		Use:   "chat --slug <slug> --email <email> <prompt>",
		Short: "Send a prompt through Agent Chat UI and wait for SSE completion",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Prompt = strings.Join(args, " ")
			return Run(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.ManagerURL, "manager-url", "", "workspace manager URL")
	cmd.Flags().StringVar(&opts.Slug, "slug", "", "workspace slug")
	cmd.Flags().StringVar(&opts.Email, "email", "", "actor email for browser session")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 10*time.Minute, "maximum time to wait for chat completion")
	cmd.Flags().BoolVar(&opts.Headless, "headless", true, "run browser headlessly")
	cmd.Flags().StringVar(&opts.Profile, "profile", "default", "credential profile name")
	cmd.Flags().StringVar(&opts.BaseURL, "base-url", "", "workspace base URL; defaults to slug under manager domain")
	return cmd
}

func Run(ctx context.Context, opts Options, d deps, out io.Writer) error {
	if strings.TrimSpace(opts.Slug) == "" {
		return errors.New("slug is required")
	}
	if strings.TrimSpace(opts.Email) == "" {
		return errors.New("email is required")
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		return errors.New("prompt is required")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Minute
	}
	if out == nil {
		out = os.Stdout
	}

	profile, secret, err := loadProfile(d.Store, opts.Profile)
	if err != nil {
		return err
	}
	managerURL, err := authcmd.ResolveManagerURL(opts.ManagerURL, opts.Cwd, profile)
	if err != nil {
		return err
	}
	baseURL, err := resolveBaseURL(opts.BaseURL, managerURL, opts.Slug)
	if err != nil {
		return err
	}
	client := d.Client
	if client == nil {
		client = authcmd.Client{ManagerURL: managerURL}
	}
	minted, err := client.MintBrowserToken(ctx, profile.KeyID, secret, authcmd.MintRequest{
		Slug:         opts.Slug,
		Purpose:      purposeHermesChat,
		Email:        opts.Email,
		RedirectPath: "/agent-chat",
		TTLSeconds:   int64((5 * time.Minute) / time.Second),
	})
	if err != nil {
		return err
	}
	loginURL, err := agentBrowserLoginURL(baseURL, minted.Token, "/agent-chat")
	if err != nil {
		return err
	}

	browserNew := d.BrowserNew
	if browserNew == nil {
		browserNew = NewPlaywrightBrowser
	}
	browser, err := browserNew(opts)
	if err != nil {
		return err
	}
	defer browser.Close(context.Background())
	if err := browser.Login(ctx, loginURL); err != nil {
		return err
	}
	runRef, err := browser.SubmitComposerPrompt(ctx, opts.Prompt)
	if err != nil {
		return err
	}
	if strings.TrimSpace(runRef.ChatSessionID) == "" {
		return fmt.Errorf("chat session id was not discoverable after UI submit")
	}
	cookies, err := browser.Cookies(ctx, baseURL)
	if err != nil {
		return err
	}
	watcherClient := httpClientWithCookies(cookies)
	watcherNew := d.WatcherNew
	if watcherNew == nil {
		watcherNew = func(client *http.Client) CompletionWatcher { return ChatSessionWatcher{HTTPClient: client} }
	}
	watchCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	completion, err := watcherNew(watcherClient).WatchUntilComplete(watchCtx, baseURL, runRef.ChatSessionID, 0)
	if err != nil {
		return err
	}
	if completion.Failed {
		return fmt.Errorf("chat run failed: %s", completion.Error)
	}
	_, err = fmt.Fprintln(out, completion.Response)
	return err
}

func loadProfile(store authcmd.CredentialStore, profileName string) (authcmd.Profile, string, error) {
	if store == nil {
		path, err := authcmd.DefaultCredentialPath()
		if err != nil {
			return authcmd.Profile{}, "", err
		}
		store = authcmd.FileCredentialStore{Path: path}
	}
	return store.Load(profileName)
}

func resolveBaseURL(explicit, managerURL, slug string) (string, error) {
	if base := strings.TrimRight(strings.TrimSpace(explicit), "/"); base != "" {
		return base, nil
	}
	u, err := url.Parse(strings.TrimSpace(managerURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("base URL required when manager URL is not absolute")
	}
	host := u.Hostname()
	port := u.Port()
	parts := strings.Split(host, ".")
	if len(parts) > 1 {
		parts[0] = strings.TrimSpace(slug)
		host = strings.Join(parts, ".")
	} else {
		host = strings.TrimSpace(slug) + "." + host
	}
	if port != "" {
		host += ":" + port
	}
	return (&url.URL{Scheme: u.Scheme, Host: host}).String(), nil
}

func agentBrowserLoginURL(baseURL, token, redirect string) (string, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + "/internal/agent-auth/browser-login")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("purpose", purposeHermesChat)
	q.Set("token", token)
	q.Set("redirect", redirect)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
