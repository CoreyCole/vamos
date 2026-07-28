package agentchat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

var ErrHermesManagerNotFound = errors.New("Hermes manager session not found")

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
	if resp.StatusCode == http.StatusNotFound {
		return ErrHermesManagerNotFound
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("Hermes gateway delivery: %s", resp.Status)
	}
	return nil
}

func (s *Service) DeliverOwnedHermesPrompt(
	ctx context.Context,
	userEmail string,
	prompt HermesPrompt,
) error {
	owner, err := s.hermesOwner(ctx, prompt.ThreadID)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(owner), strings.TrimSpace(userEmail)) {
		return fmt.Errorf("only the thread owner may deliver Hermes prompts")
	}
	prompt.OwnerEmail = owner
	return s.DeliverHermesPrompt(ctx, prompt)
}

func (s *Service) hermesOwner(ctx context.Context, threadID string) (string, error) {
	if s.hermesThreadOwner != nil {
		return s.hermesThreadOwner(ctx, threadID)
	}
	thread, err := s.queries.GetAgentThread(ctx, threadID)
	if err != nil {
		return "", err
	}
	return thread.UserEmail, nil
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

// RenderHermesTranscript maps durable Hermes events into the existing safe
// chat presentation model. Final events use the shared Markdown renderer; tool
// events remain concise cards and never expose gateway tool arguments.
func (s *Service) RenderHermesTranscript(
	planDir, threadID string,
) ([]ChatMessageArgs, error) {
	planDir, err := s.hermesPlanDir(planDir)
	if err != nil {
		return nil, err
	}
	events, err := readHermesTranscript(planDir, threadID)
	if err != nil {
		return nil, err
	}
	messages := make([]ChatMessageArgs, 0, len(events))
	for _, event := range events {
		message := ChatMessageArgs{
			ID:      event.ID,
			Role:    "assistant",
			Content: event.Content,
		}
		switch event.Type {
		case "user":
			message.Role = "user"
		case "final":
			if s.renderer == nil {
				return nil, fmt.Errorf("Hermes Markdown renderer is not configured")
			}
			html, err := renderMarkdown(s.renderer, []byte(event.Content))
			if err != nil {
				return nil, err
			}
			message.HTMLContent = html
		case "tool":
			if event.Tool == nil {
				continue
			}
			message.Content = "Tool: " + event.Tool.Name
			if event.Tool.Status != "" {
				message.Content += " — " + event.Tool.Status
			}
		case "lifecycle", "pi_run":
			message.Role = "system"
		}
		messages = append(messages, message)
	}
	return messages, nil
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
	path, err := containedResolvedPath(
		planDir,
		filepath.Join(planDir, ".vamos", "sessions", "pi", session+"_result.yaml"),
		"",
	)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
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
	return resolvedPlanDir, nil
}
