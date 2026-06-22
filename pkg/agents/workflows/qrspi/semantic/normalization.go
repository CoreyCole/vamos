package semantic

import (
	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

func NormalizeContextAware(result qrspi.Result, context Context) (qrspi.Result, []Normalization) {
	if result.Status != string(wruntime.StatusComplete) || !positiveOutcome(result.Outcome) || result.Stage != string(qrspi.NodeReviewPlan) {
		return result, nil
	}
	target, ok := ReviewPlanPositiveOutcome(context)
	if !ok || result.Outcome == string(target) {
		return result, nil
	}
	original := result.Outcome
	result.Outcome = string(target)
	return result, []Normalization{reviewPlanOutcomeNormalization(original, string(target))}
}

func NormalizeWorkflowResultContextAware(result wruntime.WorkflowResult, context Context) (wruntime.WorkflowResult, []Normalization) {
	if result.Status != wruntime.StatusComplete || !positiveOutcome(string(result.Outcome)) || result.SourceNodeID != qrspi.NodeReviewPlan {
		return result, nil
	}
	target, ok := ReviewPlanPositiveOutcome(context)
	if !ok || result.Outcome == target {
		return result, nil
	}
	original := string(result.Outcome)
	result.Outcome = target
	return result, []Normalization{reviewPlanOutcomeNormalization(original, string(target))}
}

func reviewPlanOutcomeNormalization(original, target string) Normalization {
	return Normalization{
		Field:     "outcome",
		Original:  original,
		Canonical: target,
		Reason:    "review-plan positive result normalized from workflow context",
	}
}

func qrspiNormalizations(in []qrspi.Normalization) []Normalization {
	out := make([]Normalization, 0, len(in))
	for _, n := range in {
		out = append(out, Normalization{Field: n.Field, Original: n.Original, Canonical: n.Canonical, Reason: n.Reason})
	}
	return out
}
