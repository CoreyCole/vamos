package agentchat

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
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
	Plan        string   `json:"plan"`
	Thread      string   `json:"thread"`
	Session     string   `json:"session"`
	Entry       string   `json:"entry"`
	RawResponse string   `json:"raw_response"`
	YAMLBlocks  []string `json:"yaml_blocks"`
}

type OpaqueSettlementSuccessor struct {
	Action    string `json:"action"`
	Target    string `json:"target,omitempty"`
	Discovery string `json:"discovery,omitempty"`
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
		envelope.FinalEntryID != request.FinalEntryID {
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
			RawResponse: string(raw),
			YAMLBlocks:  envelope.Fences,
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
	if err := validateOpaqueSettlementSuccessor(successor); err != nil {
		return err
	}
	events, err := readHermesTranscript(planDir, threadID)
	if err != nil {
		return err
	}
	found := false
	for _, event := range events {
		if event.Type == opaqueSettlementReceivedEvent && event.Settlement != nil &&
			event.Settlement.Session == session &&
			event.Settlement.Entry == entry {
			found = true
			break
		}
	}
	if !found {
		return errors.New("opaque settlement was not delivered to this thread")
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

func validateOpaqueSettlementSuccessor(value OpaqueSettlementSuccessor) error {
	value.Action = strings.TrimSpace(value.Action)
	value.Target = strings.TrimSpace(value.Target)
	value.Discovery = strings.TrimSpace(value.Discovery)
	switch value.Action {
	case "none":
		if value.Target != "" || value.Discovery != "" {
			return errors.New("none has no successor target")
		}
	case "start_child", "steer_existing", "handoff":
		if !safeOpaqueComponent(value.Target) || value.Discovery == "" {
			return errors.New("successor action requires a safe target and discovery")
		}
	default:
		return errors.New("unsupported successor action")
	}
	return nil
}

func (s *Service) hermesPlanDirFromRelative(plan string) (string, error) {
	if strings.TrimSpace(plan) == "" || strings.HasPrefix(plan, "/") ||
		strings.Contains(plan, "..") {
		return "", errors.New("safe thoughts-relative plan is required")
	}
	return s.hermesPlanDir(strings.TrimRight(s.thoughtsRoot, "/") + "/" + plan)
}
