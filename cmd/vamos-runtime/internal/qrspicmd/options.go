package qrspicmd

import (
	"context"
	"time"

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

type PromptFileOptions struct {
	StateFile string
	NodeID    string
	Timestamp time.Time
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
