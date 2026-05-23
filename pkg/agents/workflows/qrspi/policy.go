package qrspi

import (
	"encoding/json"
	"fmt"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type Policy struct {
	AutoMode                bool `json:"autoMode"                xml:"autoMode"`
	EnablePlanReviews       bool `json:"enablePlanReviews"       xml:"enablePlanReviews"`
	InvalidResultRetryLimit int  `json:"invalidResultRetryLimit" xml:"invalidResultRetryLimit"`
}

type Config = Policy

func DefaultPolicy() Policy {
	return Policy{AutoMode: false, EnablePlanReviews: true, InvalidResultRetryLimit: 1}
}

func DefaultConfig() Config { return DefaultPolicy() }

func (config Config) IsAutoMode() bool { return config.AutoMode }

func PolicySpec() wruntime.PolicySpec {
	defaults, _ := json.Marshal(DefaultPolicy())
	return wruntime.PolicySpec{Defaults: defaults, Validate: ValidatePolicyJSON}
}

func ValidateConfig(config Config) error {
	if config.InvalidResultRetryLimit < 0 {
		return fmt.Errorf("invalidResultRetryLimit must be non-negative")
	}
	return nil
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
		_ = json.Unmarshal(raw, &policy)
	}
	return policy
}

func ConfigPlanReviewsEnabled(ctx wruntime.TypedTransitionContext[Config]) bool {
	return ctx.Config.EnablePlanReviews
}

func ConfigPlanReviewsDisabled(ctx wruntime.TypedTransitionContext[Config]) bool {
	return !ctx.Config.EnablePlanReviews
}
