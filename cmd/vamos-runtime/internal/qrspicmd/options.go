package qrspicmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi/semantic"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type InitOptions struct {
	PlanDir           string
	ProjectRoot       string
	PolicyFile        string
	NodeID            string
	ImplementationCwd string
	ManagerPane       string
	Force             bool
}

type RunChildOptions struct {
	PlanDir      string
	Stage        string
	Cwd          string
	PromptFile   string
	StateFile    string
	Split        string
	ManagerRunID string
	ManagerPane  string
	Timeout      time.Duration
}

type StartNextOptions struct {
	PlanDir           string
	ProjectRoot       string
	StateFile         string
	PolicyFile        string
	NodeID            string
	ImplementationCwd string
	ManagerPane       string
	LatestResultFile  string
	LatestResultStdin bool
	Cwd               string
	Split             string
	Timeout           time.Duration
	Output            string
	Force             bool
	Usage             ManagerUsageInput
}

type StartNextResult struct {
	StateFile       string
	CurrentNode     string
	PromptFile      string
	ActiveChild     *ChildRunRef
	StopReason      string
	NextCommand     string
	FeedbackCommand string
}

type ManagerNotice struct {
	Kind            string `json:"kind,omitempty"`
	Validated       bool   `json:"validated"`
	ManagerNeeded   bool   `json:"managerNeeded"`
	RetryExhausted  bool   `json:"retryExhausted"`
	StateFile       string `json:"stateFile,omitempty"`
	Stage           string `json:"stage,omitempty"`
	Status          string `json:"status,omitempty"`
	Outcome         string `json:"outcome,omitempty"`
	Artifact        string `json:"artifact,omitempty"`
	ChildPane       string `json:"childPane,omitempty"`
	Summary         string `json:"summary,omitempty"`
	ManagerGuidance string `json:"managerGuidance,omitempty"`
	NextCommand     string `json:"nextCommand,omitempty"`
	FeedbackCommand string `json:"feedbackCommand,omitempty"`
}

type ChildCompletionOptions struct {
	StateFile string
	ChildID   string
	Output    string
}

type ManagerReadyOptions struct {
	StateFile   string
	ManagerPane string
	Output      string
}

type ManagerUsageInput struct {
	UsagePercent *float64 `json:"usagePercent,omitempty"`
	Tokens       *int     `json:"tokens,omitempty"`
	Window       *int     `json:"window,omitempty"`
}

type ManagerCompactionOptions struct {
	StateFile string
	Usage     ManagerUsageInput
	Output    string
}

type ChildCompletionStatus struct {
	Validated      bool                    `json:"validated"`
	ManagerNeeded  bool                    `json:"managerNeeded"`
	RetryExhausted bool                    `json:"retryExhausted"`
	ChildID        string                  `json:"childId"`
	DeliveryID     string                  `json:"deliveryId"`
	Result         ChildCompletionResult   `json:"result,omitempty"`
	Wake           WakeDeliveryInstruction `json:"wake"`
	ActionCard     *ManagerActionCard      `json:"actionCard,omitempty"`
	Normalizations []ResultNormalization   `json:"normalizations,omitempty"`
	Reason         string                  `json:"reason,omitempty"`
	Attempt        int                     `json:"attempt,omitempty"`
	RetryLimit     int                     `json:"retryLimit,omitempty"`
}

type ResultNormalization struct {
	Field     string `json:"field"`
	Original  string `json:"original"`
	Canonical string `json:"canonical"`
	Reason    string `json:"reason"`
}

type ChildCompletionResult struct {
	Stage    string `json:"stage,omitempty"`
	Status   string `json:"status,omitempty"`
	Outcome  string `json:"outcome,omitempty"`
	Artifact string `json:"artifact,omitempty"`
	Summary  string `json:"summary,omitempty"`
}

type WakeDeliveryInstruction struct {
	Mode    string `json:"mode"`
	Payload string `json:"payload,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type ManagerActionCard struct {
	Kind              string   `json:"kind"`
	Severity          string   `json:"severity"`
	Summary           string   `json:"summary"`
	Evidence          []string `json:"evidence,omitempty"`
	RecommendedAction string   `json:"recommendedAction"`
	ReviewSummary     string   `json:"reviewSummary,omitempty"`
	SafeCommand       string   `json:"safeCommand,omitempty"`
	ContinueCommand   string   `json:"continueCommand,omitempty"`
	RequiresHuman     bool     `json:"requiresHuman"`
	RecoveryLogPath   string   `json:"recoveryLogPath,omitempty"`
}

const (
	ActionStateDesync          = "state_desync"
	ActionGraphOutcomeMismatch = "graph_outcome_mismatch"
	ActionWorkspaceMoved       = "workspace_moved"
	ActionActiveChildConflict  = "active_child_conflict"
	ActionHumanGate            = "human_gate"
	ActionInvalidChildYAML     = "invalid_child_yaml"
	ActionManualChildSteer     = "manual_child_steer"
	ActionSupersededQueuedWake = "superseded_queued_wake"
)

func ProjectManagerActionCard(action semantic.NextAction, state ManagerState, stateFile string) *ManagerActionCard {
	stateFile = strings.TrimSpace(stateFile)
	continueCmd := continueCommand(stateFile)
	evidence := append([]string{}, action.Evidence...)
	if action.CurrentNodeID != "" {
		evidence = append(evidence, fmt.Sprintf("stage: %s", action.CurrentNodeID))
	}
	if action.PrimaryArtifact != "" {
		evidence = append(evidence, fmt.Sprintf("artifact: %s", action.PrimaryArtifact))
	}
	summary := strings.TrimSpace(action.RecoveryReason)
	switch action.Kind {
	case semantic.NextActionWaitHuman:
		if summary == "" {
			summary = "child requested human input"
		}
		return &ManagerActionCard{Kind: ActionHumanGate, Severity: severityOrDefault(action.Severity, "info"), Summary: summary, Evidence: evidence, RecommendedAction: "summarize the artifact/question for the human, then steer the same child", SafeCommand: fmt.Sprintf("vamos qrspi steer-child --state-file %s --feedback-file <file>", stateFile), ContinueCommand: continueCmd, RequiresHuman: true}
	case semantic.NextActionInvalidRetry, semantic.NextActionInvalidExhausted:
		if summary == "" {
			summary = "child result needs deterministic repair"
		}
		return &ManagerActionCard{Kind: ActionInvalidChildYAML, Severity: severityOrDefault(action.Severity, "warning"), Summary: summary, Evidence: evidence, RecommendedAction: "reprompt or steer the active child with canonical YAML", SafeCommand: invalidActionSafeCommand(action, state, stateFile), ContinueCommand: continueCmd, RequiresHuman: false}
	case semantic.NextActionBlocked:
		if summary == "" {
			summary = "child reported blocked"
		}
		return &ManagerActionCard{Kind: ActionGraphOutcomeMismatch, Severity: severityOrDefault(action.Severity, "warning"), Summary: summary, Evidence: evidence, RecommendedAction: "inspect the artifact/session, then steer or continue only if deterministic", SafeCommand: continueCmd, ContinueCommand: continueCmd, RequiresHuman: false}
	case semantic.NextActionError:
		if summary == "" {
			summary = "child reported error"
		}
		return &ManagerActionCard{Kind: ActionGraphOutcomeMismatch, Severity: severityOrDefault(action.Severity, "error"), Summary: summary, Evidence: evidence, RecommendedAction: "diagnose the child artifact/session before continuing", SafeCommand: continueCmd, ContinueCommand: continueCmd, RequiresHuman: false}
	case semantic.NextActionManualRecovery:
		if summary == "" {
			summary = "manual recovery needed"
		}
		return &ManagerActionCard{Kind: ActionManualChildSteer, Severity: severityOrDefault(action.Severity, "info"), Summary: summary, Evidence: evidence, RecommendedAction: "recover latest relevant child session", SafeCommand: fmt.Sprintf("vamos qrspi recover-manual --state-file %s --mode latest-session", stateFile), ContinueCommand: continueCmd, RequiresHuman: false}
	default:
		return nil
	}
}

func severityOrDefault(severity, fallback string) string {
	if strings.TrimSpace(severity) != "" {
		return severity
	}
	return fallback
}

func invalidActionSafeCommand(action semantic.NextAction, state ManagerState, stateFile string) string {
	stage := strings.TrimSpace(string(action.CurrentNodeID))
	if stage == "" && state.ActiveChild != nil {
		stage = state.ActiveChild.Stage
	}
	if action.Kind == semantic.NextActionInvalidExhausted {
		return feedbackCommand(stateFile)
	}
	attempt := 1
	if state.ActiveChild != nil {
		attempt = state.ActiveChild.ValidationRetryCount + 1
	}
	return fmt.Sprintf("vamos qrspi reprompt-child --state-file %s --plan-dir %s --stage %s --attempt %d", stateFile, state.CanonicalPlanDir, stage, attempt)
}

type RepairStateOptions struct {
	StateFile        string
	AlignActiveChild bool
	Output           string
}

type MarkChildActiveOptions struct {
	StateFile string
	ChildID   string
	Reason    string
	Output    string
}

type ValidationRecoveryLog struct {
	Timestamp        time.Time `json:"timestamp"`
	StateFile        string    `json:"stateFile,omitempty"`
	PlanDir          string    `json:"planDir,omitempty"`
	CurrentNode      string    `json:"currentNode,omitempty"`
	ActiveChildStage string    `json:"activeChildStage,omitempty"`
	ResultStage      string    `json:"resultStage,omitempty"`
	ResultStatus     string    `json:"resultStatus,omitempty"`
	ResultOutcome    string    `json:"resultOutcome,omitempty"`
	Recovered        bool      `json:"recovered"`
	RecoveryAction   string    `json:"recoveryAction,omitempty"`
	Reason           string    `json:"reason,omitempty"`
}

type PromptFileOptions struct {
	StateFile string
	NodeID    string
	Timestamp time.Time
}

type SteerChildOptions struct {
	StateFile     string
	FeedbackFile  string
	Feedback      string
	Stage         string
	Output        string
	RequireActive bool
}

type SteerChildResult struct {
	StateFile    string
	Stage        string
	PaneID       string
	FeedbackPath string
	NextCommand  string
}

type ValidateResultOptions struct {
	Stage       string
	StateFile   string
	ResultFile  string // deprecated/debug fallback
	SessionFile string
	PlanDir     string
	RunID       string
	SessionID   string
}

type DecideNextOptions struct {
	StateFile   string
	ResultFile  string // deprecated/debug fallback
	SessionFile string
	PlanDir     string
}

type RepromptChildOptions struct {
	StateFile string
	PlanDir   string
	Stage     string
	Attempt   int
	ErrorText string
	ErrorFile string
}

type ContinueOptions struct {
	StateFile string
	PlanDir   string
	Stage     string
	Cwd       string
	Split     string
	Timeout   time.Duration
	Output    string
	Usage     ManagerUsageInput
}

type ContinueResult struct {
	Validated       *ParsedDecision
	Reprompted      bool
	Decided         bool
	StartedChild    *ChildRunRef
	CleanedChild    *ChildRunRef
	StopReason      string
	WaitingHuman    bool
	NextNodeID      wruntime.NodeID
	PrimaryArtifact string
	HumanPrompt     HumanPromptContext
	ActionCard      *ManagerActionCard
}

type HumanPromptContext struct {
	Stage                    string
	Status                   string
	Summary                  string
	Artifact                 string
	SuggestedFeedbackCommand string
}

type ResultSourceOptions struct {
	ResultFile  string
	SessionFile string
	SessionID   string
	RunID       string
}

type RenderPromptOptions struct {
	StateFile string
	NodeID    string
	PlanDir   string
}

type deps struct {
	StateStore StateStore
	Runner     ChildRunner
	Tmux       TmuxClient
	Clock      func() time.Time
	StateRoot  func() (string, error)
}

// StateStore is implemented by the external q-manager state store in a later slice.
type StateStore interface {
	Load(path string) (ManagerState, error)
	Save(path string, state ManagerState) error
	AcquireLock(ctx context.Context, key LockKey, owner string, ttl time.Duration) (Lock, error)
}

// ChildRunner starts visible child QRSPI sessions and observes their done marker/session refs.
type ChildRunner interface {
	Start(ctx context.Context, req ChildRunRequest) (ChildRun, error)
	Wait(ctx context.Context, run ChildRun) (ChildRunResult, error)
}

// TmuxClient adapts tmux pane operations for visible child sessions.
type TmuxClient interface {
	SplitPane(ctx context.Context, req TmuxSplitRequest) (TmuxPane, error)
	SendKeys(ctx context.Context, pane TmuxPane, keys []string) error
	PasteText(ctx context.Context, pane TmuxPane, text string) error
	KillPane(ctx context.Context, pane TmuxPane) error
	SelectLayout(ctx context.Context, pane TmuxPane, layout string) error
}
