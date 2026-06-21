package qrspicmd

import (
	"context"
	"time"
)

type InitOptions struct {
	PlanDir     string
	ProjectRoot string
	PolicyFile  string
	Force       bool
}

type RunChildOptions struct {
	PlanDir      string
	Stage        string
	Cwd          string
	PromptFile   string
	StateFile    string
	Split        string
	ManagerRunID string
	Timeout      time.Duration
}

type ValidateResultOptions struct {
	Stage      string
	StateFile  string
	ResultFile string
	PlanDir    string
	RunID      string
	SessionID  string
}

type DecideNextOptions struct {
	StateFile  string
	ResultFile string
	PlanDir    string
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

// ChildRunner starts visible child QRSPI sessions and observes their result file.
type ChildRunner interface {
	Start(ctx context.Context, req ChildRunRequest) (ChildRun, error)
	Wait(ctx context.Context, run ChildRun) (ChildRunResult, error)
}

// TmuxClient adapts tmux pane operations for visible child sessions.
type TmuxClient interface {
	SplitPane(ctx context.Context, req TmuxSplitRequest) (TmuxPane, error)
	SendKeys(ctx context.Context, pane TmuxPane, keys []string) error
}
