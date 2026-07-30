---
name: qrspi-planning
description: Ticket-level QRSPI planning using Hermes-managed isolated Pi workers and durable plan artifacts.
---

# Hermes-managed QRSPI

Hermes owns worker lifecycle and decides what happens next. Pi performs one bounded stage in an isolated context. The filesystem under `thoughts/...` is durable truth; Hermes operational state, process handles, gateway settings, and credentials are not artifacts.

## Bootstrap

The first child of any stage owns plan bootstrap. It creates the plan directory under `thoughts/.../plans/`, copies the manager-provided `AGENTS.md` template into that directory, and then writes the first durable artifact for its active stage. Hermes may supply a not-yet-existing absolute plan path to `vamos hermes pi start`; the launcher must not require `AGENTS.md`, `design.md`, `outline.md`, or `plan.md` before this bootstrap child runs.

## Worker contract

Every worker starts by reading this skill, its active stage skill, the plan `AGENTS.md`, and only the stage artifacts named there. It creates or updates the durable stage artifact, then ends with a normal final response naming the artifact and the smallest decision for Hermes or the lead. In a managed opaque-settlement run, do not invoke `vamos hermes pi done`, select a successor, or hand-write a machine-routing document.

When stopping to ask the manager a question or request a decision, include one fenced `yaml` or `yml` block in that final response. This is human-readable opaque communication, not a machine-routing protocol: Hermes preserves it exactly and does not parse it, infer a lifecycle outcome, or launch a successor from it.

```yaml
manager_message:
  kind: question
  questions:
    - State the smallest decision needed from the manager.
```

`PI_SESSION_ID` is Pi's injected session identity and `VAMOS_PLAN_DIR` is transient plan context. At every actual `agent_settled` boundary, the extension captures the final assistant response and any qualifying YAML/YML fence as immutable opaque evidence. A stop with no YAML is still delivered to the manager as the raw response; missing YAML must not become a fabricated outcome. Hermes alone owns lifecycle, process steering, and any successor decision. `pi done` remains an explicit human/operator recovery path for manual sessions, not a normal managed-worker action.

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

Use concise prose that names the durable artifact and the smallest manager decision. For an intentional manager question, add the opaque YAML block above. Do not emit `outcome`, `next`, lifecycle aliases, or machine-readable successor instructions.

## Artifact rules

- Plan artifacts use `thoughts/...` relative identities.
- Plan-owned Pi and Hermes sessions live in `.vamos/sessions/{pi,hermes}`.
- The database is a disposable projection; disk remains authoritative.
- Implementation uses the workspace recorded by workspace preparation; do not create another copy.
- Implementation-review follow-up work stays in the reviewed implementation workspace and stacks on its reviewed head.
