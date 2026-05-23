# Agent Chat workflow authoring

Agent Chat workflows are graph-authoritative runtime definitions. A workflow definition declares nodes, result contracts, typed config, and transition edges. Agent output is parsed into a `WorkflowResult`; XML `<next>` is display/debug text only and never chooses the next node.

## Runtime shape

Workflow authors use the typed builder, while the server stores and runs a type-erased `runtime.Definition` so different workflow types can coexist in one registry.

```go
type Config struct {
    AutoMode          bool `json:"autoMode"`
    EnableFastPath    bool `json:"enableFastPath"`
    RetryLimit        int  `json:"retryLimit"`
}

def, err := runtime.New[Config]("example").
    Config(DefaultConfig(), ValidateConfig).
    Version("v1").
    Name("Example workflow").
    Start("question").
    Agent("question", runtime.PromptSpec{SkillPath: "~/.agents/skills/question/SKILL.md"}).
        Statuses(runtime.StatusComplete, runtime.StatusBlocked, runtime.StatusError).
        Outcomes(runtime.OutcomeComplete).
        RequiresPrimaryArtifact().
    Agent("review", runtime.PromptSpec{SkillPath: "~/.agents/skills/review/SKILL.md"}).
        Statuses(runtime.StatusComplete, runtime.StatusNeedsHuman, runtime.StatusBlocked, runtime.StatusError).
        Outcomes(runtime.OutcomeReadyForHumanReview, runtime.OutcomeNeedsFollowup).
        RequiresPrimaryArtifact().
    HumanReview("human-review", "approve review").
        Statuses(runtime.StatusComplete, runtime.StatusBlocked, runtime.StatusError).
        Outcomes(runtime.OutcomeComplete).
        AutoApprovable(false).
    Done("done").
    From("question").On(runtime.OutcomeComplete).GoTo("review").
    From("review").On(runtime.OutcomeNeedsFollowup).GoTo("question").
    From("review").On(runtime.OutcomeReadyForHumanReview).GoTo("human-review").
    From("human-review").On(runtime.OutcomeComplete).GoTo("done").
    ResultParser(MyParser{}).
    ResultConverter(MyConverter{}).
    Build()
```

`Build()` validates that every edge references known nodes and that outcome edges are declared by the source node contract.

## Status vs outcome

`status` is lifecycle:

- `complete` selects a graph edge by explicit `outcome` plus optional config predicate.
- `needs_human`, `blocked`, and `error` stop before edge selection.
- `done` marks terminal completion where allowed.

`outcome` is branch intent. It is required for `status=complete` and must be declared by the current node's `Outcomes(...)` contract. QRSPI uses outcomes such as `ready-for-workspace`, `needs-review-research`, and `needs-followup` so review branches are explicit and testable.

## Config predicates

Use typed config predicates when the graph branches on policy or mode:

```go
func FastPathEnabled(ctx runtime.TypedTransitionContext[Config]) bool {
    return ctx.Config.EnableFastPath
}

func FastPathDisabled(ctx runtime.TypedTransitionContext[Config]) bool {
    return !ctx.Config.EnableFastPath
}

builder.From("draft").When(FastPathEnabled).GoTo("publish")
builder.From("draft").When(FastPathDisabled).GoTo("review")
```

The runtime decodes the persisted config through the definition's config spec before evaluating predicates. Workflow authors do not need raw JSON hooks in transition code.

## Human reviews and auto mode

Human review nodes are normal graph nodes with `NodeKindHumanReview`. `AutoApprovable(true)` means a generic auto-mode config may bypass that gate; `AutoApprovable(false)` always waits for a human. QRSPI marks planning outline review auto-approvable but final implementation review non-auto-approvable.

## Agent result XML

Agent-facing workflows can use XML footers like QRSPI. The parser converts XML into `WorkflowResult`; the graph, not `<next>`, selects the following node.

```xml
<qrspi-result>
  <stage>review-plan</stage>
  <status>complete</status>
  <outcome>ready-for-workspace</outcome>
  <summary>
    <plan-goal>Make QRSPI runtime-safe and graph-validated.</plan-goal>
    <stage-completed>Plan review passed after doc fixes.</stage-completed>
    <key-decisions>Proceed to workspace prep.</key-decisions>
  </summary>
  <artifact>thoughts/example/reviews/plan-review/review.md</artifact>
  <next>/q-workspace thoughts/example/plan.md</next>
</qrspi-result>
```

Keep `<next>` compatible with the graph for readability, but do not rely on it for routing. If it disagrees with the selected edge, the service can record a warning or start correction without executing the command.

## Graph inspection

Use renderers to make definitions reviewable:

```go
mermaid := runtime.RenderMermaid(def)
table := runtime.RenderTransitionTable(def)
```

`RenderMermaid` emits a `flowchart TD` diagram. `RenderTransitionTable` emits a markdown table with source node, condition, and target. These are useful in workflow docs and regression tests because they expose branches that are otherwise hidden in builder chains.
