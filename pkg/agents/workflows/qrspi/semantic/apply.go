package semantic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type ApplyInput struct {
	Definition     wruntime.Definition
	RawOutput      string
	ParsedResult   *qrspi.Result
	WorkflowResult *wruntime.WorkflowResult
	ParseContext   wruntime.ParseContext
	Context        Context
}

type ApplyResult struct {
	Parsed         qrspi.Result
	WorkflowResult wruntime.WorkflowResult
	Decision       wruntime.TransitionDecision
	Effects        []Effect
	NextAction     NextAction
	Normalizations []Normalization
}

func Apply(ctx context.Context, input ApplyInput) (ApplyResult, error) {
	_ = ctx
	def := input.Definition
	if def.ID == "" {
		var err error
		def, err = qrspi.Definition()
		if err != nil {
			return ApplyResult{}, err
		}
	}
	parseCtx := input.ParseContext
	if parseCtx.WorkflowType == "" {
		if input.Context.WorkflowType != "" {
			parseCtx.WorkflowType = string(input.Context.WorkflowType)
		} else {
			parseCtx.WorkflowType = string(qrspi.AgentChatWorkflowType)
		}
	}
	if parseCtx.ExpectedNodeID == "" {
		if input.Context.ExpectedNodeID != "" {
			parseCtx.ExpectedNodeID = input.Context.ExpectedNodeID
		} else {
			parseCtx.ExpectedNodeID = input.Context.State.CurrentNodeID
		}
	}
	if parseCtx.RunID == "" {
		parseCtx.RunID = input.Context.RunID
	}
	state := input.Context.State
	parsed, workflowResult, norms, err := parseOrConvert(def, input, parseCtx)
	if err != nil {
		return ApplyResult{}, err
	}
	if input.WorkflowResult == nil {
		var more []Normalization
		parsed, more = NormalizeContextAware(parsed, input.Context)
		norms = append(norms, more...)
		workflowResult, err = def.ResultConverter.ToWorkflowResult(parsed, parseCtx)
		if err != nil {
			return ApplyResult{}, err
		}
	}
	if err := qrspi.ValidateOutcomeArtifacts(workflowResult); err != nil {
		return ApplyResult{}, err
	}
	decision, err := wruntime.DecideTransition(def, state, workflowResult)
	if err != nil {
		return ApplyResult{}, err
	}
	effects, err := DeriveEffects(state, workflowResult, decision, input.Context)
	if err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{
		Parsed:         parsed,
		WorkflowResult: workflowResult,
		Decision:       decision,
		Effects:        effects,
		NextAction:     ProjectNextAction(workflowResult, decision, effects),
		Normalizations: norms,
	}, nil
}

func parseOrConvert(def wruntime.Definition, input ApplyInput, parseCtx wruntime.ParseContext) (qrspi.Result, wruntime.WorkflowResult, []Normalization, error) {
	if input.WorkflowResult != nil {
		parsed, err := parsedFromWorkflowResult(*input.WorkflowResult)
		return parsed, *input.WorkflowResult, nil, err
	}
	if input.ParsedResult != nil {
		parsed := *input.ParsedResult
		return parsed, wruntime.WorkflowResult{}, qrspiNormalizations(parsed.Normalizations), nil
	}
	parsedAny, err := def.ResultParser.Parse(input.RawOutput, parseCtx)
	if err != nil {
		return qrspi.Result{}, wruntime.WorkflowResult{}, nil, err
	}
	parsed, ok := parsedAny.(qrspi.Result)
	if !ok {
		return qrspi.Result{}, wruntime.WorkflowResult{}, nil, fmt.Errorf("expected qrspi Result, got %T", parsedAny)
	}
	return parsed, wruntime.WorkflowResult{}, qrspiNormalizations(parsed.Normalizations), nil
}

func parsedFromWorkflowResult(result wruntime.WorkflowResult) (qrspi.Result, error) {
	if len(result.Raw) == 0 {
		return qrspi.Result{}, nil
	}
	var parsed qrspi.Result
	if err := json.Unmarshal(result.Raw, &parsed); err != nil {
		return qrspi.Result{}, err
	}
	return parsed, nil
}
