package runtime

import "fmt"

func ValidateDefinition(def Definition) error {
	if def.ID == "" {
		return fmt.Errorf("workflow id is required")
	}
	if def.Start == "" {
		return fmt.Errorf("start node is required")
	}
	if _, ok := def.Nodes[def.Start]; !ok {
		return fmt.Errorf("start node %q is not defined", def.Start)
	}

	hasTerminal := false
	for id, node := range def.Nodes {
		if id != node.ID {
			return fmt.Errorf("node key %q does not match id %q", id, node.ID)
		}
		if node.Kind == NodeKindAgent && node.Prompt.SkillPath == "" &&
			node.Prompt.Template == "" &&
			node.Prompt.Static == "" {
			return fmt.Errorf("agent node %q has no prompt", id)
		}
		if node.Terminal || node.Kind == NodeKindDone {
			hasTerminal = true
		}
	}
	for _, edge := range def.Edges {
		from, ok := def.Nodes[edge.From]
		if !ok {
			return fmt.Errorf("edge from %q is not defined", edge.From)
		}
		if _, ok := def.Nodes[edge.To]; !ok {
			return fmt.Errorf("edge to %q is not defined", edge.To)
		}
		if edge.Outcome != "" && !from.Contract.AllowsOutcome(edge.Outcome) {
			return fmt.Errorf(
				"edge outcome %q is not declared by node %q",
				edge.Outcome,
				edge.From,
			)
		}
	}
	if !hasTerminal {
		return fmt.Errorf("definition has no terminal node")
	}
	if def.ResultParser == nil {
		return fmt.Errorf("result parser is required")
	}
	if def.ResultConverter == nil {
		return fmt.Errorf("result converter is required")
	}
	if def.PolicySpec.Validate != nil && len(def.PolicySpec.Defaults) > 0 {
		if err := def.PolicySpec.Validate(def.PolicySpec.Defaults); err != nil {
			return fmt.Errorf("default policy invalid: %w", err)
		}
	}
	return nil
}
