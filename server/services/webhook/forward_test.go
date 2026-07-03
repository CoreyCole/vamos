package webhook

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestForwardPreservesBodyAndAllowedGitHubHeaders(t *testing.T) {
	body := []byte(`{"repository":{"full_name":"CoreyCole/vamos"}}`)
	inbound := http.Header{}
	inbound.Set(headerGitHubEvent, "push")
	inbound.Set(headerGitHubDelivery, "delivery-1")
	inbound.Set(headerHubSignature, "sha256=original")
	inbound.Set(headerContentType, "application/json")
	inbound.Set(headerUserAgent, "GitHub-Hookshot/test")
	inbound.Set("Host", "public.example")
	inbound.Set("Connection", "close")

	var gotBody []byte
	var gotHeader http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	result := Forward(t.Context(), ForwardRequest{URL: server.URL, Body: body, GitHubHeaders: inbound})
	if result.Error != "" || result.StatusCode != http.StatusAccepted {
		t.Fatalf("Forward() = %+v, want accepted without error", result)
	}
	if !bytes.Equal(gotBody, body) {
		t.Fatalf("forwarded body = %s, want %s", gotBody, body)
	}
	if gotHeader.Get(headerGitHubEvent) != "push" || gotHeader.Get(headerGitHubDelivery) != "delivery-1" || gotHeader.Get(headerHubSignature) != "sha256=original" {
		t.Fatalf("forwarded GitHub headers = %#v", gotHeader)
	}
	if gotHeader.Get(headerContentType) != "application/json" || gotHeader.Get(headerUserAgent) != "GitHub-Hookshot/test" {
		t.Fatalf("forwarded content headers = %#v", gotHeader)
	}
	if gotHeader.Get("Connection") != "" || gotHeader.Get("Host") != "" {
		t.Fatalf("unsafe headers copied: %#v", gotHeader)
	}
}

func TestForwardDefaultsContentType(t *testing.T) {
	var contentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get(headerContentType)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := Forward(t.Context(), ForwardRequest{URL: server.URL, Body: []byte(`{}`)})
	if result.Error != "" {
		t.Fatalf("Forward() error = %q", result.Error)
	}
	if contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
}

func TestForwardResignsWhenSecretConfigured(t *testing.T) {
	body := []byte(`{"zen":"keep it logically awesome"}`)
	inbound := http.Header{}
	inbound.Set(headerHubSignature, "sha256=original")

	var signature string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signature = r.Header.Get(headerHubSignature)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := Forward(t.Context(), ForwardRequest{URL: server.URL, Body: body, GitHubHeaders: inbound, Secret: "downstream"})
	if result.Error != "" {
		t.Fatalf("Forward() error = %q", result.Error)
	}
	if signature != SignGitHubWebhook(body, "downstream") {
		t.Fatalf("signature = %q, want re-signed", signature)
	}
}

func TestForwardRouteMatchesRepoAndEvent(t *testing.T) {
	route := ForwardRoute{
		GitHubRepos: map[string]bool{"coreycole/vamos": true},
		Events:      map[string]bool{"push": true},
	}
	if !route.Matches("CoreyCole/vamos", "push") {
		t.Fatal("Matches() = false, want true")
	}
	if route.Matches("premiumlabs/cn-agents", "push") {
		t.Fatal("Matches() = true for wrong repo")
	}
	if route.Matches("CoreyCole/vamos", "ping") {
		t.Fatal("Matches() = true for wrong event")
	}
}

func TestForwardRouteMatchesEmptyFilters(t *testing.T) {
	route := ForwardRoute{}
	if !route.Matches("CoreyCole/vamos", "push") {
		t.Fatal("Matches() with empty filters = false, want true")
	}
}

func TestForwardReportsTimeoutAndHTTPError(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer slow.Close()
	timeout := Forward(t.Context(), ForwardRequest{URL: slow.URL, Timeout: time.Millisecond})
	if timeout.Error == "" {
		t.Fatalf("Forward() timeout result = %+v, want error", timeout)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer failing.Close()
	result := Forward(t.Context(), ForwardRequest{URL: failing.URL})
	if result.StatusCode != http.StatusBadGateway || result.Error == "" {
		t.Fatalf("Forward() = %+v, want 502 error", result)
	}
}
