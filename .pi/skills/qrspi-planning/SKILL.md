---
name: qrspi-planning
description: Ticket-level QRSPI planning using Hermes-managed isolated Pi workers and durable plan artifacts.
---

# Hermes-managed QRSPI

Hermes owns worker lifecycle and decides what happens next. Pi performs one bounded stage in an isolated context. The filesystem under `thoughts/...` is durable truth; Hermes operational state, process handles, gateway settings, and credentials are not artifacts.

## Worker contract

Every worker starts by reading this skill, its active stage skill, the plan `AGENTS.md`, and only the stage artifacts named there. It creates or updates the durable stage artifact, then ends with a normal final response naming the artifact and the smallest decision for Hermes or the lead. In a managed opaque-settlement run, do not invoke `vamos hermes pi done`, emit semantic result YAML, select a successor, or hand-write a machine-routing document.

`PI_SESSION_ID` is Pi's injected session identity and `VAMOS_PLAN_DIR` is transient plan context. The stop-bound extension captures the actual final assistant response and qualifying YAML/YML fences as immutable opaque evidence. Hermes alone owns lifecycle, process steering, and any successor decision. `pi done` remains an explicit human/operator recovery path for manual sessions, not a normal managed-worker action.

Do not put gateway URLs, credentials, process IDs, locks, run state, or absolute host paths in durable artifacts.

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
