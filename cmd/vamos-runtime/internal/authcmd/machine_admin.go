package authcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/CoreyCole/vamos/pkg/auth/agentbrowser"
	serverauth "github.com/CoreyCole/vamos/server/services/auth"
	dbsvc "github.com/CoreyCole/vamos/server/services/db"
)

type MachineKeyCreateOptions struct {
	DatabasePath       string
	ManagerURL         string
	Profile            string
	Name               string
	DefaultActorEmail  string
	AllowedActorEmails []string
	AllowedSlugs       []string
	AllowedPurposes    []string
	ExpiresAt          string
	SaveProfile        bool
}

type MachineKeyListOptions struct {
	DatabasePath string
}

type MachineKeyRevokeOptions struct {
	DatabasePath string
	KeyID        string
}

func newCreateMachineKeyCommand(deps commandDeps) *cobra.Command {
	opts := MachineKeyCreateOptions{Profile: "default"}
	cmd := &cobra.Command{
		Use:   "create-machine-key --database-path <manager-agents.db> --manager-url <url>",
		Short: "Create a manager machine key in the local manager database",
		RunE: func(cmd *cobra.Command, args []string) error {
			input, err := parseMachineCredentialInput(opts)
			if err != nil {
				return err
			}
			service, store, err := openMachineCredentialStore(cmd.Context(), opts.DatabasePath)
			if err != nil {
				return err
			}
			defer service.Close()
			created, err := store.Create(cmd.Context(), input)
			if err != nil {
				return err
			}
			if opts.SaveProfile {
				credentialStore, err := deps.credentialStore()
				if err != nil {
					return err
				}
				if err := credentialStore.Save(opts.Profile, Profile{ManagerURL: opts.ManagerURL, KeyID: created.Credential.ID}, created.Secret); err != nil {
					return err
				}
			}
			return printCreatedMachineCredential(deps.output(), opts.ManagerURL, opts.Profile, created)
		},
	}
	cmd.Flags().StringVar(&opts.DatabasePath, "database-path", "", "manager SQLite database path; defaults to VAMOS_DATABASE_PATH")
	cmd.Flags().StringVar(&opts.ManagerURL, "manager-url", "", "workspace manager URL for printed login command and optional saved profile")
	cmd.Flags().StringVar(&opts.Profile, "profile", "default", "credential profile name for printed/saved login")
	cmd.Flags().StringVar(&opts.Name, "name", "", "human label for this machine credential")
	cmd.Flags().StringVar(&opts.DefaultActorEmail, "email", "", "default actor email for browser sessions")
	cmd.Flags().StringArrayVar(&opts.AllowedActorEmails, "actor-email", nil, "allowed actor email; repeatable")
	cmd.Flags().StringArrayVar(&opts.AllowedSlugs, "slug", nil, "allowed workspace slug; repeatable")
	cmd.Flags().StringArrayVar(&opts.AllowedPurposes, "purpose", nil, "allowed purpose; repeatable: e2e_playwright, hermes_chat, verify")
	cmd.Flags().StringVar(&opts.ExpiresAt, "expires-at", "", "optional RFC3339 expiry time")
	cmd.Flags().BoolVar(&opts.SaveProfile, "save-profile", false, "also save the created credential to the local CLI profile")
	_ = cmd.MarkFlagRequired("manager-url")
	return cmd
}

func newListMachineKeysCommand(deps commandDeps) *cobra.Command {
	opts := MachineKeyListOptions{}
	cmd := &cobra.Command{
		Use:   "list-machine-keys --database-path <manager-agents.db>",
		Short: "List manager machine keys from the local manager database",
		RunE: func(cmd *cobra.Command, args []string) error {
			service, store, err := openMachineCredentialStore(cmd.Context(), opts.DatabasePath)
			if err != nil {
				return err
			}
			defer service.Close()
			credentials, err := store.List(cmd.Context())
			if err != nil {
				return err
			}
			return printMachineCredentials(deps.output(), credentials)
		},
	}
	cmd.Flags().StringVar(&opts.DatabasePath, "database-path", "", "manager SQLite database path; defaults to VAMOS_DATABASE_PATH")
	return cmd
}

func newRevokeMachineKeyCommand(deps commandDeps) *cobra.Command {
	opts := MachineKeyRevokeOptions{}
	cmd := &cobra.Command{
		Use:   "revoke-machine-key --database-path <manager-agents.db> --key-id <id>",
		Short: "Revoke a manager machine key in the local manager database",
		RunE: func(cmd *cobra.Command, args []string) error {
			service, store, err := openMachineCredentialStore(cmd.Context(), opts.DatabasePath)
			if err != nil {
				return err
			}
			defer service.Close()
			if err := store.Revoke(cmd.Context(), opts.KeyID); err != nil {
				return err
			}
			_, err = fmt.Fprintf(deps.output(), "revoked machine credential %s\n", strings.TrimSpace(opts.KeyID))
			return err
		},
	}
	cmd.Flags().StringVar(&opts.DatabasePath, "database-path", "", "manager SQLite database path; defaults to VAMOS_DATABASE_PATH")
	cmd.Flags().StringVar(&opts.KeyID, "key-id", "", "machine credential id to revoke")
	_ = cmd.MarkFlagRequired("key-id")
	return cmd
}

func openMachineCredentialStore(ctx context.Context, databasePath string) (*dbsvc.Service, serverauth.MachineCredentialStore, error) {
	_ = ctx // retained for future context-aware open; NewService is currently synchronous.
	resolved, err := resolveMachineKeyDatabasePath(databasePath)
	if err != nil {
		return nil, nil, err
	}
	service, err := dbsvc.NewService(resolved)
	if err != nil {
		return nil, nil, err
	}
	return service, serverauth.NewSQLMachineCredentialStore(service.Queries), nil
}

func resolveMachineKeyDatabasePath(explicit string) (string, error) {
	if path := strings.TrimSpace(explicit); path != "" {
		return path, nil
	}
	if path := strings.TrimSpace(os.Getenv("VAMOS_DATABASE_PATH")); path != "" {
		return path, nil
	}
	return "", errors.New("manager database path required via --database-path or VAMOS_DATABASE_PATH")
}

func parseMachineCredentialInput(opts MachineKeyCreateOptions) (serverauth.CreateMachineCredentialInput, error) {
	purposes := make([]agentbrowser.Purpose, 0, len(opts.AllowedPurposes))
	for _, raw := range opts.AllowedPurposes {
		purpose, err := parseMachineCredentialPurpose(raw)
		if err != nil {
			return serverauth.CreateMachineCredentialInput{}, err
		}
		purposes = append(purposes, purpose)
	}
	expiresAt, err := parseMachineCredentialExpiry(opts.ExpiresAt)
	if err != nil {
		return serverauth.CreateMachineCredentialInput{}, err
	}
	return serverauth.CreateMachineCredentialInput{
		Name:               opts.Name,
		DefaultActorEmail:  opts.DefaultActorEmail,
		AllowedActorEmails: opts.AllowedActorEmails,
		AllowedSlugs:       opts.AllowedSlugs,
		AllowedPurposes:    purposes,
		ExpiresAt:          expiresAt,
	}, nil
}

func parseMachineCredentialPurpose(raw string) (agentbrowser.Purpose, error) {
	switch strings.TrimSpace(raw) {
	case purposeE2EPlaywright:
		return agentbrowser.PurposeE2EPlaywright, nil
	case purposeHermesChat:
		return agentbrowser.PurposeHermesChat, nil
	case purposeVerify:
		return agentbrowser.PurposeVerify, nil
	case "":
		return "", errors.New("purpose is required")
	default:
		return "", fmt.Errorf("unsupported purpose %q; expected e2e_playwright, hermes_chat, or verify", raw)
	}
}

func parseMachineCredentialExpiry(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, fmt.Errorf("expires-at must be RFC3339: %w", err)
	}
	return &parsed, nil
}

func printCreatedMachineCredential(w io.Writer, managerURL, profile string, created serverauth.CreatedMachineCredential) error {
	_, err := fmt.Fprintf(w, `created machine credential
key_id: %s
secret: %s

Store on client:
vamos auth login-machine --manager-url %s --key-id %s --secret %s --profile %s
`, created.Credential.ID, created.Secret, strings.TrimRight(strings.TrimSpace(managerURL), "/"), created.Credential.ID, created.Secret, normalizeProfileName(profile))
	return err
}

func printMachineCredentials(w io.Writer, credentials []serverauth.MachineCredential) error {
	if len(credentials) == 0 {
		_, err := fmt.Fprintln(w, "no machine credentials")
		return err
	}
	for _, credential := range credentials {
		revoked := ""
		if credential.RevokedAt != nil {
			revoked = credential.RevokedAt.Format(time.RFC3339)
		}
		lastUsed := ""
		if credential.LastUsedAt != nil {
			lastUsed = credential.LastUsedAt.Format(time.RFC3339)
		}
		expires := ""
		if credential.ExpiresAt != nil {
			expires = credential.ExpiresAt.Format(time.RFC3339)
		}
		if _, err := fmt.Fprintf(w, "%s\tname=%q\temail=%q\tslugs=%s\tpurposes=%s\tcreated=%s\tlast_used=%s\texpires=%s\trevoked=%s\n",
			credential.ID,
			credential.Name,
			credential.DefaultActorEmail,
			strings.Join(credential.AllowedSlugs, ","),
			joinPurposes(credential.AllowedPurposes),
			credential.CreatedAt.Format(time.RFC3339),
			lastUsed,
			expires,
			revoked,
		); err != nil {
			return err
		}
	}
	return nil
}

func joinPurposes(values []agentbrowser.Purpose) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return strings.Join(out, ",")
}
