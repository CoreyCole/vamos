# Background Pi stage delegation

Use this when the human asks to delegate QRSPI stages to Pi/background agents or to auto-advance stages without approval prompts.

## Core rule

Every Pi prompt for the next stage must include the full fenced `qrspi_result` YAML from the immediately previous stage verbatim. Do not summarize it. The prior result carries:

- `stage`, `status`, and `outcome`
- `workspace` / `workspace_metadata`
- `policy`
- primary `artifact`
- all `artifacts`
- exact `next.steps`

This prevents later stage agents from losing the implementation workspace, graph route, review/verify artifacts, or the user's auto-advance policy.

## Orchestrator loop

1. Start the stage in a background Pi process with `notify_on_complete=true`.
2. When it completes, read the full process log, not only the notification tail.
3. Copy the complete `qrspi_result` block into the next prompt.
4. Start the next graph-safe stage immediately unless the result is `needs_human`, `blocked`, `error`, invalid, or hits an explicit safety/lost-work gate.
5. For implementation `status: handoff`, launch `/q-resume` from the new handoff path; do not pause just because a handoff exists.
6. Only ask the human for real human context, tradeoffs, manual testing, safety decisions, or merge confirmation.

## Prompt skeleton

```text
You are the [next-stage] stage subagent for a QRSPI workflow.

Run from cwd [absolute cwd].

Task: [stage-specific task and primary artifact path]

Follow these skills exactly:
- First read /Users/swarm/dotfiles/context/vamos/.pi/skills/qrspi-planning/SKILL.md
- Then read /Users/swarm/dotfiles/context/vamos/.pi/skills/[next-stage]/SKILL.md
- Load all required artifacts from the recorded workspace.

User instruction for this run:
- Delegate each phase to Pi background agents.
- Do not request approval for advancing the stage.
- Ask only if real human context, safety, blocker, or manual confirmation is required.
- Include full qrspi_result from previous stage in your prompt/context and preserve its transition.

IMPORTANT previous stage result:

```yaml
[full prior qrspi_result here]
```

When complete, emit required fenced YAML qrspi_result followed by the concise stage summary.
```

## Implementation-loop notes

- `q-implement` and `q-resume` execute exactly one unchecked work chunk per invocation.
- Intermediate implementation results use `status: handoff` and route to `q-resume` with the new handoff.
- Final implementation result routes to implementation review.
- `q-verify` may legitimately return `needs_human` for manual testing; do not invent human confirmation.
