package verifycmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const e2ePlaywrightPurpose = "e2e_playwright"

type machineProfile struct {
	ManagerURL string `json:"manager_url"`
	KeyID      string `json:"key_id"`
}

type machineCredentialFile struct {
	Profiles map[string]storedMachineProfile `json:"profiles"`
}

type storedMachineProfile struct {
	ManagerURL string `json:"manager_url"`
	KeyID      string `json:"key_id"`
	Secret     string `json:"secret"`
}

type mintBrowserTokenRequest struct {
	Slug         string `json:"slug"`
	Purpose      string `json:"purpose"`
	Email        string `json:"email"`
	RedirectPath string `json:"redirect_path"`
	TTLSeconds   int64  `json:"ttl_seconds"`
}

type mintBrowserTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type e2eTokenMinter interface {
	MintE2EToken(ctx context.Context, cfg WorkspaceVerifyConfig) (string, error)
}

type managerE2ETokenMinter struct {
	HTTPClient *http.Client
}

func EnsureE2EAuthToken(ctx context.Context, cfg WorkspaceVerifyConfig) (string, error) {
	if token := strings.TrimSpace(firstNonEmpty(
		cfg.PlaywrightAuthToken,
		os.Getenv("VAMOS_E2E_AUTH_TOKEN"),
		os.Getenv("VAMOS_PLAYWRIGHT_AUTH_TOKEN"),
	)); token != "" {
		return token, nil
	}
	return managerE2ETokenMinter{}.MintE2EToken(ctx, cfg)
}

func (m managerE2ETokenMinter) MintE2EToken(ctx context.Context, cfg WorkspaceVerifyConfig) (string, error) {
	profile, secret, err := loadMachineProfile(cfg.MachineProfile)
	if err != nil {
		return "", fmt.Errorf("playwright auth token missing; run vamos auth create-machine-key on the manager, then vamos auth login-machine and eval \"$(vamos auth playwright-env --slug %s)\": %w", cfg.Slug, err)
	}
	managerURL := strings.TrimRight(strings.TrimSpace(firstNonEmpty(cfg.ManagerURL, profile.ManagerURL, cfg.BaseURL)), "/")
	if managerURL == "" {
		return "", errors.New("manager URL required to mint playwright auth token; pass --manager-url or run vamos auth login-machine")
	}
	if _, err := url.ParseRequestURI(managerURL); err != nil {
		return "", fmt.Errorf("invalid manager URL for playwright auth mint: %w", err)
	}
	body, err := json.Marshal(mintBrowserTokenRequest{
		Slug:         cfg.Slug,
		Purpose:      e2ePlaywrightPurpose,
		Email:        cfg.BrowserEmail,
		RedirectPath: "/",
		TTLSeconds:   int64((15 * time.Minute) / time.Second),
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, managerURL+"/internal/agent-auth/mint-browser-token", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer vamos_machine_"+strings.TrimSpace(profile.KeyID)+"."+strings.TrimSpace(secret))
	client := m.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("manager mint failed: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var out mintBrowserTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.Token) == "" {
		return "", errors.New("manager mint response missing token")
	}
	return out.Token, nil
}

func loadMachineProfile(profileName string) (machineProfile, string, error) {
	profileName = normalizeMachineProfileName(profileName)
	file, err := readMachineCredentialFile(defaultMachineCredentialPath())
	if err != nil {
		return machineProfile{}, "", err
	}
	stored, ok := file.Profiles[profileName]
	if !ok {
		return machineProfile{}, "", errors.New("machine credential profile not found")
	}
	profile := machineProfile{ManagerURL: strings.TrimRight(strings.TrimSpace(stored.ManagerURL), "/"), KeyID: strings.TrimSpace(stored.KeyID)}
	secret := strings.TrimSpace(stored.Secret)
	if profile.KeyID == "" || secret == "" {
		return machineProfile{}, "", errors.New("machine credential profile incomplete")
	}
	return profile, secret, nil
}

func defaultMachineCredentialPath() string {
	if dir := strings.TrimSpace(os.Getenv("VAMOS_CONFIG_HOME")); dir != "" {
		return filepath.Join(dir, "credentials.json")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".vamos", "credentials.json")
	}
	return "credentials.json"
}

func readMachineCredentialFile(path string) (machineCredentialFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return machineCredentialFile{}, err
	}
	var file machineCredentialFile
	if err := json.Unmarshal(data, &file); err != nil {
		return machineCredentialFile{}, err
	}
	if file.Profiles == nil {
		file.Profiles = map[string]storedMachineProfile{}
	}
	return file, nil
}

func normalizeMachineProfileName(profileName string) string {
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		return "default"
	}
	return profileName
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
