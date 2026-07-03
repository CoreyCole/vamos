# 2026-07-02 Hermes-managed QRSPI run: webhook forwarding

Session-specific lessons from managing a multi-stage QRSPI workflow with background Pi processes.

## What worked

- Treat `[IMPORTANT: Background process ... completed]` messages as wake signals.
- Immediately read the full process log with the process tool; notification snippets may start mid-YAML.
- Preserve the previous full `qrspi_result` in every next prompt.
- Start the next graph-safe stage before a long prose update.
- Implementation handoffs are not human gates; route them to `/q-resume` with the exact new handoff artifact.

## Pitfalls observed

- Tool/process notifications may omit the opening ```yaml fence while the full log still contains coherent `qrspi_result:` YAML. Validate required fields before continuing.
- Reconstructing artifact paths from memory is dangerous. One missing `reviews/` path segment can point the next agent at a non-existent artifact.
- Direct-outline workflows may not have `design.md`, but downstream generic `next.steps` can still mention it. Preserve the YAML, but tell the next Pi prompt not to block on absent optional design artifacts when outline/plan/handoff are sufficient.
- Keep mode language explicit: Hermes background orchestration, not tmux q-manager panes or q_manager_child_wake.
