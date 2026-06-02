package qrspi

import (
	"encoding/json"
	"fmt"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type AdvanceMode string

const (
	AdvanceModeDiscuss   AdvanceMode = "discuss"
	AdvanceModeGuided    AdvanceMode = "guided"
	AdvanceModeAutopilot AdvanceMode = "autopilot"
)

type Policy struct {
	AdvanceMode             AdvanceMode `json:"advanceMode,omitempty" xml:"advanceMode"`
	AutoMode                bool        `json:"autoMode"                xml:"autoMode"`
	EnablePlanReviews       bool        `json:"enablePlanReviews"       xml:"enablePlanReviews"`
	InvalidResultRetryLimit int         `json:"invalidResultRetryLimit" xml:"invalidResultRetryLimit"`
}

type Config = Policy

func DefaultPolicy() Policy {
	return Policy{AdvanceMode: AdvanceModeGuided, EnablePlanReviews: true, InvalidResultRetryLimit: 1}
}

func DefaultConfig() Config { return DefaultPolicy() }

func (config Config) IsAutoMode() bool {
	return config.EffectiveAdvanceMode() == AdvanceModeAutopilot
}

func (config Config) EffectiveAdvanceMode() AdvanceMode {
	switch config.AdvanceMode {
	case AdvanceModeDiscuss, AdvanceModeGuided, AdvanceModeAutopilot:
		return config.AdvanceMode
	case "":
		if config.AutoMode {
			return AdvanceModeAutopilot
		}
		return AdvanceModeGuided
	default:
		return AdvanceModeGuided
	}
}

func (config Config) ShouldStartNonHumanEdges() bool {
	return config.EffectiveAdvanceMode() != AdvanceModeDiscuss
}

func PolicySpec() wruntime.PolicySpec {
	defaults, _ := json.Marshal(DefaultPolicy())
	return wruntime.PolicySpec{Defaults: defaults, Validate: ValidatePolicyJSON}
}

func ValidateConfig(config Config) error {
	if config.InvalidResultRetryLimit < 0 {
		return fmt.Errorf("invalidResultRetryLimit must be non-negative")
	}
	switch config.AdvanceMode {
	case "", AdvanceModeDiscuss, AdvanceModeGuided, AdvanceModeAutopilot:
		return nil
	default:
		return fmt.Errorf("advanceMode must be one of discuss, guided, or autopilot")
	}
}

func ValidatePolicyJSON(raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var policy Policy
	if err := json.Unmarshal(raw, &policy); err != nil {
		return err
	}
	return ValidateConfig(policy)
}

func ParsePolicy(raw json.RawMessage) Policy {
	policy := DefaultPolicy()
	if len(raw) > 0 && string(raw) != "null" {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		_ = json.Unmarshal(raw, &policy)
		if _, ok := fields["advanceMode"]; !ok {
			policy.AdvanceMode = ""
		}
	}
	return policy
}

func ConfigPlanReviewsEnabled(ctx wruntime.TypedTransitionContext[Config]) bool {
	return ctx.Config.EnablePlanReviews
}

func ConfigPlanReviewsDisabled(ctx wruntime.TypedTransitionContext[Config]) bool {
	return !ctx.Config.EnablePlanReviews
}
