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

var (
	ErrHermesManagerNotFound = errors.New("Hermes manager session not found")
	ErrHermesPiRunNotFound   = errors.New("Hermes Pi run not found")
	ErrHermesPiRunAmbiguous  = errors.New("Hermes Pi run is ambiguous")
)

type HermesGatewayClient interface {
	DeliverPrompt(context.Context, HermesPrompt) error
	DeliverPiCompletion(context.Context, string, string, []byte) error
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
	threadID, session string,
	result []byte,
) error {
	return c.post(
		ctx,
		"/vamos/threads/"+threadID+"/pi/"+session+"/complete",
		json.RawMessage(result),
	)
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
	identity := HermesPlanIdentity(prompt.PlanDir)
	planDir, err := s.hermesPlanDir(prompt.PlanDir)
	if err != nil {
		return err
	}
	events, err := readHermesTranscriptContext(ctx, planDir, identity, prompt.ThreadID)
	if err != nil {
		return err
	}
	metadata := events[0]
	thread := HermesThread{
		ID: prompt.ThreadID, CreatorEmail: metadata.CreatorEmail,
		PromptAuthority: *metadata.PromptAuthority, PlanDir: prompt.PlanDir,
	}
	if !s.CanPromptThread(userEmail, thread) {
		return fmt.Errorf("prompt authority is required")
	}
	prompt.OwnerEmail = metadata.PromptAuthority.PrincipalValue
	return s.DeliverHermesPrompt(ctx, prompt)
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
	planDirIdentity, threadID string,
) ([]ChatMessageArgs, error) {
	planDir, err := s.hermesPlanDir(planDirIdentity)
	if err != nil {
		return nil, err
	}
	events, err := readHermesTranscriptContext(
		context.Background(), planDir, HermesPlanIdentity(planDirIdentity), threadID,
	)
	if err != nil {
		return nil, err
	}
	messages := make([]ChatMessageArgs, 0, len(events))
	for _, event := range events {
		if event.Type == "thread_metadata" {
			continue
		}
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
	identity := HermesPlanIdentity(event.PlanDir)
	planDir, err := s.hermesPlanDir(event.PlanDir)
	if err != nil {
		return err
	}
	event.HermesTranscriptEvent.PlanDir = identity
	return appendHermesTranscript(ctx, planDir, event.HermesTranscriptEvent)
}

// HermesPiResult reads the current result only after confirming that the plan
// directory is a real descendant of the configured thoughts root. Gateway
// callback payloads are untrusted and must never select arbitrary host files.
// HermesThreadForPiRun resolves durable child ownership from the plan transcript.
// Hermes process and manager state are deliberately not involved.
func (s *Service) HermesThreadForPiRun(planDirIdentity, session string) (string, error) {
	identity := HermesPlanIdentity(planDirIdentity)
	planDir, err := s.hermesPlanDir(planDirIdentity)
	if err != nil {
		return "", err
	}
	if filepath.Base(session) != session || strings.TrimSpace(session) == "" {
		return "", fmt.Errorf("safe Pi session ID is required")
	}
	thoughtsRoot, err := filepath.EvalSymlinks(s.thoughtsRoot)
	if err != nil {
		return "", fmt.Errorf("resolve thoughts root: %w", err)
	}
	threads, err := ScanHermesThreads(thoughtsRoot, planDir)
	if err != nil {
		return "", err
	}
	matched := ""
	for _, thread := range threads {
		events, err := readHermesTranscriptContext(
			context.Background(), planDir, identity, thread.ID,
		)
		if err != nil {
			return "", err
		}
		for _, event := range events {
			if event.Type != "pi_run" || event.PiSessionID != session {
				continue
			}
			if matched != "" && matched != thread.ID {
				return "", ErrHermesPiRunAmbiguous
			}
			matched = thread.ID
		}
	}
	if matched == "" {
		return "", ErrHermesPiRunNotFound
	}
	return matched, nil
}

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
	identity := HermesPlanIdentity(strings.TrimSpace(planDir))
	if err := ValidateHermesPlanIdentity(identity); err != nil {
		return "", err
	}
	root, err := resolveExistingDirectory(s.thoughtsRoot)
	if err != nil {
		return "", fmt.Errorf("resolve thoughts root: %w", err)
	}
	resolvedPlan, err := resolveExistingDirectory(
		filepath.Join(root, filepath.FromSlash(string(identity))),
	)
	if err != nil {
		return "", fmt.Errorf("resolve plan directory: %w", err)
	}
	if !pathWithinRoot(resolvedPlan, root) || resolvedPlan == root {
		return "", errors.New("plan directory escapes thoughts root")
	}
	derived, _, err := ResolveHermesPlanIdentity(root, resolvedPlan, "")
	if err != nil {
		return "", err
	}
	if derived != identity {
		return "", errors.New("plan identity does not match resolved plan")
	}
	return resolvedPlan, nil
}
