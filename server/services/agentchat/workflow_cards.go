package agentchat

import (
	"encoding/json"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi/semantic"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type QRSPIWorkflowCard struct {
	ThreadID        string
	WorkspaceID     string
	Stage           string
	Status          string
	Outcome         string
	Summary         string
	PrimaryArtifact string
	Artifacts       []wruntime.ArtifactRef
	RuntimeNextStep string
	NextSteps       []string
	AgentProgress   AgentProgress
	NextSessionID   string
	NextThreadID    string
	JumpCurrentHref string
	JumpNextEndHref string
	Cwd             WorkspaceCwdProjection
	Policy          WorkspaceWorkflowPolicyProjection
	WaitingHuman    bool
	CanContinue     bool
	RawResult       string
}

type AgentProgress struct {
	State           string
	CurrentNodeID   string
	CurrentThreadID string
	CurrentRunID    string
	LastMessageID   string
	CurrentHref     string
	NextEndHref     string
	UpdatedAt       string
}

func ProjectQRSPIWorkflowCard(
	state wruntime.State,
	runResult wruntime.WorkflowResult,
	policy WorkspaceWorkflowPolicyProjection,
	cwd WorkspaceCwdProjection,
	nextLabel string,
	workspaceID string,
	threadID string,
) (*QRSPIWorkflowCard, error) {
	action := semantic.ProjectNextActionFromState(runResult, state)
	return ProjectQRSPIWorkflowCardFromAction(
		state,
		runResult,
		action,
		policy,
		cwd,
		nextLabel,
		workspaceID,
		threadID,
	)
}

func ProjectQRSPIWorkflowCardFromAction(
	state wruntime.State,
	runResult wruntime.WorkflowResult,
	action semantic.NextAction,
	policy WorkspaceWorkflowPolicyProjection,
	cwd WorkspaceCwdProjection,
	nextLabel string,
	workspaceID string,
	threadID string,
) (*QRSPIWorkflowCard, error) {
	if strings.TrimSpace(string(runResult.SourceNodeID)) == "" {
		return nil, nil
	}
	rawResult := ""
	if len(runResult.Raw) > 0 && string(runResult.Raw) != "null" {
		var raw any
		if err := json.Unmarshal(runResult.Raw, &raw); err == nil {
			pretty, _ := json.MarshalIndent(raw, "", "  ")
			rawResult = string(pretty)
		} else {
			rawResult = string(runResult.Raw)
		}
	}
	threadID = strings.TrimSpace(threadID)
	progress := agentProgressForWorkflowState(state, threadID)
	if action.NextNodeID != "" {
		progress.CurrentNodeID = string(action.NextNodeID)
	}
	if strings.TrimSpace(nextLabel) == "" {
		nextLabel = string(action.NextNodeID)
	}
	return &QRSPIWorkflowCard{
		ThreadID:        threadID,
		WorkspaceID:     workspaceID,
		Stage:           string(runResult.SourceNodeID),
		Status:          string(runResult.Status),
		Outcome:         string(runResult.Outcome),
		Summary:         runResult.Summary,
		PrimaryArtifact: runResult.PrimaryArtifact,
		Artifacts:       runResult.Artifacts,
		RuntimeNextStep: nextLabel,
		NextSteps:       displayNextSteps(runResult.DisplayNext),
		AgentProgress:   progress,
		NextThreadID:    threadID,
		JumpCurrentHref: progress.CurrentHref,
		JumpNextEndHref: progress.NextEndHref,
		Cwd:             cwd,
		Policy:          policy,
		WaitingHuman:    action.Kind == semantic.NextActionWaitHuman,
		CanContinue: state.Status == wruntime.WorkspaceStatusIdle &&
			(action.Kind == semantic.NextActionStartNext || action.Kind == semantic.NextActionContinuePending),
		RawResult: rawResult,
	}, nil
}

func displayNextSteps(displayNext string) []string {
	lines := strings.Split(displayNext, "\n")
	steps := make([]string, 0, len(lines))
	for _, line := range lines {
		if step := strings.TrimSpace(line); step != "" {
			steps = append(steps, step)
		}
	}
	return steps
}

func agentProgressForWorkflowState(state wruntime.State, threadID string) AgentProgress {
	progress := AgentProgress{
		State:           string(state.Status),
		CurrentNodeID:   string(state.CurrentNodeID),
		CurrentThreadID: threadID,
	}
	if threadID != "" {
		progress.CurrentHref = BuildThoughtsChatDocURL(EmbeddedChatURLState{Context: ThoughtsChatContext, ThreadID: threadID})
		progress.NextEndHref = progress.CurrentHref
	}
	if state.PendingNextNodeID != "" {
		progress.CurrentNodeID = string(state.PendingNextNodeID)
	}
	return progress
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
