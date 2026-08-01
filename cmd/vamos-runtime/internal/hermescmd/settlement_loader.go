package hermescmd

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/CoreyCole/vamos/pkg/hermes/sessioningress"
)

const (
	MaxSettlementEvidenceBytes = 262_144
	yamlMappingPairWidth       = 2
	yamlStringTag              = "!!str"
)

type SettlementLoadExpectation struct {
	HermesSessionID             string
	Frame                       HandoffFrame
	OwnerUID                    int
	allowEvidenceHermesIdentity bool
}

type SettlementEvidenceV1 struct {
	Version         int
	HermesSessionID string
	PiSessionID     string
	MessageID       string
	RawResponse     string
	RawBytes        []byte
}

func validateSettlementExpectation(expected SettlementLoadExpectation) error {
	if expected.allowEvidenceHermesIdentity {
		if expected.HermesSessionID != "" {
			return errors.New("recovery Hermes session identity must come from evidence")
		}
	} else if _, err := sessioningress.ValidateSessionID(
		expected.HermesSessionID,
	); err != nil {
		return errors.New("invalid expected Hermes session ID")
	}

	return validateHandoffFrame(expected.Frame)
}

func parseSettlementEvidence(
	data []byte,
	expected SettlementLoadExpectation,
) (SettlementEvidenceV1, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return SettlementEvidenceV1{}, errors.New("invalid settlement evidence")
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return SettlementEvidenceV1{}, errors.New(
			"settlement evidence must be one mapping",
		)
	}
	mapping := document.Content[0]
	if err := validateSettlementYAML(mapping, make(map[*yaml.Node]struct{})); err != nil {
		return SettlementEvidenceV1{}, err
	}
	fields := make(
		map[string]*yaml.Node,
		len(mapping.Content)/yamlMappingPairWidth,
	)
	for index := 0; index < len(mapping.Content); index += yamlMappingPairWidth {
		key := mapping.Content[index]
		if key.Kind != yaml.ScalarNode || key.Tag != yamlStringTag {
			return SettlementEvidenceV1{}, errors.New(
				"settlement evidence keys must be strings",
			)
		}
		if _, duplicate := fields[key.Value]; duplicate {
			return SettlementEvidenceV1{}, errors.New(
				"duplicate settlement evidence field",
			)
		}
		fields[key.Value] = mapping.Content[index+1]
	}
	version, err := settlementInteger(fields["version"])
	if err != nil || version != 1 {
		return SettlementEvidenceV1{}, errors.New("invalid settlement evidence version")
	}
	hermesID, hermesOK := settlementString(fields["hermes_session_id"])
	piID, piOK := settlementString(fields["pi_session_id"])
	messageID, messageOK := settlementString(fields["message_id"])
	rawResponse, rawOK := settlementString(fields["raw_response"])
	if !hermesOK || !piOK || !messageOK || !rawOK || hermesID == "" ||
		piID == "" || messageID == "" {
		return SettlementEvidenceV1{}, errors.New("missing settlement evidence identity")
	}
	evidence := SettlementEvidenceV1{
		Version:         version,
		HermesSessionID: hermesID,
		PiSessionID:     piID,
		MessageID:       messageID,
		RawResponse:     rawResponse,
		RawBytes:        append([]byte(nil), data...),
	}
	if _, err := sessioningress.ValidateSessionID(evidence.HermesSessionID); err != nil {
		return SettlementEvidenceV1{}, errors.New("invalid settlement Hermes session ID")
	}
	if err := sessioningress.ValidatePiSessionID(evidence.PiSessionID); err != nil {
		return SettlementEvidenceV1{}, errors.New("invalid settlement Pi session ID")
	}
	if err := sessioningress.ValidateMessageID(evidence.MessageID); err != nil {
		return SettlementEvidenceV1{}, errors.New("invalid settlement message ID")
	}
	if (!expected.allowEvidenceHermesIdentity &&
		evidence.HermesSessionID != expected.HermesSessionID) ||
		evidence.PiSessionID != expected.Frame.PiSessionID ||
		evidence.MessageID != expected.Frame.MessageID {
		return SettlementEvidenceV1{}, errors.New("settlement evidence identity mismatch")
	}

	return evidence, nil
}

func validateSettlementYAML(node *yaml.Node, seen map[*yaml.Node]struct{}) error {
	if node == nil {
		return errors.New("invalid settlement evidence node")
	}
	if _, duplicate := seen[node]; duplicate {
		return errors.New("settlement evidence contains an alias or cycle")
	}
	seen[node] = struct{}{}
	defer delete(seen, node)
	if node.Kind == yaml.AliasNode || node.Anchor != "" {
		return errors.New("settlement evidence aliases and anchors are forbidden")
	}
	if node.Kind == yaml.MappingNode {
		if len(node.Content)%yamlMappingPairWidth != 0 {
			return errors.New("invalid settlement evidence mapping")
		}
		keys := make(
			map[string]struct{},
			len(node.Content)/yamlMappingPairWidth,
		)
		for index := 0; index < len(node.Content); index += yamlMappingPairWidth {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode || key.Tag != yamlStringTag {
				return errors.New("settlement evidence keys must be strings")
			}
			if _, duplicate := keys[key.Value]; duplicate {
				return errors.New("duplicate settlement evidence field")
			}
			keys[key.Value] = struct{}{}
		}
	}
	for _, child := range node.Content {
		if err := validateSettlementYAML(child, seen); err != nil {
			return err
		}
	}

	return nil
}

func settlementString(node *yaml.Node) (string, bool) {
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag != yamlStringTag {
		return "", false
	}

	return node.Value, true
}

func settlementInteger(node *yaml.Node) (int, error) {
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag != "!!int" ||
		node.Value != "1" {
		return 0, errors.New("invalid integer")
	}

	return 1, nil
}

func settlementFileUnchanged(before, after os.FileInfo) bool {
	return before.Mode() == after.Mode() && before.Size() == after.Size() &&
		before.ModTime().Equal(after.ModTime()) && os.SameFile(before, after)
}
