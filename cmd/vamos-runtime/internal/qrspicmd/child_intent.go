package qrspicmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"gopkg.in/yaml.v3"
)

const (
	ChildIntentGraphValidResult     ChildIntentKind = "graph_valid_result"
	ChildIntentActiveResultEvidence ChildIntentKind = "child_active_with_result_evidence"
	ChildIntentInteractiveChat      ChildIntentKind = "interactive_child_chat"
	ChildIntentRepairableResult     ChildIntentKind = "repairable_result"
	ChildIntentManagerQuestion      ChildIntentKind = "manager_question"
	ChildIntentPivotRequest         ChildIntentKind = "pivot_request"
	ChildIntentProviderFailure      ChildIntentKind = "provider_failure"
	ChildIntentNoResultIncomplete   ChildIntentKind = "no_result_or_incomplete"
	ChildIntentManualAdvanceRepair  ChildIntentKind = "manual_advance_repair"
	ChildIntentAmbiguousUnsafe      ChildIntentKind = "ambiguous_unsafe_stop"
)

type ResultEvidence struct {
	Message            SessionMessageEvidence `json:"message"`
	Output             string                 `json:"output"`
	Parsed             *ParsedDecision        `json:"parsed,omitempty"`
	ParseError         string                 `json:"parseError,omitempty"`
	ExplicitCompletion bool                   `json:"explicitCompletion"`
	PrimaryArtifact    string                 `json:"primaryArtifact,omitempty"`
	ResolvedArtifact   string                 `json:"resolvedArtifact,omitempty"`
	ArtifactVerified   bool                   `json:"artifactVerified"`
}

type ChildManagerRequest struct {
	Kind          string          `yaml:"kind"`
	RequestedNode wruntime.NodeID `yaml:"requested_node"`
	PlanDir       string          `yaml:"plan_dir"`
	Reason        string          `yaml:"reason"`
}

type ChildEvidence struct {
	Boundary               ChildBoundaryKind
	Interaction            ChildInteractionMode
	AfterMessageID         string
	CurrentMessage         SessionMessageEvidence
	CurrentResult          *ResultEvidence
	CurrentTerminal        *AssistantTerminalEvidence
	CurrentManagerRequest  *ChildManagerRequest
	ManagerRequestError    string
	LatestGraphValidResult *ResultEvidence
	ContentFingerprint     string
}

type ChildIntent struct {
	Kind          ChildIntentKind
	Evidence      ChildEvidence
	Parsed        *ParsedDecision
	Question      string
	RequestedNode wruntime.NodeID
	ManagerNeeded bool
	Retryable     bool
	Reason        string
}

func GatherChildEvidence(
	state ManagerState,
	opts ChildCompletionOptions,
) (ChildEvidence, error) {
	if state.ActiveChild == nil {
		return ChildEvidence{}, fmt.Errorf("no active child")
	}
	child := state.ActiveChild
	sessionPath := strings.TrimSpace(child.SessionPath)
	if sessionPath == "" {
		var err error
		sessionPath, err = ResolveSessionPath(child.SessionDir, child.SessionID, child.Cwd)
		if err != nil {
			return ChildEvidence{}, err
		}
	}
	messages, err := ExtractSessionEvidence(sessionPath)
	if err != nil {
		return ChildEvidence{}, err
	}
	current, postCursor, err := latestSessionEvidenceAfter(
		messages,
		child.EvidenceCursorMessageID,
	)
	if err != nil {
		return ChildEvidence{}, err
	}
	evidence := ChildEvidence{
		Boundary:           opts.Boundary,
		Interaction:        opts.Interaction,
		AfterMessageID:     child.EvidenceCursorMessageID,
		CurrentMessage:     current,
		ContentFingerprint: current.Fingerprint,
	}
	parseCtx := wruntime.ParseContext{
		RunID:          child.ID,
		SessionID:      child.SessionID,
		ExpectedNodeID: wruntime.NodeID(child.Stage),
	}
	if result := parseResultEvidence(current, state, parseCtx); result != nil {
		evidence.CurrentResult = result
	}
	for i := len(postCursor) - 1; i >= 0; i-- {
		result := parseResultEvidence(postCursor[i], state, parseCtx)
		if result != nil && result.Parsed != nil {
			evidence.LatestGraphValidResult = result
			break
		}
	}
	request, requestErr := ParseChildManagerRequest(current.Text)
	if requestErr != nil {
		evidence.ManagerRequestError = requestErr.Error()
	} else {
		evidence.CurrentManagerRequest = request
	}
	if isProviderTerminalMessage(current) {
		terminal := AssistantTerminalEvidence{
			SessionPath:        sessionPath,
			SessionID:          child.SessionID,
			Line:               current.Line,
			Timestamp:          current.Timestamp,
			StopReason:         current.StopReason,
			ErrorMessage:       current.ErrorMessage,
			ContextWindowError: IsContextWindowErrorMessage(current.ErrorMessage),
		}
		terminal.EvidenceID = terminalEvidenceID(terminal)
		evidence.CurrentTerminal = &terminal
	}

	return evidence, nil
}

func parseResultEvidence(
	message SessionMessageEvidence,
	state ManagerState,
	parseCtx wruntime.ParseContext,
) *ResultEvidence {
	if strings.TrimSpace(message.Text) == "" {
		return nil
	}
	if _, err := extractCompleteQRSPIResult(message.Text); err != nil {
		return nil
	}
	result := &ResultEvidence{
		Message:            message,
		Output:             message.Text,
		ExplicitCompletion: true,
	}
	parsed, err := ParseNormalizeValidateDecide(message.Text, state, parseCtx)
	if err != nil {
		result.ParseError = err.Error()

		return result
	}
	result.Parsed = &parsed
	result.PrimaryArtifact = parsed.Result.PrimaryArtifact

	return result
}

func extractCompleteQRSPIResult(text string) (string, error) {
	return qrspi.ExtractQRSPIResultYAML(text)
}

func ClassifyChildIntentForState(state ManagerState, evidence ChildEvidence) ChildIntent {
	if evidence.CurrentTerminal != nil &&
		evidence.LatestGraphValidResult != nil &&
		resultHasDurableArtifact(state, evidence.LatestGraphValidResult) {
		result := evidence.LatestGraphValidResult
		evidence.ContentFingerprint = sessionEvidenceFingerprint(
			result.Message.Fingerprint,
			evidence.CurrentMessage.Fingerprint,
		)

		return ChildIntent{
			Kind:     ChildIntentGraphValidResult,
			Evidence: evidence,
			Parsed:   result.Parsed,
			Reason:   "graph-valid result retained over later provider failure by durable artifact proof",
		}
	}

	return ClassifyChildIntent(evidence)
}

func ClassifyChildIntent(evidence ChildEvidence) ChildIntent {
	if evidence.CurrentResult != nil && evidence.CurrentResult.Parsed != nil {
		return ChildIntent{
			Kind:     ChildIntentGraphValidResult,
			Evidence: evidence,
			Parsed:   evidence.CurrentResult.Parsed,
			Reason:   string(ChildIntentGraphValidResult),
		}
	}
	if evidence.Interaction != ChildInteractionStageWork {
		return ChildIntent{
			Kind:     ChildIntentInteractiveChat,
			Evidence: evidence,
			Reason:   string(ChildIntentInteractiveChat),
		}
	}
	if evidence.CurrentResult != nil && evidence.CurrentResult.ExplicitCompletion {
		return ChildIntent{
			Kind:      ChildIntentRepairableResult,
			Evidence:  evidence,
			Retryable: true,
			Reason:    evidence.CurrentResult.ParseError,
		}
	}
	if evidence.CurrentManagerRequest != nil {
		return ChildIntent{
			Kind:          ChildIntentPivotRequest,
			Evidence:      evidence,
			RequestedNode: evidence.CurrentManagerRequest.RequestedNode,
			ManagerNeeded: true,
			Reason:        strings.TrimSpace(evidence.CurrentManagerRequest.Reason),
		}
	}
	if evidence.ManagerRequestError != "" {
		return ChildIntent{
			Kind:          ChildIntentAmbiguousUnsafe,
			Evidence:      evidence,
			ManagerNeeded: true,
			Reason:        evidence.ManagerRequestError,
		}
	}
	if IsConservativePivotRequest(evidence.CurrentMessage.Text) {
		return ChildIntent{
			Kind:          ChildIntentPivotRequest,
			Evidence:      evidence,
			ManagerNeeded: true,
			Reason:        strings.TrimSpace(evidence.CurrentMessage.Text),
		}
	}
	if evidence.CurrentTerminal == nil && IsConciseManagerQuestion(evidence.CurrentMessage.Text) {
		question := strings.TrimSpace(evidence.CurrentMessage.Text)

		return ChildIntent{
			Kind:          ChildIntentManagerQuestion,
			Evidence:      evidence,
			Question:      question,
			ManagerNeeded: true,
			Reason:        question,
		}
	}
	if evidence.CurrentTerminal != nil {
		return ChildIntent{
			Kind:          ChildIntentProviderFailure,
			Evidence:      evidence,
			ManagerNeeded: true,
			Reason:        strings.TrimSpace(evidence.CurrentTerminal.ErrorMessage),
		}
	}
	if strings.TrimSpace(evidence.CurrentMessage.Text) == "" {
		return ChildIntent{
			Kind:          ChildIntentNoResultIncomplete,
			Evidence:      evidence,
			ManagerNeeded: true,
			Reason:        string(ChildIntentNoResultIncomplete),
		}
	}

	return ChildIntent{
		Kind:          ChildIntentAmbiguousUnsafe,
		Evidence:      evidence,
		ManagerNeeded: true,
		Reason:        strings.TrimSpace(evidence.CurrentMessage.Text),
	}
}

var managerRequestFencePattern = regexp.MustCompile("(?s)```(?:yaml|yml)\\s*\\n(.*?)\\n?```")

func ParseChildManagerRequest(text string) (*ChildManagerRequest, error) {
	var candidates []string
	for _, match := range managerRequestFencePattern.FindAllStringSubmatch(text, -1) {
		if strings.Contains(match[1], "q_manager_request:") {
			candidates = append(candidates, strings.TrimSpace(match[1]))
		}
	}
	whole := strings.TrimSpace(text)
	if len(candidates) == 0 && strings.HasPrefix(whole, "q_manager_request:") {
		candidates = append(candidates, whole)
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	if len(candidates) != 1 {
		return nil, fmt.Errorf("expected exactly one q_manager_request envelope")
	}
	var envelope struct {
		Request ChildManagerRequest `yaml:"q_manager_request"`
	}
	decoder := yaml.NewDecoder(strings.NewReader(candidates[0]))
	decoder.KnownFields(true)
	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("parse q_manager_request YAML: %w", err)
	}
	envelope.Request.Kind = strings.TrimSpace(envelope.Request.Kind)
	envelope.Request.RequestedNode = wruntime.NodeID(strings.TrimSpace(string(envelope.Request.RequestedNode)))
	envelope.Request.PlanDir = strings.TrimSpace(envelope.Request.PlanDir)
	envelope.Request.Reason = strings.TrimSpace(envelope.Request.Reason)
	if envelope.Request.Kind == "" || envelope.Request.RequestedNode == "" || envelope.Request.Reason == "" {
		return nil, fmt.Errorf("q_manager_request requires kind, requested_node, and reason")
	}

	return &envelope.Request, nil
}

func IsConservativePivotRequest(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return false
	}
	hasRequest := strings.Contains(normalized, "follow-up") ||
		strings.Contains(normalized, "follow up") ||
		strings.Contains(normalized, "need to route") ||
		strings.Contains(normalized, "requesting") ||
		strings.Contains(normalized, "please route")
	if !hasRequest {
		return false
	}
	for _, node := range []string{
		"question", "research", "design", "outline", "plan", "implement", "implementation-review",
	} {
		if strings.Contains(normalized, node) {
			return true
		}
	}

	return false
}

func IsConciseManagerQuestion(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || len([]rune(trimmed)) > 500 || !strings.HasSuffix(trimmed, "?") {
		return false
	}
	if strings.Contains(trimmed, "qrspi_result") || strings.Contains(trimmed, "q_manager_request") {
		return false
	}

	return true
}

func isProviderTerminalMessage(message SessionMessageEvidence) bool {
	return strings.TrimSpace(message.ErrorMessage) != "" ||
		strings.EqualFold(message.StopReason, "error") ||
		strings.EqualFold(message.StopReason, "aborted")
}

func resultHasDurableArtifact(state ManagerState, result *ResultEvidence) bool {
	if result == nil || result.Parsed == nil {
		return false
	}
	rel := filepath.Clean(strings.TrimSpace(result.Parsed.Result.PrimaryArtifact))
	if rel == "." || filepath.IsAbs(rel) ||
		(rel != "thoughts" && !strings.HasPrefix(rel, "thoughts"+string(filepath.Separator))) {
		return false
	}
	planPath, err := filepath.EvalSymlinks(state.CanonicalPlanDir)
	if err != nil || !filepath.IsAbs(planPath) {
		return false
	}
	thoughtsRoot, ok := thoughtsRootForPlan(planPath)
	if !ok {
		return false
	}
	artifactPath, err := filepath.EvalSymlinks(filepath.Join(filepath.Dir(thoughtsRoot), rel))
	if err != nil || !pathWithin(planPath, artifactPath) {
		return false
	}
	info, err := os.Stat(artifactPath)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	result.ResolvedArtifact = artifactPath
	result.ArtifactVerified = true

	return true
}

func thoughtsRootForPlan(planPath string) (string, bool) {
	clean := filepath.Clean(planPath)
	volume := filepath.VolumeName(clean)
	rest := strings.TrimPrefix(clean, volume)
	parts := strings.Split(strings.Trim(rest, string(filepath.Separator)), string(filepath.Separator))
	for i, part := range parts {
		if part != "thoughts" {
			continue
		}
		root := filepath.Join(append([]string{volume + string(filepath.Separator)}, parts[:i+1]...)...)

		return filepath.Clean(root), true
	}

	return "", false
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}

	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
