package authcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	purposeE2EPlaywright = "e2e_playwright"
	purposeHermesChat    = "hermes_chat"
	purposeVerify        = "verify"
)

type commandDeps struct {
	Store  CredentialStore
	Client AgentAuthClient
	Cwd    string
	Out    io.Writer
}

func NewCommand() *cobra.Command {
	return newCommand(commandDeps{})
}

func newCommand(deps commandDeps) *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Authenticate Vamos automation"}
	cmd.AddCommand(newLoginMachineCommand(deps))
	cmd.AddCommand(newStatusCommand(deps))
	cmd.AddCommand(newPlaywrightEnvCommand(deps))
	cmd.AddCommand(newCreateMachineKeyCommand(deps))
	cmd.AddCommand(newListMachineKeysCommand(deps))
	cmd.AddCommand(newRevokeMachineKeyCommand(deps))
	return cmd
}

func newLoginMachineCommand(deps commandDeps) *cobra.Command {
	var managerURL, keyID, secret, profileName string
	cmd := &cobra.Command{
		Use:   "login-machine --manager-url <url> --key-id <id> [--secret <secret>]",
		Short: "Store a manager-issued machine credential",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(secret) == "" {
				secret = os.Getenv("VAMOS_MACHINE_SECRET")
			}
			store, err := deps.credentialStore()
			if err != nil {
				return err
			}
			if err := store.Save(profileName, Profile{ManagerURL: managerURL, KeyID: keyID}, secret); err != nil {
				return err
			}
			_, err = fmt.Fprintf(deps.output(), "saved machine credential profile %q\n", normalizeProfileName(profileName))
			return err
		},
	}
	cmd.Flags().StringVar(&managerURL, "manager-url", "", "workspace manager URL")
	cmd.Flags().StringVar(&keyID, "key-id", "", "manager-issued machine credential id")
	cmd.Flags().StringVar(&secret, "secret", "", "manager-issued machine credential secret; defaults to VAMOS_MACHINE_SECRET")
	cmd.Flags().StringVar(&profileName, "profile", "default", "credential profile name")
	_ = cmd.MarkFlagRequired("manager-url")
	_ = cmd.MarkFlagRequired("key-id")
	return cmd
}

func newStatusCommand(deps commandDeps) *cobra.Command {
	var managerURL, profileName, slug string
	cmd := &cobra.Command{
		Use:   "status --slug <slug>",
		Short: "Verify the stored machine credential with the manager",
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, secret, resolvedManagerURL, err := deps.loadProfile(profileName, managerURL)
			if err != nil {
				return err
			}
			if strings.TrimSpace(slug) == "" {
				return errors.New("slug is required for manager credential status until a status endpoint exists")
			}
			client := deps.client(resolvedManagerURL)
			err = client.Status(cmd.Context(), profile.KeyID, secret, MintRequest{Slug: slug, Purpose: purposeVerify, RedirectPath: "/", TTLSeconds: 60})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(deps.output(), "ok")
			return err
		},
	}
	cmd.Flags().StringVar(&managerURL, "manager-url", "", "workspace manager URL")
	cmd.Flags().StringVar(&profileName, "profile", "default", "credential profile name")
	cmd.Flags().StringVar(&slug, "slug", "", "workspace slug to verify against")
	return cmd
}

func newPlaywrightEnvCommand(deps commandDeps) *cobra.Command {
	var managerURL, profileName, slug, email string
	var ttl time.Duration
	cmd := &cobra.Command{
		Use:   "playwright-env --slug <slug> [--email <email>]",
		Short: "Print shell exports for minted Playwright browser auth",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(slug) == "" {
				return errors.New("slug is required")
			}
			profile, secret, resolvedManagerURL, err := deps.loadProfile(profileName, managerURL)
			if err != nil {
				return err
			}
			if ttl <= 0 {
				ttl = 15 * time.Minute
			}
			resp, err := deps.client(resolvedManagerURL).MintBrowserToken(cmd.Context(), profile.KeyID, secret, MintRequest{
				Slug:         slug,
				Purpose:      purposeE2EPlaywright,
				Email:        email,
				RedirectPath: "/",
				TTLSeconds:   int64(ttl / time.Second),
			})
			if err != nil {
				return err
			}
			return PrintPlaywrightEnv(deps.output(), resp.Token)
		},
	}
	cmd.Flags().StringVar(&managerURL, "manager-url", "", "workspace manager URL")
	cmd.Flags().StringVar(&profileName, "profile", "default", "credential profile name")
	cmd.Flags().StringVar(&slug, "slug", "", "workspace slug")
	cmd.Flags().StringVar(&email, "email", "", "actor email for browser session")
	cmd.Flags().DurationVar(&ttl, "ttl", 15*time.Minute, "browser token TTL")
	return cmd
}

func PrintPlaywrightEnv(w io.Writer, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("browser auth token is required")
	}
	quoted := strconv.Quote(token)
	_, err := fmt.Fprintf(w, "export VAMOS_E2E_AUTH_TOKEN=%s\nexport VAMOS_PLAYWRIGHT_AUTH_TOKEN=%s\n", quoted, quoted)
	return err
}

func ResolveManagerURL(explicit, cwd string, profile Profile) (string, error) {
	if url := strings.TrimRight(strings.TrimSpace(explicit), "/"); url != "" {
		return url, nil
	}
	if url := strings.TrimRight(strings.TrimSpace(os.Getenv("VAMOS_WORKSPACE_MANAGER_URL")), "/"); url != "" {
		return url, nil
	}
	if url, ok := readWorkspaceEnv(cwd, "VAMOS_WORKSPACE_MANAGER_URL"); ok {
		return strings.TrimRight(url, "/"), nil
	}
	if url := strings.TrimRight(strings.TrimSpace(profile.ManagerURL), "/"); url != "" {
		return url, nil
	}
	return "", errors.New("manager URL required via --manager-url, VAMOS_WORKSPACE_MANAGER_URL, .vamos/run/workspace.env, or profile")
}

func readWorkspaceEnv(cwd, key string) (string, bool) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", false
		}
	}
	current, err := filepath.Abs(cwd)
	if err != nil {
		return "", false
	}
	for {
		path := filepath.Join(current, ".vamos", "run", "workspace.env")
		if value, ok := readEnvFileValue(path, key); ok {
			return value, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

func readEnvFileValue(path, key string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	prefix := key + "="
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		value = strings.Trim(value, "'\"")
		if value != "" {
			return value, true
		}
	}
	return "", false
}

func (d commandDeps) credentialStore() (CredentialStore, error) {
	if d.Store != nil {
		return d.Store, nil
	}
	path, err := DefaultCredentialPath()
	if err != nil {
		return nil, err
	}
	return FileCredentialStore{Path: path}, nil
}

func (d commandDeps) loadProfile(profileName, explicitManagerURL string) (Profile, string, string, error) {
	store, err := d.credentialStore()
	if err != nil {
		return Profile{}, "", "", err
	}
	profile, secret, err := store.Load(profileName)
	if err != nil {
		return Profile{}, "", "", err
	}
	managerURL, err := ResolveManagerURL(explicitManagerURL, d.cwd(), profile)
	if err != nil {
		return Profile{}, "", "", err
	}
	return profile, secret, managerURL, nil
}

func (d commandDeps) client(managerURL string) AgentAuthClient {
	if d.Client != nil {
		return d.Client
	}
	return Client{ManagerURL: managerURL}
}

func (d commandDeps) output() io.Writer {
	if d.Out != nil {
		return d.Out
	}
	return os.Stdout
}

func (d commandDeps) cwd() string {
	if strings.TrimSpace(d.Cwd) != "" {
		return d.Cwd
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}
