# Delegated Pi Background QRSPI Orchestration

Session pattern captured from approved-PR review-ticket work.

## Durable orchestration rule

When a human asks for QRSPI phases to be delegated to Pi/background agents:

1. Start each graph-safe next stage immediately in a new background Pi process.
2. Do not ask the human for approval just to advance stages.
3. Pause only for genuine human-context blockers, safety/lost-work risk, invalid artifacts, or an explicit manual-test/human-gate requirement.
4. Include the full fenced `qrspi_result` YAML from the immediately previous stage verbatim in the next-stage prompt.

## Why full `qrspi_result` matters

Do not pass only a summary or artifact path. The next agent needs the exact:

- `stage`, `status`, and `outcome`
- `workspace_metadata.plan_workspace`
- `workspace_metadata.implementation_workspace`
- Graphite branch metadata
- `policy`
- primary `artifact`
- full `artifacts` list
- intended `next.steps`

This prevents workspace mixups, stale branch assumptions, and lost QRSPI graph state when each stage runs in a fresh Pi context.

## Prompt skeleton

```text
You are the [stage] stage subagent for a QRSPI workflow.

Run from cwd [implementation_or_plan_workspace].

Task: [specific stage task/artifact].

Follow these skills exactly:
- First read /Users/swarm/dotfiles/context/vamos/.pi/skills/qrspi-planning/SKILL.md
- Then read /Users/swarm/dotfiles/context/vamos/.pi/skills/[stage]/SKILL.md
- Load required artifacts from [plan/workspace].

User instruction for this run:
- Delegate each phase to Pi background agents; do not request approval for advancing the stage; only ask if real human context is needed.
- Include full qrspi_result from previous stage in your prompt/context and preserve its transition.

IMPORTANT previous stage result:

```yaml
qrspi_result:
  ...verbatim previous result...
```

When complete, emit required fenced YAML qrspi_result followed by the stage-specific concise summary.
```

## Implementation-loop nuance

For `q-implement` / `q-resume`, each background process should execute exactly one unchecked implementation checkpoint, verify it, update the plan checkbox, create/modify the Graphite slice branch if applicable, write/amend the handoff, and stop. The parent orchestrator then starts the next `q-resume` background process using the new handoff and the full prior `qrspi_result`.
