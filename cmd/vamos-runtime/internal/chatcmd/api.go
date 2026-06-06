package chatcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type APIClient interface {
	Start(ctx context.Context, keyID, secret string, req ChatStartRequest) (ChatAPIResponse, error)
	Steer(ctx context.Context, keyID, secret string, req ChatSteerRequest) (ChatAPIResponse, error)
	Snapshot(ctx context.Context, keyID, secret, sessionID string) (snapshotResponse, error)
	Events(ctx context.Context, keyID, secret, sessionID string, after int64) (*http.Response, error)
}

type HTTPAPIClient struct {
	HTTPClient *http.Client
	ManagerURL string
}

type ChatStartRequest struct {
	ProjectID string `json:"project_id"`
	Prompt    string `json:"prompt"`
}

type ChatSteerRequest struct {
	ThreadID string `json:"thread_id"`
	Prompt   string `json:"prompt"`
}

type ChatAPIResponse struct {
	Type             string     `json:"type"`
	Ref              ChatRunRef `json:"ref,omitempty"`
	Error            string     `json:"error,omitempty"`
	Reason           string     `json:"reason,omitempty"`
	LatestThreadID   string     `json:"latest_thread_id,omitempty"`
	LatestWebURL     string     `json:"latest_web_url,omitempty"`
	InfluencesLatest bool       `json:"influences_latest,omitempty"`
}

func (c HTTPAPIClient) Start(ctx context.Context, keyID, secret string, req ChatStartRequest) (ChatAPIResponse, error) {
	var out ChatAPIResponse
	err := c.doJSON(ctx, http.MethodPost, "/agent-chat/api/runs", keyID, secret, req, &out)
	return out, err
}

func (c HTTPAPIClient) Steer(ctx context.Context, keyID, secret string, req ChatSteerRequest) (ChatAPIResponse, error) {
	var out ChatAPIResponse
	err := c.doJSON(ctx, http.MethodPost, "/agent-chat/api/steer", keyID, secret, req, &out)
	return out, err
}

func (c HTTPAPIClient) Snapshot(ctx context.Context, keyID, secret, sessionID string) (snapshotResponse, error) {
	var out snapshotResponse
	err := c.doJSON(ctx, http.MethodGet, "/agent-chat/api/chat-sessions/"+url.PathEscape(sessionID), keyID, secret, nil, &out)
	return out, err
}

func (c HTTPAPIClient) Events(ctx context.Context, keyID, secret, sessionID string, after int64) (*http.Response, error) {
	path := "/agent-chat/api/chat-sessions/" + url.PathEscape(sessionID) + "/events"
	u, err := c.resolve(path)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("after", fmt.Sprint(after))
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	setMachineBearer(req, keyID, secret)
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("chat session SSE failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

func (c HTTPAPIClient) doJSON(ctx context.Context, method, path, keyID, secret string, in, out any) error {
	u, err := c.resolve(path)
	if err != nil {
		return err
	}
	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return err
	}
	setMachineBearer(req, keyID, secret)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("chat API failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c HTTPAPIClient) resolve(path string) (*url.URL, error) {
	base := strings.TrimRight(strings.TrimSpace(c.ManagerURL), "/")
	if base == "" {
		return nil, fmt.Errorf("manager URL is required")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("manager URL must be absolute")
	}
	return url.Parse(base + path)
}

func setMachineBearer(req *http.Request, keyID, secret string) {
	req.Header.Set("Authorization", "Bearer vamos_machine_"+strings.TrimSpace(keyID)+"."+strings.TrimSpace(secret))
}
