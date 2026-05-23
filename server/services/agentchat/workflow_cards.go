package agentchat

import (
	"encoding/json"
	"strings"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type QRSPIWorkflowCard struct {
	WorkspaceID     string
	Stage           string
	Status          string
	Outcome         string
	Summary         string
	PrimaryArtifact string
	Artifacts       []wruntime.ArtifactRef
	RuntimeNextStep string
	Cwd             WorkspaceCwdProjection
	Policy          WorkspaceWorkflowPolicyProjection
	WaitingHuman    bool
	RawXML          string
}

func ProjectQRSPIWorkflowCard(
	state wruntime.State,
	runResult wruntime.WorkflowResult,
	policy WorkspaceWorkflowPolicyProjection,
	cwd WorkspaceCwdProjection,
	nextLabel string,
	workspaceID string,
) (*QRSPIWorkflowCard, error) {
	if strings.TrimSpace(string(runResult.SourceNodeID)) == "" {
		return nil, nil
	}
	rawXML := ""
	if len(runResult.Raw) > 0 && string(runResult.Raw) != "null" {
		var raw any
		if err := json.Unmarshal(runResult.Raw, &raw); err == nil {
			pretty, _ := json.MarshalIndent(raw, "", "  ")
			rawXML = string(pretty)
		} else {
			rawXML = string(runResult.Raw)
		}
	}
	return &QRSPIWorkflowCard{
		WorkspaceID:     workspaceID,
		Stage:           string(runResult.SourceNodeID),
		Status:          string(runResult.Status),
		Outcome:         string(runResult.Outcome),
		Summary:         runResult.Summary,
		PrimaryArtifact: runResult.PrimaryArtifact,
		Artifacts:       runResult.Artifacts,
		RuntimeNextStep: nextLabel,
		Cwd:             cwd,
		Policy:          policy,
		WaitingHuman:    state.Status == wruntime.WorkspaceStatusWaitingHuman,
		RawXML:          rawXML,
	}, nil
}

func RuntimeNextNodeLabel(def wruntime.Definition, state wruntime.State) string {
	next := state.PendingNextNodeID
	if next == "" && state.HumanGate != nil {
		next = state.HumanGate.To
	}
	if next == "" {
		return ""
	}
	if node, ok := def.Nodes[next]; ok && strings.TrimSpace(node.DisplayName) != "" {
		return node.DisplayName
	}
	return string(next)
}

func attachQRSPIWorkflowCardToLatestAssistantMessage(
	messages []TranscriptMessage,
	card *QRSPIWorkflowCard,
) []TranscriptMessage {
	if card == nil || len(messages) == 0 {
		return messages
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].WorkflowCard != nil {
			return messages
		}
		if messages[i].Variant == "detail" ||
			strings.TrimSpace(messages[i].Role) != "assistant" {
			continue
		}
		messages[i].WorkflowCard = card
		return messages
	}
	return messages
}
