package runtime

import "encoding/json"

type (
	WorkflowID    string
	NodeID        string
	NodeKind      string
	ResultStatus  string
	ResultOutcome string
)

const (
	NodeKindAgent       NodeKind = "agent"
	NodeKindHumanReview NodeKind = "human_review"
	NodeKindDone        NodeKind = "done"
)

const (
	StatusComplete   ResultStatus = "complete"
	StatusDone       ResultStatus = "done"
	StatusNeedsHuman ResultStatus = "needs_human"
	StatusBlocked    ResultStatus = "blocked"
	StatusError      ResultStatus = "error"
)

const (
	OutcomeComplete            ResultOutcome = "complete"
	OutcomeReadyForOutline     ResultOutcome = "ready-for-outline"
	OutcomeReadyForHumanReview ResultOutcome = "ready-for-human-review"
	OutcomeReadyForWorkspace   ResultOutcome = "ready-for-workspace"
	OutcomeNeedsReviewResearch ResultOutcome = "needs-review-research"
	OutcomeNeedsFollowup       ResultOutcome = "needs-followup"
)

type Definition struct {
	ID              WorkflowID
	Version         string
	Name            string
	Start           NodeID
	Nodes           map[NodeID]Node
	Edges           []Edge
	ResultParser    ResultParser
	ResultConverter ResultConverter
	PolicySpec      PolicySpec
}

type Node struct {
	ID             NodeID
	DisplayName    string
	Kind           NodeKind
	Prompt         PromptSpec
	Terminal       bool
	Contract       ResultContract
	AutoApprovable bool
}

type ResultContract struct {
	Statuses                []ResultStatus
	Outcomes                []ResultOutcome
	PrimaryArtifactRequired bool
}

func (c ResultContract) AllowsStatus(status ResultStatus) bool {
	if len(c.Statuses) == 0 {
		return isKnownStatus(status)
	}
	for _, allowed := range c.Statuses {
		if allowed == status {
			return true
		}
	}
	return false
}

func (c ResultContract) AllowsOutcome(outcome ResultOutcome) bool {
	if len(c.Outcomes) == 0 {
		return true
	}
	for _, allowed := range c.Outcomes {
		if allowed == outcome {
			return true
		}
	}
	return false
}

func isKnownStatus(status ResultStatus) bool {
	switch status {
	case StatusComplete, StatusDone, StatusNeedsHuman, StatusBlocked, StatusError:
		return true
	default:
		return false
	}
}

type Edge struct {
	From      NodeID
	To        NodeID
	Gate      GateSpec
	Condition EdgeCondition
	Outcome   ResultOutcome
	Predicate EdgePredicate
}

type TransitionContext struct {
	Config any
	State  State
	Result WorkflowResult
}

type TypedTransitionContext[TConfig any] struct {
	Config TConfig
	State  State
	Result WorkflowResult
}

type EdgePredicate func(TransitionContext) bool

type EdgeCondition struct {
	Status string
}

type PromptSpec struct {
	SkillPath string
	Template  string
	Static    string
}

type GateSpec struct {
	Human  bool
	Reason string
}

type PolicySpec struct {
	Defaults json.RawMessage
	Decode   func(json.RawMessage) (any, error)
	Validate func(json.RawMessage) error
}

type AutoModeConfig interface {
	IsAutoMode() bool
}
