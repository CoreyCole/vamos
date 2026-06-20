---
name: q-manager
description: Manage QRSPI stage sessions from a main Pi/tmux manager session. Use when asked to run q-manager, supervise a QRSPI plan, auto-advance QRSPI stages in tmux, or continue a QRSPI workflow from a manager session.
---

# q-manager

## Purpose

Supervise QRSPI from a main Pi manager session. Launch focused child Pi sessions in visible tmux panes, capture the child result, validate through canonical Vamos QRSPI graph helpers, then advance or stop according to graph decision and QRSPI policy.

## Required context load

1. Read `.pi/skills/qrspi-planning/SKILL.md`.
1. Read `docs/q-manager.md` when present.
1. Read the target plan `AGENTS.md`.
1. Read the latest QRSPI result artifact or user-provided result YAML.
1. Use `vamos qrspi render-prompt` to render the next child-stage prompt when available.

## Rules

- Existing QRSPI graph is canonical. Do not infer transitions from YAML text alone.
- Use `cmd/vamos-runtime` helpers, not a new binary.
- Child work must be visible and interruptible in tmux. No hidden background child runner as primary UX.
- Manager state is disposable control state under user state dir, keyed by canonical `plan_dir`; never use repo-local `.vamos/q-manager`.
- QRSPI artifacts and fenced `qrspi_result` YAML remain durable truth.
- Respect `advanceMode`: `discuss` stops after valid result; `guided` starts graph-safe non-human edges; `autopilot` can auto-approve only graph-marked safe gates.
- Stop on human gates, blocked/error results, invalid result retry exhaustion, lock conflict, or ambiguous project judgment named by `docs/q-manager.md`.
- After `/q-workspace`, run implementation/review/verify child stages in `workspace_metadata.implementation_workspace` when graph semantics require implementation cwd.
- Pi session metadata redesign is out of scope; rely on explicit child output/result capture paths or stable current session output APIs only.

## Start flow

1. Resolve plan dir and project root.
1. Run `vamos qrspi init --plan-dir <plan-dir> --project-root <repo-root>`.
1. Render prompt for current/next node: `vamos qrspi render-prompt --state-file <state> --node <node> --plan-dir <plan-dir>`.
1. Start child and wait for its result file: `vamos qrspi run-child --state-file <state> --plan-dir <plan-dir> --stage <node> --cwd <cwd> --prompt-file <prompt> --split right --timeout 12h`.
1. Validate: `vamos qrspi validate-result --state-file <state> --stage <node> --result-file <result> --plan-dir <plan-dir>`.
1. Decide: `vamos qrspi decide-next --state-file <state> --result-file <result> --plan-dir <plan-dir>`.
1. If decision starts next, repeat. If decision stops, report concise reason and next human action.

## Result retry

If validation fails and policy retry budget remains, send the correction prompt from the QRSPI parser to the same child pane. If retry budget is exhausted, stop and ask the human.

## Human gates

Ask the human one direct question. Preserve graph decision, latest result, and any human answer in manager session context. Do not rewrite workflow state by hand.
