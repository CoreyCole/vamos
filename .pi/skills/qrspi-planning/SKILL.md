---
name: qrspi-planning
description: Ticket-level QRSPI planning using Hermes-managed isolated Pi workers and durable plan artifacts.
---

# Hermes-managed QRSPI

Hermes owns worker lifecycle and decides what happens next. Pi performs one bounded stage in an isolated context. The filesystem under `thoughts/...` is durable truth; Hermes operational state, process handles, gateway settings, and credentials are not artifacts.

## Worker contract

Every worker starts by reading this skill, its active stage skill, the plan `AGENTS.md`, and only the stage artifacts named there. It creates or updates the durable stage artifact, then records its conclusion through the CLI; never hand-write a machine-routing document.

```bash
vamos hermes pi done \
  --session "$VAMOS_PI_SESSION_ID" \
  --outcome <complete|handoff|needs_human|blocked|error> \
  --next <question|research|design|outline|plan|workspace|implement|review|verify|milestone-question|milestone-research|milestone-design|milestone-create-tickets|none> \
  [--artifact thoughts/.../artifact.md] \
  --summary $'1. Durable work completed.\n2. Verification or decision.\n3. Recommended Hermes follow-up.'
```

`VAMOS_PI_SESSION_ID` and `VAMOS_PLAN_DIR` are transient worker context. Do not put gateway URLs, credentials, process IDs, locks, run state, or absolute host paths in durable artifacts. `next` is a non-binding recommendation: Hermes or the lead engineer may choose a different safe task.

A successor starts with `vamos hermes pi start --plan <absolute-plan-dir> --previous-session <id> "<task>"`. Hermes reads `vamos hermes pi result` rather than copying prior worker output into prompts.

## Pipeline

1. Question — align goals and research agenda with the lead engineer.
1. Research — collect codebase facts; continue research if material facts remain unknown.
1. Design — propose direction and obtain human approval.
1. Outline — summarize direction and obtain approval before writing the outline unless Hermes was explicitly authorized to proceed.
1. Plan — write tactical slices, then planning review.
1. Workspace — prepare a safe copied implementation workspace; never use a git worktree.
1. Implement — one checked plan item per worker, with handoff recovery.
1. Review — review implementation or planning artifacts.
1. Verify — collect project-specific evidence before final human approval.

Preserve human gates, workspace/lost-work safety checks, implementation handoffs, test evidence, and milestone ticket-creation approval. Remove only duplicated orchestration: Pi does not choose a graph transition, validate a previous worker’s output, manage tmux, or persist retry/run state.

## Completion wording

Use concise numbered summaries. `handoff` means another isolated worker should continue the same bounded activity from the artifact. `needs_human`, `blocked`, and `error` stop autonomous continuation and state the smallest required decision or reproducible blocker. A completed planning or implementation artifact must name what Hermes should inspect next.

## Artifact rules

- Plan artifacts use `thoughts/...` relative identities.
- Plan-owned Pi and Hermes sessions live in `.vamos/sessions/{pi,hermes}`.
- The database is a disposable projection; disk remains authoritative.
- Implementation uses the workspace recorded by workspace preparation; do not create another copy.
- Implementation-review follow-up work stays in the reviewed implementation workspace and stacks on its reviewed head.
