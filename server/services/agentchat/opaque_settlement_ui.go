package agentchat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	opaqueSettlementReceivedEvent = "pi_settlement_received"
	opaqueSettlementDecisionEvent = "pi_settlement_successor_decided"
)

// OpaqueSettlementEvidence is presentation-only evidence. Neither its raw
// response nor its lexical YAML blocks carry an outcome or an instruction.
type OpaqueSettlementEvidence struct {
	Plan        string                  `json:"plan"`
	Thread      string                  `json:"thread"`
	Session     string                  `json:"session"`
	Entry       string                  `json:"entry"`
	RawResponse string                  `json:"raw_response"`
	YAMLBlocks  []opaqueSettlementFence `json:"yaml_blocks"`
}

type OpaqueSettlementSuccessor struct {
	Action     string `json:"action"`
	Target     string `json:"target,omitempty"`
	Discovery  string `json:"discovery"`
	Rationale  string `json:"rationale"`
	Actor      string `json:"actor"`
	Provenance string `json:"provenance"`
}

func (s *Service) ReceiveOpaqueSettlement(
	ctx context.Context,
	request OpaqueSettlementDeliveryRequest,
) error {
	if request.Version != opaqueSettlementDeliveryVersion ||
		!safeOpaqueComponent(request.Session) ||
		!safeOpaqueComponent(request.FinalEntryID) ||
		!safeOpaqueComponent(request.ManagerThread) ||
		strings.TrimSpace(request.Plan) == "" {
		return errors.New("invalid opaque settlement delivery identity")
	}
	raw, err := base64.StdEncoding.DecodeString(request.SettlementBytesBase64)
	if err != nil {
		return errors.New("invalid opaque settlement response bytes")
	}
	envelope, err := decodeOpaqueSettlement(raw)
	if err != nil || envelope.Plan != request.Plan ||
		envelope.ManagerThread != request.ManagerThread ||
		envelope.Session != request.Session ||
		envelope.AssistantEntryID != request.FinalEntryID {
		return errors.New("opaque settlement delivery does not match its response")
	}
	if request.DeliveryID != opaqueSettlementDeliveryID(
		envelope.Plan, envelope.Session, envelope.AssistantEntryID,
	) {
		return errors.New("opaque settlement delivery ID does not match its identity")
	}
	if err := s.requireOpaqueSettlementAdmission(ctx, request, raw); err != nil {
		return err
	}
	planDir, err := s.hermesPlanDirFromRelative(request.Plan)
	if err != nil {
		return err
	}
	if err := VerifyHermesPiRunBinding(
		planDir,
		request.ManagerThread,
		request.Session,
	); err != nil {
		return err
	}
	return AppendHermesTranscript(planDir, HermesTranscriptEvent{
		ID:       request.DeliveryID,
		At:       time.Now().UTC(),
		Type:     opaqueSettlementReceivedEvent,
		ThreadID: request.ManagerThread,
		Settlement: &OpaqueSettlementEvidence{
			Plan:        request.Plan,
			Thread:      request.ManagerThread,
			Session:     request.Session,
			Entry:       request.FinalEntryID,
			RawResponse: envelope.RawResponse,
			YAMLBlocks:  opaqueSettlementBlocks(envelope),
		},
	})
}

func (s *Service) DecideOpaqueSettlementSuccessor(
	ctx context.Context,
	planDir, threadID, session, entry string,
	successor OpaqueSettlementSuccessor,
) error {
	_ = ctx
	planDir, err := s.hermesPlanDir(planDir)
	if err != nil {
		return err
	}
	if !safeOpaqueComponent(threadID) || !safeOpaqueComponent(session) ||
		!safeOpaqueComponent(entry) {
		return errors.New("safe settlement identity is required")
	}
	if strings.TrimSpace(successor.Actor) == "" ||
		strings.TrimSpace(successor.Provenance) == "" ||
		strings.TrimSpace(successor.Rationale) == "" {
		return errors.New("decision actor, provenance, and rationale are required")
	}
	if err := validateOpaqueSettlementSuccessor(successor); err != nil {
		return err
	}
	path, err := opaqueSettlementPath(planDir, session, entry)
	if err != nil {
		return err
	}
	if err := validateOpaqueSettlementPath(planDir, path); err != nil {
		return err
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	planIdentity, err := opaqueSettlementPlanIdentity(s.thoughtsRoot, planDir)
	if err != nil {
		return err
	}
	envelope, err := decodeOpaqueSettlement(bytes)
	if err != nil || envelope.Plan != planIdentity ||
		envelope.ManagerThread != threadID || envelope.Session != session ||
		envelope.AssistantEntryID != entry {
		return errors.New("contained settlement does not match decision identity")
	}
	if err := VerifyHermesPiRunBinding(planDir, threadID, session); err != nil {
		return err
	}
	request := OpaqueSettlementDeliveryRequest{
		Version:       opaqueSettlementDeliveryVersion,
		DeliveryID:    opaqueSettlementDeliveryID(planIdentity, session, entry),
		Plan:          planIdentity,
		ManagerThread: threadID,
		Session:       session,
		FinalEntryID:  entry,
	}
	if err := s.requireOpaqueSettlementAdmission(ctx, request, bytes); err != nil {
		return err
	}
	wantDiscovery := opaqueSettlementDiscoveryReference(session, entry)
	if strings.TrimSpace(successor.Discovery) != wantDiscovery {
		return errors.New("invalid settlement discovery reference")
	}
	if err := validateOpaqueSettlementTarget(
		planDir,
		threadID,
		planIdentity,
		successor,
	); err != nil {
		return err
	}
	return AppendHermesTranscript(
		planDir,
		HermesTranscriptEvent{
			ID: fmt.Sprintf(
				"settlement-successor:%s:%s:%d",
				session,
				entry,
				time.Now().UnixNano(),
			),
			At:       time.Now().UTC(),
			Type:     opaqueSettlementDecisionEvent,
			ThreadID: threadID,
			Settlement: &OpaqueSettlementEvidence{
				Plan:    thoughtsRelativeTo(s.thoughtsRoot, planDir),
				Thread:  threadID,
				Session: session,
				Entry:   entry,
			},
			Successor: &successor,
		},
	)
}

func opaqueSettlementPath(planDir, session, entry string) (string, error) {
	if !safeOpaqueComponent(session) || !safeOpaqueComponent(entry) {
		return "", errors.New("safe settlement identity is required")
	}
	matches, err := filepath.Glob(filepath.Join(
		planDir,
		".vamos",
		"sessions",
		"pi",
		session,
		"settlements",
		"*_"+entry+".json",
	))
	if err != nil {
		return "", err
	}
	if len(matches) != 1 {
		return "", errors.New("expected exactly one timestamped opaque settlement")
	}

	return matches[0], nil
}

func opaqueSettlementPlanIdentity(root, planDir string) (string, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolvedPlan, err := filepath.EvalSymlinks(planDir)
	if err != nil {
		return "", err
	}
	return thoughtsRelativeTo(resolvedRoot, resolvedPlan), nil
}

func opaqueSettlementDiscoveryReference(session, entry string) string {
	return "pi/" + session + "/settlements/" + entry + ".json"
}

func validateOpaqueSettlementSuccessor(value OpaqueSettlementSuccessor) error {
	switch strings.TrimSpace(value.Action) {
	case "none", "handoff":
		if strings.TrimSpace(value.Target) != "" {
			return errors.New("action forbids a successor target")
		}
	case "start_child", "steer_existing":
		if !safeOpaqueComponent(strings.TrimSpace(value.Target)) {
			return errors.New("successor action requires a safe target")
		}
	default:
		return errors.New("unsupported successor action")
	}
	return nil
}

type opaqueSettlementLaunchArtifact struct {
	Version       int    `json:"version"`
	Kind          string `json:"kind"`
	LaunchID      string `json:"launch_id"`
	Plan          string `json:"plan"`
	ManagerThread string `json:"manager_thread"`
}

func validateOpaqueSettlementTarget(
	planDir, threadID, plan string,
	value OpaqueSettlementSuccessor,
) error {
	target := strings.TrimSpace(value.Target)
	switch strings.TrimSpace(value.Action) {
	case "start_child":
		path := filepath.Join(
			planDir,
			".vamos",
			"sessions",
			"hermes",
			"launches",
			target+".json",
		)
		resolved, err := containedResolvedPath(planDir, path, "")
		if err != nil {
			return err
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("contained launch target is required")
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return err
		}
		var artifact opaqueSettlementLaunchArtifact
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&artifact); err != nil {
			return errors.New("invalid contained launch target")
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return errors.New("invalid contained launch target")
		}
		if artifact.Version != 1 || artifact.Kind != "pi_child_launch" ||
			artifact.LaunchID != target ||
			artifact.Plan != plan ||
			artifact.ManagerThread != threadID {
			return errors.New("contained launch target does not match decision")
		}
	case "steer_existing":
		path := filepath.Join(planDir, ".vamos", "sessions", "pi", target)
		resolved, err := containedResolvedPath(planDir, path, "")
		if err != nil {
			return err
		}
		if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
			return errors.New("contained steer target is required")
		}
		if err := VerifyHermesPiRunBinding(planDir, threadID, target); err != nil {
			return err
		}
	}
	return nil
}

func opaqueSettlementBlocks(envelope opaqueSettlementEnvelope) []opaqueSettlementFence {
	if envelope.FencedYAMLBlocks == nil {
		return nil
	}
	return append([]opaqueSettlementFence(nil), (*envelope.FencedYAMLBlocks)...)
}

func (s *Service) hermesPlanDirFromRelative(plan string) (string, error) {
	if strings.TrimSpace(plan) == "" || strings.HasPrefix(plan, "/") ||
		strings.Contains(plan, "..") {
		return "", errors.New("safe thoughts-relative plan is required")
	}
	return s.hermesPlanDir(strings.TrimRight(s.thoughtsRoot, "/") + "/" + plan)
}
