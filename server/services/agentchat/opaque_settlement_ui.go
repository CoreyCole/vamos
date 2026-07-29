package agentchat

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
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
	path := filepath.Join(
		planDir,
		".vamos",
		"sessions",
		"pi",
		session,
		"settlements",
		entry+".json",
	)
	if err := validateOpaqueSettlementPath(planDir, path); err != nil {
		return err
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	envelope, err := decodeOpaqueSettlement(bytes)
	if err != nil || envelope.ManagerThread != threadID || envelope.Session != session ||
		envelope.AssistantEntryID != entry {
		return errors.New("contained settlement does not match decision identity")
	}
	wantDiscovery := opaqueSettlementDiscoveryReference(session, entry)
	if strings.TrimSpace(successor.Discovery) != wantDiscovery {
		return errors.New("invalid settlement discovery reference")
	}
	if err := validateOpaqueSettlementTarget(planDir, successor); err != nil {
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

func validateOpaqueSettlementTarget(
	planDir string,
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
		if _, err := containedResolvedPath(planDir, path, ""); err != nil {
			return err
		}
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			return errors.New("contained launch target is required")
		}
	case "steer_existing":
		events, err := readHermesTranscript(planDir, target)
		if err != nil || len(events) == 0 {
			return errors.New("contained steer target is required")
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
