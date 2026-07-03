package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	defaultForwardTimeout = 15 * time.Second
	headerContentType     = "Content-Type"
	headerGitHubDelivery  = "X-GitHub-Delivery"
	headerGitHubEvent     = "X-GitHub-Event"
	headerHubSignature    = "X-Hub-Signature-256"
	headerUserAgent       = "User-Agent"
)

type ForwardRoute struct {
	URL         string
	GitHubRepos map[string]bool
	Events      map[string]bool
	Secret      string
	Timeout     time.Duration
	BestEffort  bool
}

type ForwardRequest struct {
	URL           string
	Body          []byte
	GitHubHeaders http.Header
	Secret        string
	Timeout       time.Duration
	BestEffort    bool
}

type ForwardResult struct {
	URL        string
	StatusCode int
	Error      string
	Duration   time.Duration
}

func (r ForwardRoute) Matches(repository, eventType string) bool {
	repo := strings.ToLower(strings.TrimSpace(repository))
	event := strings.ToLower(strings.TrimSpace(eventType))
	if len(r.GitHubRepos) > 0 && !r.GitHubRepos[repo] {
		return false
	}
	if len(r.Events) > 0 && !r.Events[event] {
		return false
	}
	return true
}

func Forward(ctx context.Context, req ForwardRequest) ForwardResult {
	start := time.Now()
	result := ForwardResult{URL: req.URL}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = defaultForwardTimeout
	}
	forwardCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(forwardCtx, http.MethodPost, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}
	copyAllowedGitHubHeaders(httpReq.Header, req.GitHubHeaders)
	if strings.TrimSpace(req.Secret) != "" {
		httpReq.Header.Set(headerHubSignature, SignGitHubWebhook(req.Body, req.Secret))
	}
	if httpReq.Header.Get(headerContentType) == "" {
		httpReq.Header.Set(headerContentType, "application/json")
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Error = fmt.Sprintf("forward returned HTTP %d", resp.StatusCode)
	}
	result.Duration = time.Since(start)
	return result
}

func copyAllowedGitHubHeaders(dst, src http.Header) {
	for _, name := range []string{
		headerContentType,
		headerGitHubDelivery,
		headerGitHubEvent,
		headerHubSignature,
		headerUserAgent,
	} {
		for _, value := range src.Values(name) {
			dst.Add(name, value)
		}
	}
}

func SignGitHubWebhook(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
