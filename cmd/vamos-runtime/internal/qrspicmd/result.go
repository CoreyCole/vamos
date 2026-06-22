package qrspicmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi/semantic"
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
	semCtx := semantic.Context{
		WorkflowType:      qrspi.AgentChatWorkflowType,
		State:             manager.Workflow,
		ExpectedNodeID:    ctx.ExpectedNodeID,
		Source:            semantic.SourceCLIChildSession,
		PlanDir:           manager.CanonicalPlanDir,
		ImplementationCwd: manager.ImplementationCwd,
		PlanningCwd:       manager.SourceCwd,
		RunID:             ctx.RunID,
	}
	if !managerAware {
		semCtx.PlanDir = ""
		semCtx.ImplementationCwd = ""
	}
	applied, err := semantic.Apply(nil, semantic.ApplyInput{
		Definition:   def,
		RawOutput:    output,
		ParseContext: ctx,
		Context:      semCtx,
	})
	if err != nil {
		return ParsedDecision{}, GraphContractError(err)
	}
	rawYAML, _ := qrspi.ExtractQRSPIResultYAML(output)
	return ParsedDecision{Result: applied.WorkflowResult, Decision: applied.Decision, RawYAML: rawYAML, Normalizations: resultNormalizations(applied.Normalizations)}, nil
}

func resultNormalizations(in []semantic.Normalization) []ResultNormalization {
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
