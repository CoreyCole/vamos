package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Builder[TConfig any] struct {
	def    Definition
	err    error
	lastID NodeID
}

func New[TConfig any](id WorkflowID) *Builder[TConfig] {
	return &Builder[TConfig]{
		def: Definition{ID: id, Version: "v1", Nodes: map[NodeID]Node{}},
	}
}

func (b *Builder[TConfig]) Version(version string) *Builder[TConfig] {
	b.def.Version = version
	return b
}

func (b *Builder[TConfig]) Name(name string) *Builder[TConfig] {
	b.def.Name = name
	return b
}

func (b *Builder[TConfig]) Start(id NodeID) *Builder[TConfig] {
	b.def.Start = id
	return b
}

func (b *Builder[TConfig]) Agent(id NodeID, prompt PromptSpec) *Builder[TConfig] {
	return b.node(
		Node{ID: id, DisplayName: string(id), Kind: NodeKindAgent, Prompt: prompt},
	)
}

func (b *Builder[TConfig]) HumanReview(id NodeID, reason string) *Builder[TConfig] {
	return b.node(
		Node{
			ID:          id,
			DisplayName: string(id),
			Kind:        NodeKindHumanReview,
			Prompt:      PromptSpec{Static: reason},
		},
	)
}

func (b *Builder[TConfig]) Done(id NodeID) *Builder[TConfig] {
	return b.node(
		Node{
			ID:          id,
			DisplayName: string(id),
			Kind:        NodeKindDone,
			Terminal:    true,
			Contract: ResultContract{
				Statuses: []ResultStatus{StatusDone, StatusComplete},
			},
		},
	)
}

func (b *Builder[TConfig]) Statuses(statuses ...ResultStatus) *Builder[TConfig] {
	node, ok := b.lastNode()
	if !ok {
		return b
	}
	node.Contract.Statuses = append([]ResultStatus(nil), statuses...)
	b.def.Nodes[node.ID] = node
	return b
}

func (b *Builder[TConfig]) Outcomes(outcomes ...ResultOutcome) *Builder[TConfig] {
	node, ok := b.lastNode()
	if !ok {
		return b
	}
	node.Contract.Outcomes = append([]ResultOutcome(nil), outcomes...)
	b.def.Nodes[node.ID] = node
	return b
}

func (b *Builder[TConfig]) RequiresPrimaryArtifact() *Builder[TConfig] {
	node, ok := b.lastNode()
	if !ok {
		return b
	}
	node.Contract.PrimaryArtifactRequired = true
	b.def.Nodes[node.ID] = node
	return b
}

func (b *Builder[TConfig]) AutoApprovable(auto bool) *Builder[TConfig] {
	node, ok := b.lastNode()
	if !ok {
		return b
	}
	if node.Kind != NodeKindHumanReview {
		b.err = fmt.Errorf("node %q is not a human review node", node.ID)
		return b
	}
	node.AutoApprovable = auto
	b.def.Nodes[node.ID] = node
	return b
}

func (b *Builder[TConfig]) Edge(from, to NodeID) *Builder[TConfig] {
	b.def.Edges = append(b.def.Edges, Edge{From: from, To: to})
	return b
}

func (b *Builder[TConfig]) HumanGate(from, to NodeID, reason string) *Builder[TConfig] {
	b.def.Edges = append(
		b.def.Edges,
		Edge{From: from, To: to, Gate: GateSpec{Human: true, Reason: reason}},
	)
	return b
}

func (b *Builder[TConfig]) From(node NodeID) *TransitionBuilder[TConfig] {
	return &TransitionBuilder[TConfig]{builder: b, from: node}
}

func (b *Builder[TConfig]) ResultParser(parser ResultParser) *Builder[TConfig] {
	b.def.ResultParser = parser
	return b
}

func (b *Builder[TConfig]) ResultConverter(converter ResultConverter) *Builder[TConfig] {
	b.def.ResultConverter = converter
	return b
}

func (b *Builder[TConfig]) PolicySpec(spec PolicySpec) *Builder[TConfig] {
	b.def.PolicySpec = spec
	return b
}

func (b *Builder[TConfig]) Config(
	defaults TConfig,
	validate func(TConfig) error,
) *Builder[TConfig] {
	raw, err := json.Marshal(defaults)
	if err != nil {
		b.err = err
		return b
	}
	b.def.PolicySpec = PolicySpec{
		Defaults: raw,
		Decode: func(raw json.RawMessage) (any, error) {
			if len(raw) == 0 || string(raw) == "null" {
				raw = b.def.PolicySpec.Defaults
			}
			var config TConfig
			if err := json.Unmarshal(raw, &config); err != nil {
				return nil, err
			}
			if validate != nil {
				if err := validate(config); err != nil {
					return nil, err
				}
			}
			return config, nil
		},
		Validate: func(raw json.RawMessage) error {
			if len(raw) == 0 || string(raw) == "null" {
				raw = b.def.PolicySpec.Defaults
			}
			var config TConfig
			if err := json.Unmarshal(raw, &config); err != nil {
				return err
			}
			if validate != nil {
				return validate(config)
			}
			return nil
		},
	}
	return b
}

func (b *Builder[TConfig]) Build() (Definition, error) {
	if b.err != nil {
		return Definition{}, b.err
	}
	if err := ValidateDefinition(b.def); err != nil {
		return Definition{}, err
	}
	return b.def, nil
}

func (b *Builder[TConfig]) node(node Node) *Builder[TConfig] {
	if b.err != nil {
		return b
	}
	if node.ID == "" {
		b.err = fmt.Errorf("node id is required")
		return b
	}
	if _, exists := b.def.Nodes[node.ID]; exists {
		b.err = fmt.Errorf("duplicate node %q", node.ID)
		return b
	}
	b.def.Nodes[node.ID] = node
	b.lastID = node.ID
	return b
}

func (b *Builder[TConfig]) lastNode() (Node, bool) {
	if b.err != nil {
		return Node{}, false
	}
	if b.lastID == "" {
		b.err = fmt.Errorf("node modifier called before defining a node")
		return Node{}, false
	}
	node, ok := b.def.Nodes[b.lastID]
	if !ok {
		b.err = fmt.Errorf("last node %q is not defined", b.lastID)
		return Node{}, false
	}
	return node, true
}

type TransitionBuilder[TConfig any] struct {
	builder   *Builder[TConfig]
	from      NodeID
	outcome   ResultOutcome
	predicate EdgePredicate
}

func (t *TransitionBuilder[TConfig]) On(
	outcome ResultOutcome,
) *TransitionTargetBuilder[TConfig] {
	return &TransitionTargetBuilder[TConfig]{
		transition: &TransitionBuilder[TConfig]{
			builder: t.builder,
			from:    t.from,
			outcome: outcome,
		},
	}
}

func (t *TransitionBuilder[TConfig]) When(
	predicate func(TypedTransitionContext[TConfig]) bool,
) *TransitionTargetBuilder[TConfig] {
	wrapped := func(ctx TransitionContext) bool {
		config, ok := ctx.Config.(TConfig)
		if !ok {
			return false
		}
		return predicate(
			TypedTransitionContext[TConfig]{
				Config: config,
				State:  ctx.State,
				Result: ctx.Result,
			},
		)
	}
	return &TransitionTargetBuilder[TConfig]{
		transition: &TransitionBuilder[TConfig]{
			builder:   t.builder,
			from:      t.from,
			predicate: wrapped,
		},
	}
}

type TransitionTargetBuilder[TConfig any] struct {
	transition *TransitionBuilder[TConfig]
}

func (t *TransitionTargetBuilder[TConfig]) GoTo(node NodeID) *Builder[TConfig] {
	tr := t.transition
	tr.builder.def.Edges = append(
		tr.builder.def.Edges,
		Edge{From: tr.from, To: node, Outcome: tr.outcome, Predicate: tr.predicate},
	)
	return tr.builder
}

func RenderMermaid(def Definition) string {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	for _, edge := range def.Edges {
		label := edgeLabel(edge)
		if label == "" {
			fmt.Fprintf(&b, "  %s --> %s\n", edge.From, edge.To)
			continue
		}
		fmt.Fprintf(&b, "  %s -- %s --> %s\n", edge.From, label, edge.To)
	}
	return b.String()
}

func RenderTransitionTable(def Definition) string {
	var b strings.Builder
	b.WriteString("| From | Condition | To |\n|---|---|---|\n")
	for _, edge := range def.Edges {
		condition := edgeLabel(edge)
		if condition == "" {
			condition = "default"
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", edge.From, condition, edge.To)
	}
	return b.String()
}

func edgeLabel(edge Edge) string {
	parts := make([]string, 0, 2)
	if edge.Outcome != "" {
		parts = append(parts, "outcome="+string(edge.Outcome))
	}
	if edge.Predicate != nil {
		parts = append(parts, "predicate")
	}
	if edge.Gate.Human {
		parts = append(parts, "human")
	}
	return strings.Join(parts, ", ")
}
