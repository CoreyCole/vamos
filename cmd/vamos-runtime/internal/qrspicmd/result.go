package qrspicmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type ParsedDecision struct {
	Result   wruntime.WorkflowResult     `json:"result"`
	Decision wruntime.TransitionDecision `json:"decision"`
	RawYAML  string                      `json:"rawYaml,omitempty"`
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
	def, err := Definition()
	if err != nil {
		return ParsedDecision{}, err
	}
	ctx.WorkflowType = string(qrspi.AgentChatWorkflowType)
	if ctx.ExpectedNodeID == "" {
		ctx.ExpectedNodeID = state.CurrentNodeID
	}
	parsed, err := def.ResultParser.Parse(output, ctx)
	if err != nil {
		return ParsedDecision{}, err
	}
	result, err := def.ResultConverter.ToWorkflowResult(parsed, ctx)
	if err != nil {
		return ParsedDecision{}, err
	}
	if err := wruntime.ValidateWorkflowResult(def, state, result); err != nil {
		return ParsedDecision{}, GraphContractError(err)
	}
	if err := qrspi.ValidateOutcomeArtifacts(result); err != nil {
		return ParsedDecision{}, err
	}
	decision, err := wruntime.DecideTransition(def, state, result)
	if err != nil {
		return ParsedDecision{}, GraphContractError(err)
	}
	rawYAML, _ := qrspi.ExtractQRSPIResultYAML(output)
	return ParsedDecision{Result: result, Decision: decision, RawYAML: rawYAML}, nil
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
