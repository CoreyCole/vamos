package authcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type AgentAuthClient interface {
	MintBrowserToken(ctx context.Context, keyID, secret string, req MintRequest) (MintResponse, error)
	Status(ctx context.Context, keyID, secret string, req MintRequest) error
}

type Client struct {
	HTTPClient *http.Client
	ManagerURL string
}

type MintRequest struct {
	Slug         string `json:"slug"`
	Purpose      string `json:"purpose"`
	Email        string `json:"email"`
	RedirectPath string `json:"redirect_path"`
	TTLSeconds   int64  `json:"ttl_seconds"`
}

type MintResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (c Client) MintBrowserToken(ctx context.Context, keyID, secret string, req MintRequest) (MintResponse, error) {
	managerURL := strings.TrimRight(strings.TrimSpace(c.ManagerURL), "/")
	if managerURL == "" {
		return MintResponse{}, fmt.Errorf("manager URL is required")
	}
	if _, err := url.ParseRequestURI(managerURL); err != nil {
		return MintResponse{}, fmt.Errorf("invalid manager URL: %w", err)
	}
	body, err := json.Marshal(req)
	if err != nil {
		return MintResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, managerURL+"/internal/agent-auth/mint-browser-token", bytes.NewReader(body))
	if err != nil {
		return MintResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer vamos_machine_"+strings.TrimSpace(keyID)+"."+strings.TrimSpace(secret))

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return MintResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return MintResponse{}, fmt.Errorf("manager mint failed: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var out MintResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return MintResponse{}, err
	}
	if strings.TrimSpace(out.Token) == "" {
		return MintResponse{}, fmt.Errorf("manager mint response missing token")
	}
	return out, nil
}

func (c Client) Status(ctx context.Context, keyID, secret string, req MintRequest) error {
	_, err := c.MintBrowserToken(ctx, keyID, secret, req)
	return err
}
