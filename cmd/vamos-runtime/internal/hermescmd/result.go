package hermescmd

import (
	"fmt"
	"os"
	"path/filepath"

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
