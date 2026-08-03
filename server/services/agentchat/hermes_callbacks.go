package agentchat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/safecomponent"
)

type HermesPrompt struct {
	CommandID             string   `json:"command_id"`
	ThreadID              string   `json:"thread_id"`
	OwnerEmail            string   `json:"owner_email"`
	PlanDir               string   `json:"plan_dir"`
	ConversationReference string   `json:"conversation_reference"`
	ContextPaths          []string `json:"context_paths,omitempty"`
	Prompt                string   `json:"prompt"`
}

type HermesPromptDeliveryStatus string

const (
	HermesPromptAccepted  HermesPromptDeliveryStatus = "accepted"
	HermesPromptRejected  HermesPromptDeliveryStatus = "rejected"
	HermesPromptFailed    HermesPromptDeliveryStatus = "failed"
	HermesPromptUncertain HermesPromptDeliveryStatus = "uncertain"
)

type HermesPromptDeliveryReason string

const HermesPromptGatewayUnavailable HermesPromptDeliveryReason = "gateway_unavailable"

type HermesPromptDeliveryObservation struct {
	Status HermesPromptDeliveryStatus
	Reason HermesPromptDeliveryReason
	Detail string
}

type HermesPromptResult struct {
	CommandID  string
	Status     HermesPromptDeliveryStatus
	Reason     HermesPromptDeliveryReason
	Detail     string
	InProgress bool
}

var hermesPromptAfterGatewayHook func()

type HermesCallbackEvent struct {
	PlanDir string `json:"plan_dir"`
	HermesTranscriptEvent
}

var (
	ErrHermesManagerNotFound    = errors.New("Hermes manager session not found")
	ErrHermesPiRunNotFound      = errors.New("Hermes Pi run not found")
	ErrHermesPiRunAmbiguous     = errors.New("Hermes Pi run is ambiguous")
	ErrHermesPromptUnauthorized = errors.New("Hermes prompt authority is required")
	ErrHermesPromptConflict     = errors.New("Hermes prompt command conflicts with its durable request")
)

type HermesGatewayClient interface {
	DeliverPrompt(context.Context, HermesPrompt) HermesPromptDeliveryObservation
	DeliverPiCompletion(context.Context, string, string, []byte) error
}
type httpHermesGatewayClient struct {
	url, token string
	client     *http.Client
}

func (c httpHermesGatewayClient) DeliverPrompt(
	ctx context.Context,
	p HermesPrompt,
) HermesPromptDeliveryObservation {
	body, err := json.Marshal(p)
	if err != nil {
		return HermesPromptDeliveryObservation{Status: HermesPromptFailed, Detail: err.Error()}
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(c.url, "/")+"/vamos/prompts",
		bytes.NewReader(body),
	)
	if err != nil {
		return HermesPromptDeliveryObservation{Status: HermesPromptFailed, Detail: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return HermesPromptDeliveryObservation{Status: HermesPromptUncertain, Detail: err.Error()}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusAccepted:
		return HermesPromptDeliveryObservation{Status: HermesPromptAccepted}
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return HermesPromptDeliveryObservation{Status: HermesPromptRejected, Detail: resp.Status}
	default:
		return HermesPromptDeliveryObservation{Status: HermesPromptUncertain, Detail: resp.Status}
	}
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

func (s *Service) SubmitOwnedHermesPrompt(
	ctx context.Context,
	userEmail string,
	prompt HermesPrompt,
) (HermesPromptResult, error) {
	identity := HermesPlanIdentity(prompt.PlanDir)
	planDir, err := s.hermesPlanDir(prompt.PlanDir)
	if err != nil {
		return HermesPromptResult{}, err
	}
	events, err := readHermesTranscriptContext(ctx, planDir, identity, prompt.ThreadID)
	if err != nil {
		return HermesPromptResult{}, err
	}
	metadata := events[0]
	thread := HermesThread{
		ID: prompt.ThreadID, CreatorEmail: metadata.CreatorEmail,
		PromptAuthority: *metadata.PromptAuthority, PlanDir: prompt.PlanDir,
	}
	if !s.CanPromptThread(userEmail, thread) {
		return HermesPromptResult{}, ErrHermesPromptUnauthorized
	}
	prompt.OwnerEmail = metadata.PromptAuthority.PrincipalValue
	prompt, digest, err := s.validateHermesPromptCommand(prompt)
	if err != nil {
		return HermesPromptResult{}, err
	}
	commandLock, err := tryAcquireHermesCommandLock(ctx, planDir, prompt.ThreadID, prompt.CommandID)
	if err != nil {
		if errors.Is(err, ErrHermesPromptInProgress) {
			if conflictErr := compareHermesPromptRequest(
				ctx, planDir, identity, prompt.ThreadID, prompt.CommandID, digest,
			); conflictErr != nil {
				return HermesPromptResult{}, conflictErr
			}
			return HermesPromptResult{CommandID: prompt.CommandID, InProgress: true}, nil
		}
		return HermesPromptResult{}, err
	}
	defer commandLock.Close()

	result, send, err := reserveHermesPromptCommand(ctx, planDir, identity, prompt, digest)
	if err != nil || !send {
		return result, err
	}

	var observation HermesPromptDeliveryObservation
	if s.hermesGateway == nil {
		observation = HermesPromptDeliveryObservation{
			Status: HermesPromptFailed,
			Reason: HermesPromptGatewayUnavailable,
			Detail: "Hermes gateway is not configured",
		}
	} else {
		deliveryCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		observation = s.hermesGateway.DeliverPrompt(deliveryCtx, prompt)
		cancel()
		if !validHermesPromptDeliveryStatus(observation.Status) {
			observation = HermesPromptDeliveryObservation{
				Status: HermesPromptUncertain,
				Detail: "Hermes gateway returned an invalid delivery observation",
			}
		}
	}
	if hermesPromptAfterGatewayHook != nil {
		hermesPromptAfterGatewayHook()
	}
	persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer persistCancel()
	return finishHermesPromptCommand(persistCtx, planDir, identity, prompt, digest, observation)
}

func (s *Service) validateHermesPromptCommand(
	prompt HermesPrompt,
) (HermesPrompt, string, error) {
	if err := ValidateHermesPlanIdentity(HermesPlanIdentity(prompt.PlanDir)); err != nil {
		return HermesPrompt{}, "", err
	}
	if err := safecomponent.ValidateBounded(prompt.ThreadID); err != nil {
		return HermesPrompt{}, "", err
	}
	if err := safecomponent.ValidateBounded(prompt.CommandID); err != nil {
		return HermesPrompt{}, "", err
	}
	if strings.TrimSpace(prompt.Prompt) == "" {
		return HermesPrompt{}, "", errors.New("prompt is required")
	}
	reference, err := HermesConversationReference(HermesPlanIdentity(prompt.PlanDir), prompt.ThreadID)
	if err != nil {
		return HermesPrompt{}, "", err
	}
	if prompt.ConversationReference != "" && prompt.ConversationReference != reference {
		return HermesPrompt{}, "", errors.New("Hermes conversation reference mismatch")
	}
	prompt.ConversationReference = reference
	canonicalPaths := make([]string, 0, len(prompt.ContextPaths))
	for _, raw := range prompt.ContextPaths {
		attached, err := s.ValidateAttachedThoughtsPath(raw)
		if err != nil {
			return HermesPrompt{}, "", err
		}
		canonicalPaths = append(canonicalPaths, attached.Path)
	}
	prompt.ContextPaths = canonicalPaths
	binding, err := json.Marshal(struct {
		Prompt       string   `json:"prompt"`
		ContextPaths []string `json:"context_paths"`
	}{Prompt: prompt.Prompt, ContextPaths: prompt.ContextPaths})
	if err != nil {
		return HermesPrompt{}, "", err
	}
	digestBytes := sha256.Sum256(append([]byte("vamos-hermes-prompt-command-v1\x00"), binding...))
	return prompt, hex.EncodeToString(digestBytes[:]), nil
}

func compareHermesPromptRequest(
	ctx context.Context,
	planDir string,
	identity HermesPlanIdentity,
	threadID, commandID, digest string,
) error {
	events, err := readHermesTranscriptContext(ctx, planDir, identity, threadID)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.CommandID == commandID && event.Type == "prompt_requested" && event.CommandDigest != digest {
			return ErrHermesPromptConflict
		}
	}
	return nil
}

func reserveHermesPromptCommand(
	ctx context.Context,
	planDir string,
	identity HermesPlanIdentity,
	prompt HermesPrompt,
	digest string,
) (HermesPromptResult, bool, error) {
	lock, err := acquireHermesTranscriptLock(ctx, planDir, prompt.ThreadID, false)
	if err != nil {
		return HermesPromptResult{}, false, err
	}
	defer lock.Close()
	path, err := hermesTranscriptReadPath(planDir, prompt.ThreadID)
	if err != nil {
		return HermesPromptResult{}, false, err
	}
	parsed, err := readHermesTranscriptFile(path, identity, prompt.ThreadID)
	if err != nil {
		return HermesPromptResult{}, false, err
	}
	requested, started := false, false
	for _, event := range parsed.events {
		if event.CommandID != prompt.CommandID {
			continue
		}
		if event.CommandDigest != digest {
			return HermesPromptResult{}, false, ErrHermesPromptConflict
		}
		switch event.Type {
		case "prompt_requested":
			requested = true
		case "prompt_delivery_started":
			started = true
		case "prompt_delivery":
			status := HermesPromptDeliveryStatus(event.DeliveryStatus)
			if !validHermesPromptDeliveryStatus(status) {
				return HermesPromptResult{}, false, errors.New("invalid durable Hermes prompt delivery status")
			}
			return HermesPromptResult{
				CommandID: prompt.CommandID,
				Status:    status,
				Reason:    HermesPromptDeliveryReason(event.DeliveryReason),
				Detail:    event.Content,
			}, false, nil
		}
	}
	if !requested {
		if err := appendHermesTranscriptUnlocked(path, HermesTranscriptEvent{
			ID: hermesPromptEventID("request", prompt.CommandID), Type: "prompt_requested",
			ThreadID: prompt.ThreadID, PlanDir: identity, CommandID: prompt.CommandID,
			CommandDigest: digest, ContextPaths: prompt.ContextPaths, Content: prompt.Prompt,
		}); err != nil {
			return HermesPromptResult{}, false, err
		}
	}
	if started {
		observation := HermesPromptDeliveryObservation{
			Status: HermesPromptUncertain,
			Detail: "delivery started without a durable terminal observation",
		}
		result, err := appendHermesPromptTerminal(path, identity, prompt, digest, observation)
		return result, false, err
	}
	if err := appendHermesTranscriptUnlocked(path, HermesTranscriptEvent{
		ID: hermesPromptEventID("started", prompt.CommandID), Type: "prompt_delivery_started",
		ThreadID: prompt.ThreadID, PlanDir: identity, CommandID: prompt.CommandID,
		CommandDigest: digest,
	}); err != nil {
		return HermesPromptResult{}, false, err
	}
	return HermesPromptResult{CommandID: prompt.CommandID}, true, nil
}

func finishHermesPromptCommand(
	ctx context.Context,
	planDir string,
	identity HermesPlanIdentity,
	prompt HermesPrompt,
	digest string,
	observation HermesPromptDeliveryObservation,
) (HermesPromptResult, error) {
	lock, err := acquireHermesTranscriptLock(ctx, planDir, prompt.ThreadID, false)
	if err != nil {
		return HermesPromptResult{}, err
	}
	defer lock.Close()
	path, err := hermesTranscriptReadPath(planDir, prompt.ThreadID)
	if err != nil {
		return HermesPromptResult{}, err
	}
	parsed, err := readHermesTranscriptFile(path, identity, prompt.ThreadID)
	if err != nil {
		return HermesPromptResult{}, err
	}
	for _, event := range parsed.events {
		if event.CommandID != prompt.CommandID {
			continue
		}
		if event.CommandDigest != digest {
			return HermesPromptResult{}, ErrHermesPromptConflict
		}
		if event.Type == "prompt_delivery" {
			if event.DeliveryStatus != string(observation.Status) ||
				event.DeliveryReason != string(observation.Reason) {
				return HermesPromptResult{}, ErrHermesPromptConflict
			}
			return HermesPromptResult{
				CommandID: prompt.CommandID, Status: observation.Status,
				Reason: HermesPromptDeliveryReason(event.DeliveryReason), Detail: event.Content,
			}, nil
		}
	}
	return appendHermesPromptTerminal(path, identity, prompt, digest, observation)
}

func appendHermesPromptTerminal(
	path string,
	identity HermesPlanIdentity,
	prompt HermesPrompt,
	digest string,
	observation HermesPromptDeliveryObservation,
) (HermesPromptResult, error) {
	if !validHermesPromptDeliveryStatus(observation.Status) {
		return HermesPromptResult{}, errors.New("invalid Hermes prompt delivery status")
	}
	err := appendHermesTranscriptUnlocked(path, HermesTranscriptEvent{
		ID: hermesPromptEventID("terminal", prompt.CommandID), Type: "prompt_delivery",
		ThreadID: prompt.ThreadID, PlanDir: identity, CommandID: prompt.CommandID,
		CommandDigest: digest, DeliveryStatus: string(observation.Status),
		DeliveryReason: string(observation.Reason), Content: strings.TrimSpace(observation.Detail),
	})
	return HermesPromptResult{
		CommandID: prompt.CommandID,
		Status:    observation.Status,
		Reason:    observation.Reason,
		Detail:    strings.TrimSpace(observation.Detail),
	}, err
}

func validHermesPromptDeliveryStatus(status HermesPromptDeliveryStatus) bool {
	switch status {
	case HermesPromptAccepted, HermesPromptRejected, HermesPromptFailed, HermesPromptUncertain:
		return true
	default:
		return false
	}
}

func hermesPromptEventID(kind, commandID string) string {
	digest := sha256.Sum256([]byte("vamos-hermes-prompt-event-v1\x00" + kind + "\x00" + commandID))
	return "prompt_" + hex.EncodeToString(digest[:])
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
		case "user", "prompt_requested":
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
		case "lifecycle", "pi_run", "prompt_delivery_started", "prompt_delivery":
			message.Role = "system"
			if event.Type == "prompt_delivery" {
				message.Content = "Prompt delivery: " + event.DeliveryStatus
			}
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
