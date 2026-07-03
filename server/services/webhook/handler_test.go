package webhook

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestHandlerForwardsGitHubHeadersAfterVerification(t *testing.T) {
	t.Parallel()

	body := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"CoreyCole/vamos"}}`)
	signature := SignGitHubWebhook(body, "secret")
	var gotSignature string
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSignature = r.Header.Get("X-Hub-Signature-256")
		w.WriteHeader(http.StatusOK)
	}))
	defer downstream.Close()

	svc := NewServiceWithRoutes("secret", RepoRoute{}, nil, []ForwardRoute{{
		URL:         downstream.URL,
		GitHubRepos: map[string]bool{"coreycole/vamos": true},
		Events:      map[string]bool{"push": true},
		BestEffort:  true,
	}})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", signature)
	rec := httptest.NewRecorder()

	err := NewHandler(svc).HandleGitHubWebhook(e.NewContext(req, rec))
	if err != nil {
		t.Fatalf("HandleGitHubWebhook() error = %v", err)
	}
	if rec.Code != http.StatusOK || gotSignature != signature {
		t.Fatalf("code=%d signature=%q want 200 original signature", rec.Code, gotSignature)
	}
}

func TestHandlerReturnsSuccessEnvelopeForBestEffortForwardFailure(t *testing.T) {
	t.Parallel()

	body := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"CoreyCole/vamos"}}`)
	signature := SignGitHubWebhook(body, "secret")
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer downstream.Close()

	svc := NewServiceWithRoutes("secret", RepoRoute{}, nil, []ForwardRoute{{
		URL:         downstream.URL,
		GitHubRepos: map[string]bool{"coreycole/vamos": true},
		Events:      map[string]bool{"push": true},
		BestEffort:  true,
	}})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", signature)
	rec := httptest.NewRecorder()

	if err := NewHandler(svc).HandleGitHubWebhook(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleGitHubWebhook() error = %v", err)
	}
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"status":"success"`)) {
		t.Fatalf("response = code %d body %s, want success envelope", rec.Code, rec.Body.String())
	}
}

func TestHandlerReturnsErrorEnvelopeForRequiredForwardFailure(t *testing.T) {
	t.Parallel()

	body := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"CoreyCole/vamos"}}`)
	signature := SignGitHubWebhook(body, "secret")
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer downstream.Close()

	svc := NewServiceWithRoutes("secret", RepoRoute{}, nil, []ForwardRoute{{
		URL:         downstream.URL,
		GitHubRepos: map[string]bool{"coreycole/vamos": true},
		Events:      map[string]bool{"push": true},
		BestEffort:  false,
	}})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", signature)
	rec := httptest.NewRecorder()

	if err := NewHandler(svc).HandleGitHubWebhook(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleGitHubWebhook() error = %v", err)
	}
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"status":"error"`)) {
		t.Fatalf("response = code %d body %s, want error envelope", rec.Code, rec.Body.String())
	}
}
