package agentchat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type HermesPrompt struct {
	ThreadID     string   `json:"thread_id"`
	OwnerEmail   string   `json:"owner_email"`
	PlanDir      string   `json:"plan_dir,omitempty"`
	ContextPaths []string `json:"context_paths,omitempty"`
	Prompt       string   `json:"prompt"`
}
type HermesCallbackEvent struct {
	PlanDir string `json:"plan_dir"`
	HermesTranscriptEvent
}

type HermesGatewayClient interface {
	DeliverPrompt(context.Context, HermesPrompt) error
	DeliverPiCompletion(context.Context, string, []byte) error
}
type httpHermesGatewayClient struct {
	url, token string
	client     *http.Client
}

func (c httpHermesGatewayClient) DeliverPrompt(
	ctx context.Context,
	p HermesPrompt,
) error {
	return c.post(ctx, "/vamos/prompts", p)
}

func (c httpHermesGatewayClient) DeliverPiCompletion(
	ctx context.Context,
	session string,
	result []byte,
) error {
	return c.post(ctx, "/vamos/pi/"+session+"/complete", json.RawMessage(result))
}

func (c httpHermesGatewayClient) post(
	ctx context.Context,
	suffix string,
	value any,
) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(c.url, "/")+suffix,
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("Hermes gateway delivery: %s", resp.Status)
	}
	return nil
}

func (s *Service) DeliverHermesPrompt(ctx context.Context, prompt HermesPrompt) error {
	if s.hermesGateway == nil {
		return fmt.Errorf("Hermes gateway is not configured")
	}
	if strings.TrimSpace(prompt.ThreadID) == "" ||
		strings.TrimSpace(prompt.OwnerEmail) == "" ||
		strings.TrimSpace(prompt.Prompt) == "" {
		return fmt.Errorf("thread, owner, and prompt are required")
	}
	return s.hermesGateway.DeliverPrompt(ctx, prompt)
}

func (s *Service) AppendHermesTranscript(
	ctx context.Context,
	event HermesCallbackEvent,
) error {
	_ = ctx
	planDir, err := s.hermesPlanDir(event.PlanDir)
	if err != nil {
		return err
	}
	return AppendHermesTranscript(planDir, event.HermesTranscriptEvent)
}

// HermesPiResult reads the current result only after confirming that the plan
// directory is a real descendant of the configured thoughts root. Gateway
// callback payloads are untrusted and must never select arbitrary host files.
func (s *Service) HermesPiResult(planDir, session string) ([]byte, error) {
	planDir, err := s.hermesPlanDir(planDir)
	if err != nil {
		return nil, err
	}
	if filepath.Base(session) != session || strings.TrimSpace(session) == "" {
		return nil, fmt.Errorf("safe Pi session ID is required")
	}
	return os.ReadFile(
		filepath.Join(planDir, ".vamos", "sessions", "pi", session+"_result.yaml"),
	)
}

func (s *Service) hermesPlanDir(planDir string) (string, error) {
	root := strings.TrimSpace(s.thoughtsRoot)
	planDir = strings.TrimSpace(planDir)
	if root == "" || planDir == "" {
		return "", fmt.Errorf("thoughts root and plan directory are required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	planDir, err = filepath.Abs(planDir)
	if err != nil {
		return "", err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve thoughts root: %w", err)
	}
	resolvedPlanDir, err := filepath.EvalSymlinks(planDir)
	if err != nil {
		return "", fmt.Errorf("resolve plan directory: %w", err)
	}
	if !pathWithinRoot(resolvedPlanDir, resolvedRoot) {
		return "", fmt.Errorf("plan directory escapes thoughts root")
	}
	return planDir, nil
}
