package hermescmd

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type (
	Outcome    string
	NextAction string
)

const (
	OutcomeComplete            Outcome    = "complete"
	OutcomeHandoff             Outcome    = "handoff"
	OutcomeNeedsHuman          Outcome    = "needs_human"
	OutcomeBlocked             Outcome    = "blocked"
	OutcomeError               Outcome    = "error"
	NextNone                   NextAction = "none"
	NextQuestion               NextAction = "question"
	NextResearch               NextAction = "research"
	NextDesign                 NextAction = "design"
	NextOutline                NextAction = "outline"
	NextPlan                   NextAction = "plan"
	NextWorkspace              NextAction = "workspace"
	NextImplement              NextAction = "implement"
	NextReview                 NextAction = "review"
	NextVerify                 NextAction = "verify"
	NextMilestoneQuestion      NextAction = "milestone-question"
	NextMilestoneResearch      NextAction = "milestone-research"
	NextMilestoneDesign        NextAction = "milestone-design"
	NextMilestoneCreateTickets NextAction = "milestone-create-tickets"
)

type CheckpointIdentity struct {
	Session       string `yaml:"session"        json:"session"`
	Plan          string `yaml:"plan"           json:"plan"`
	ManagerThread string `yaml:"manager_thread" json:"manager_thread"`
	FinalEntryID  string `yaml:"final_entry_id" json:"final_entry_id"`
}

// PiCheckpoint is the legacy immutable v2 managed completion fact. It remains
// readable for recovery and is not an opaque settlement.
type PiCheckpoint struct {
	Version            int `yaml:"version"                   json:"version"`
	CheckpointIdentity `yaml:",inline" json:",inline"`
	Outcome            Outcome        `yaml:"outcome"                   json:"outcome"`
	Next               NextAction     `yaml:"next"                      json:"next"`
	CreatedAt          time.Time      `yaml:"created_at"                json:"created_at"`
	Recommendation     string         `yaml:"recommendation,omitempty"  json:"recommendation,omitempty"`
	Summary            string         `yaml:"summary,omitempty"         json:"summary,omitempty"`
	Artifacts          []string       `yaml:"artifacts,omitempty"       json:"artifacts,omitempty"`
	RawResponse        string         `yaml:"raw_response,omitempty"    json:"raw_response,omitempty"`
	RawYAML            string         `yaml:"raw_yaml,omitempty"        json:"raw_yaml,omitempty"`
	IntentMetadata     map[string]any `yaml:"intent_metadata,omitempty" json:"intent_metadata,omitempty"`
	Diagnostics        []string       `yaml:"diagnostics,omitempty"     json:"diagnostics,omitempty"`
}

// CheckpointDeliveryAttempt is an immutable server delivery fact. Attempt
// records and the rebuildable projection deliberately cannot alter a checkpoint.
type CheckpointDeliveryAttempt struct {
	CheckpointIdentity `yaml:",inline" json:",inline"`
	Attempt            uint64    `yaml:"attempt"              json:"attempt"`
	CreatedAt          time.Time `yaml:"created_at"           json:"created_at"`
	Result             string    `yaml:"result"               json:"result"`
	Diagnostic         string    `yaml:"diagnostic,omitempty" json:"diagnostic,omitempty"`
}

type PiResult struct {
	Session            string     `yaml:"session"                       json:"session"`
	Plan               string     `yaml:"plan"                          json:"plan"`
	PreviousSession    string     `yaml:"previous_session,omitempty"    json:"previous_session,omitempty"`
	Outcome            Outcome    `yaml:"outcome"                       json:"outcome"`
	Next               NextAction `yaml:"next"                          json:"next"`
	Artifact           string     `yaml:"artifact,omitempty"            json:"artifact,omitempty"`
	Summary            string     `yaml:"summary"                       json:"summary"`
	RecommendedCommand string     `yaml:"recommended_command,omitempty" json:"recommended_command,omitempty"`
}

func ParseOutcome(value string) (Outcome, error) {
	outcome := Outcome(value)
	for _, candidate := range []Outcome{OutcomeComplete, OutcomeHandoff, OutcomeNeedsHuman, OutcomeBlocked, OutcomeError} {
		if outcome == candidate {
			return outcome, nil
		}
	}
	return "", fmt.Errorf(
		"unknown outcome %q; valid options: complete, handoff, needs_human, blocked, error",
		value,
	)
}

func ParseNextAction(value string) (NextAction, error) {
	next := NextAction(value)
	for _, candidate := range []NextAction{NextNone, NextQuestion, NextResearch, NextDesign, NextOutline, NextPlan, NextWorkspace, NextImplement, NextReview, NextVerify, NextMilestoneQuestion, NextMilestoneResearch, NextMilestoneDesign, NextMilestoneCreateTickets} {
		if next == candidate {
			return next, nil
		}
	}
	return "", fmt.Errorf(
		"unknown next action %q; valid options: none, question, research, design, outline, plan, workspace, implement, review, verify, milestone-question, milestone-research, milestone-design, milestone-create-tickets",
		value,
	)
}

func WritePiResult(ctx PlanContext, result PiResult) (string, error) {
	if result.Session == "" {
		return "", fmt.Errorf("session is required")
	}
	if result.Plan == "" {
		result.Plan = ctx.PlanRel
	}
	data, err := yaml.Marshal(result)
	if err != nil {
		return "", err
	}
	path := ResultPath(ctx.PlanDir, result.Session)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".result-*")
	if err != nil {
		return "", err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return "", err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(name, path); err != nil {
		return "", err
	}
	return path, nil
}

func ReadPiResult(planDir, sessionID string) (PiResult, error) {
	data, err := os.ReadFile(ResultPath(planDir, sessionID))
	if err != nil {
		return PiResult{}, err
	}
	var result PiResult
	if err := yaml.Unmarshal(data, &result); err != nil {
		return PiResult{}, err
	}
	return result, nil
}

func CanonicalPiCheckpoint(checkpoint PiCheckpoint) ([]byte, error) {
	if checkpoint.Version == 0 {
		checkpoint.Version = 2
	}
	if checkpoint.Version != 2 {
		return nil, fmt.Errorf("unsupported checkpoint version %d", checkpoint.Version)
	}
	if err := ValidateSafeComponent(checkpoint.Session); err != nil {
		return nil, fmt.Errorf("checkpoint session: %w", err)
	}
	if err := ValidateSafeComponent(checkpoint.ManagerThread); err != nil {
		return nil, fmt.Errorf("checkpoint manager thread: %w", err)
	}
	if err := ValidateSafeComponent(checkpoint.FinalEntryID); err != nil {
		return nil, fmt.Errorf("checkpoint final entry: %w", err)
	}
	if err := validateThoughtsRelativePlan(checkpoint.Plan); err != nil {
		return nil, err
	}
	if checkpoint.Next != NextNone {
		return nil, fmt.Errorf("checkpoint next must be none")
	}
	if _, err := ParseOutcome(string(checkpoint.Outcome)); err != nil {
		return nil, err
	}
	if checkpoint.CreatedAt.IsZero() {
		return nil, errors.New("checkpoint created_at is required")
	}
	return yaml.Marshal(checkpoint)
}

func WritePiCheckpoint(ctx PlanContext, checkpoint PiCheckpoint) (string, error) {
	if checkpoint.Plan == "" {
		checkpoint.Plan = ctx.PlanRel
	}
	if checkpoint.Plan != ctx.PlanRel {
		return "", fmt.Errorf(
			"checkpoint plan %q does not match plan %q",
			checkpoint.Plan,
			ctx.PlanRel,
		)
	}
	data, err := CanonicalPiCheckpoint(checkpoint)
	if err != nil {
		return "", err
	}
	return writePiCheckpointBytes(
		ctx.PlanDir,
		checkpoint.Session,
		checkpoint.FinalEntryID,
		data,
	)
}

func ReadPiCheckpoint(planDir, sessionID, finalEntryID string) (PiCheckpoint, error) {
	path, err := CheckpointPath(planDir, sessionID, finalEntryID)
	if err != nil {
		return PiCheckpoint{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return PiCheckpoint{}, err
	}
	var checkpoint PiCheckpoint
	if err := yaml.Unmarshal(data, &checkpoint); err != nil {
		return PiCheckpoint{}, err
	}
	canonical, err := CanonicalPiCheckpoint(checkpoint)
	if err != nil {
		return PiCheckpoint{}, err
	}
	if !bytes.Equal(data, canonical) {
		return PiCheckpoint{}, errors.New("checkpoint is not canonical")
	}
	if checkpoint.Session != sessionID || checkpoint.FinalEntryID != finalEntryID ||
		checkpoint.Plan != thoughtsRelative(planDir) {
		return PiCheckpoint{}, errors.New(
			"checkpoint payload does not match its immutable path identity",
		)
	}
	return checkpoint, nil
}

func validateThoughtsRelativePlan(plan string) error {
	plan = filepath.ToSlash(plan)
	if plan == "" || filepath.IsAbs(plan) {
		return errors.New("checkpoint plan must be thoughts-relative")
	}
	for _, component := range strings.Split(plan, "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf(
				"checkpoint plan must be a contained thoughts-relative path %q",
				plan,
			)
		}
	}
	return nil
}

func writePiCheckpointBytes(
	planDir, sessionID, finalEntryID string,
	data []byte,
) (string, error) {
	path, err := CheckpointPath(planDir, sessionID, finalEntryID)
	if err != nil {
		return "", err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(directory, ".checkpoint-*")
	if err != nil {
		return "", err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return "", err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := os.Link(tempName, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			existing, readErr := os.ReadFile(path)
			if readErr != nil {
				return "", readErr
			}
			if bytes.Equal(existing, data) {
				return path, nil
			}
			return "", fmt.Errorf("immutable checkpoint identity conflict at %q", path)
		}
		return "", err
	}
	if err := syncDirectory(directory); err != nil {
		_ = os.Remove(path)
		_ = syncDirectory(directory)
		return "", err
	}
	if err := os.Remove(tempName); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	_ = syncDirectory(directory)
	return path, nil
}

func syncDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func RecommendedCommand(ctx PlanContext, result PiResult) string {
	if result.Next == NextNone {
		return ""
	}
	return fmt.Sprintf(
		"vamos hermes pi start --plan %q --previous-session %q %q",
		ctx.PlanDir,
		result.Session,
		"Continue the "+string(result.Next)+" stage using the previous result.",
	)
}
