package qrspicmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type ParsedDecision struct {
	Result         wruntime.WorkflowResult     `json:"result"`
	Decision       wruntime.TransitionDecision `json:"decision"`
	RawYAML        string                      `json:"rawYaml,omitempty"`
	Normalizations []ResultNormalization       `json:"normalizations,omitempty"`
}

type InitOverrides struct {
	NodeID            string
	ImplementationCwd string
}

func Definition() (wruntime.Definition, error) {
	return qrspi.Definition()
}

func InitialManagerState(planDir, projectRoot string, policy json.RawMessage) (ManagerState, error) {
	def, err := Definition()
	if err != nil {
		return ManagerState{}, err
	}
	workflow, err := wruntime.InitialState(def, policy)
	if err != nil {
		return ManagerState{}, err
	}
	canonicalPlanDir, err := CanonicalPlanDir(projectRoot, planDir)
	if err != nil {
		return ManagerState{}, err
	}
	repoID, err := RepoID(projectRoot)
	if err != nil {
		return ManagerState{}, err
	}
	workflow.ExecutionCwd = projectRoot
	return ManagerState{
		SchemaVersion:    schemaVersion,
		RepoID:           repoID,
		CanonicalPlanDir: canonicalPlanDir,
		SourceCwd:        projectRoot,
		Workflow:         workflow,
	}, nil
}

func ApplyInitOverrides(state *ManagerState, opts InitOverrides) error {
	if state == nil {
		return nil
	}
	if nodeID := wruntime.NodeID(strings.TrimSpace(opts.NodeID)); nodeID != "" {
		def, err := Definition()
		if err != nil {
			return err
		}
		if _, ok := def.Nodes[nodeID]; !ok {
			return fmt.Errorf("node %q is not in QRSPI definition", nodeID)
		}
		state.Workflow.CurrentNodeID = nodeID
	}
	if implementationCwd := strings.TrimSpace(opts.ImplementationCwd); implementationCwd != "" {
		state.ImplementationCwd = implementationCwd
	}
	return nil
}

func ParseValidateDecide(output string, state wruntime.State, ctx wruntime.ParseContext) (ParsedDecision, error) {
	return parseValidateDecide(output, ManagerState{Workflow: state}, ctx, false)
}

func ParseNormalizeValidateDecide(output string, manager ManagerState, ctx wruntime.ParseContext) (ParsedDecision, error) {
	return parseValidateDecide(output, manager, ctx, true)
}

func parseValidateDecide(output string, manager ManagerState, ctx wruntime.ParseContext, managerAware bool) (ParsedDecision, error) {
	def, err := Definition()
	if err != nil {
		return ParsedDecision{}, err
	}
	ctx.WorkflowType = string(qrspi.AgentChatWorkflowType)
	if ctx.ExpectedNodeID == "" {
		ctx.ExpectedNodeID = manager.Workflow.CurrentNodeID
	}
	parsedAny, err := def.ResultParser.Parse(output, ctx)
	if err != nil {
		return ParsedDecision{}, err
	}
	parsed := parsedAny
	var norms []ResultNormalization
	if qrspiResult, ok := parsedAny.(qrspi.Result); ok {
		norms = append(norms, resultNormalizations(qrspiResult.Normalizations)...)
		if managerAware {
			var more []ResultNormalization
			qrspiResult, more = normalizeManagerAwarePositiveOutcome(qrspiResult, manager)
			norms = append(norms, more...)
			parsed = qrspiResult
		}
	}
	result, err := def.ResultConverter.ToWorkflowResult(parsed, ctx)
	if err != nil {
		return ParsedDecision{}, err
	}
	if err := wruntime.ValidateWorkflowResult(def, manager.Workflow, result); err != nil {
		return ParsedDecision{}, GraphContractError(err)
	}
	if err := qrspi.ValidateOutcomeArtifacts(result); err != nil {
		return ParsedDecision{}, err
	}
	decision, err := wruntime.DecideTransition(def, manager.Workflow, result)
	if err != nil {
		return ParsedDecision{}, GraphContractError(err)
	}
	rawYAML, _ := qrspi.ExtractQRSPIResultYAML(output)
	return ParsedDecision{Result: result, Decision: decision, RawYAML: rawYAML, Normalizations: norms}, nil
}

func normalizeManagerAwarePositiveOutcome(parsed qrspi.Result, manager ManagerState) (qrspi.Result, []ResultNormalization) {
	if parsed.Status != string(wruntime.StatusComplete) || !positiveOutcome(parsed.Outcome) || parsed.Stage != string(qrspi.NodeReviewPlan) {
		return parsed, nil
	}
	target, ok := reviewPlanPositiveOutcome(manager)
	if !ok || parsed.Outcome == string(target) {
		return parsed, nil
	}
	original := parsed.Outcome
	parsed.Outcome = string(target)
	return parsed, []ResultNormalization{{
		Field:     "outcome",
		Original:  original,
		Canonical: string(target),
		Reason:    "review-plan positive result normalized from manager workspace context",
	}}
}

func positiveOutcome(outcome string) bool {
	switch strings.ReplaceAll(strings.ToLower(strings.TrimSpace(outcome)), "_", "-") {
	case "complete", "completed", "done", "success", "succeeded", "ok", "ready", "approved", "ready-to-plan", "ready-for-planning", "ready-to-implement", "ready-to-implementation", "ready-to-workspace", "ready-for-workspaces":
		return true
	default:
		return false
	}
}

func reviewPlanPositiveOutcome(manager ManagerState) (wruntime.ResultOutcome, bool) {
	if strings.TrimSpace(manager.ImplementationCwd) != "" || strings.Contains(filepath.ToSlash(manager.CanonicalPlanDir), "/reviews/") {
		return wruntime.OutcomeReadyForImplement, true
	}
	return wruntime.OutcomeReadyForWorkspace, true
}

func resultNormalizations(in []qrspi.Normalization) []ResultNormalization {
	out := make([]ResultNormalization, 0, len(in))
	for _, n := range in {
		out = append(out, ResultNormalization{Field: n.Field, Original: n.Original, Canonical: n.Canonical, Reason: n.Reason})
	}
	return out
}

func UpdateImplementationCwd(state ManagerState, result wruntime.WorkflowResult) ManagerState {
	implementation := strings.TrimSpace(qrspi.WorkflowResultImplementationWorkspace(result))
	if implementation != "" {
		state.ImplementationCwd = implementation
	}
	return state
}

func CorrectionPrompt(err error, attempt int) string {
	return qrspi.QRSPIResultParser{}.CorrectionPrompt(err, attempt)
}

func GraphContractError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("canonical QRSPI graph rejected result: %w", err)
}
