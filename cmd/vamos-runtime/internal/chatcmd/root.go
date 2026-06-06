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

type StartOptions struct {
	ManagerURL string
	ProjectID  string
	Prompt     string
	Profile    string
	Cwd        string
	Timeout    time.Duration
}

type SteerOptions struct {
	ManagerURL string
	ThreadID   string
	Prompt     string
	Profile    string
	Cwd        string
	Timeout    time.Duration
}

type deps struct {
	Store        authcmd.CredentialStore
	APIClientNew func(managerURL string) APIClient

	// Retained for legacy browser helpers/tests outside the command surface.
	Client     authcmd.AgentAuthClient
	BrowserNew func(Options) (BrowserAutomation, error)
	WatcherNew func(*http.Client) CompletionWatcher
}

type CompletionWatcher interface {
	WatchUntilComplete(ctx context.Context, baseURL, sessionID string, afterSeq int64) (ChatCompletion, error)
}

func NewCommand() *cobra.Command { return newCommand(deps{}) }

func newCommand(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Start or steer Agent Chat runs",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return errors.New("use an explicit subcommand: start or steer")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("use an explicit subcommand: start or steer")
		},
	}
	cmd.AddCommand(newStartCommand(d), newSteerCommand(d))
	return cmd
}

func newStartCommand(d deps) *cobra.Command {
	opts := StartOptions{Timeout: 10 * time.Minute, Profile: "default"}
	cmd := &cobra.Command{
		Use:   "start --project <project_id> <prompt>",
		Short: "Start an Agent Chat run and stream NDJSON",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Prompt = strings.Join(args, " ")
			return RunStart(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.ManagerURL, "manager-url", "", "workspace manager URL")
	cmd.Flags().StringVar(&opts.ProjectID, "project", "", "configured project ID to run in")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 10*time.Minute, "maximum time to wait for chat completion")
	cmd.Flags().StringVar(&opts.Profile, "profile", "default", "credential profile name")
	return cmd
}

func newSteerCommand(d deps) *cobra.Command {
	opts := SteerOptions{Timeout: 10 * time.Minute, Profile: "default"}
	cmd := &cobra.Command{
		Use:   "steer --thread <thread_id> <guidance>",
		Short: "Start a follow-up run in an existing Agent Chat thread",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Prompt = strings.Join(args, " ")
			return RunSteer(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.ManagerURL, "manager-url", "", "workspace manager URL")
	cmd.Flags().StringVar(&opts.ThreadID, "thread", "", "Agent Chat thread ID to steer")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 10*time.Minute, "maximum time to wait for chat completion")
	cmd.Flags().StringVar(&opts.Profile, "profile", "default", "credential profile name")
	return cmd
}

func RunStart(ctx context.Context, opts StartOptions, d deps, out io.Writer) error {
	if strings.TrimSpace(opts.ProjectID) == "" {
		return errors.New("project is required")
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
	client := newAPIClient(d, managerURL)
	resp, err := client.Start(ctx, profile.KeyID, secret, ChatStartRequest{ProjectID: opts.ProjectID, Prompt: opts.Prompt})
	if err != nil {
		return err
	}
	if resp.Type != "started" || strings.TrimSpace(resp.Ref.ChatSessionID) == "" || strings.TrimSpace(resp.Ref.RunID) == "" {
		return fmt.Errorf("invalid start response")
	}
	if err := WriteNDJSON(out, ChatNDJSONEvent{Type: "started", Ref: resp.Ref}); err != nil {
		return err
	}
	watchCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	return StreamRunNDJSON(watchCtx, client, profile.KeyID, secret, resp.Ref, out)
}

func RunSteer(ctx context.Context, opts SteerOptions, d deps, out io.Writer) error {
	if strings.TrimSpace(opts.ThreadID) == "" {
		return errors.New("thread is required")
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
	client := newAPIClient(d, managerURL)
	resp, err := client.Steer(ctx, profile.KeyID, secret, ChatSteerRequest{ThreadID: opts.ThreadID, Prompt: opts.Prompt})
	if err != nil {
		return err
	}
	line := ChatNDJSONEvent{Type: resp.Type, Ref: resp.Ref, Error: resp.Error, Reason: resp.Reason, LatestThreadID: resp.LatestThreadID, LatestWebURL: resp.LatestWebURL, InfluencesLatest: resp.InfluencesLatest}
	if err := WriteNDJSON(out, line); err != nil {
		return err
	}
	if resp.Type != "steer_accepted" {
		return nil
	}
	watchCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	return StreamRunNDJSON(watchCtx, client, profile.KeyID, secret, resp.Ref, out)
}

func newAPIClient(d deps, managerURL string) APIClient {
	if d.APIClientNew != nil {
		return d.APIClientNew(managerURL)
	}
	return HTTPAPIClient{ManagerURL: managerURL}
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
