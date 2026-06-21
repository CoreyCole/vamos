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
- Pi session metadata redesign is out of scope; q-manager assigns exact child `--session-id` / `--session-dir` using current Pi flags.
- Child session JSONL is authoritative for result parsing. tmux transcript/output is diagnostic.

## Wake-driven manager loop

1. Resolve plan dir and project root.
1. Initialize or resume graph state:
   ```bash
   vamos qrspi init --plan-dir <plan-dir> --project-root <repo-root> --manager-pane "$TMUX_PANE"
   ```
   Add `--node <node>` / `--implementation-cwd <cwd>` only when deliberately resuming or testing a specific implementation, review, or verify stage.
1. Render prompt for the current graph node:
   ```bash
   vamos qrspi render-prompt --state-file <state> --node <node> --plan-dir <plan-dir>
   ```
1. Start the visible child and return immediately:
   ```bash
   vamos qrspi run-child --state-file <state> --plan-dir <plan-dir> --stage <node> --cwd <cwd> --prompt-file <prompt> --split right --timeout 0
   ```
1. Wait for the child extension to paste the parent wake:
   ```text
   q-manager child finished: <node>
   ```
1. Validate from the active child session JSONL:
   ```bash
   vamos qrspi validate-result --state-file <state> --stage <node> --plan-dir <plan-dir>
   ```
1. Invalid result with retry budget: save the validation error, then reprompt the same pane/session:
   ```bash
   vamos qrspi reprompt-child --state-file <state> --plan-dir <plan-dir> --stage <node> --attempt <n> --error-file <validation-error-file>
   ```
1. Valid result: decide through the canonical graph:
   ```bash
   vamos qrspi decide-next --state-file <state> --plan-dir <plan-dir>
   ```
1. If `startNext=true`, render and run the graph-selected next child. Old child pane cleanup happens only after the new child pane starts successfully. If the decision stops, report the concise stop reason and next human action.

Manual/debug overrides: `--session-file <jsonl>` validates a specific child session JSONL. `--result-file <path>` is deprecated fallback for plaintext result files only when no active child session refs are available.

## Child wake contract

q-manager loads a project-local child Pi extension only for q-manager child sessions. On `agent_end`, the extension writes `status.json`, touches `done`, and pastes the wake text to the captured parent tmux pane. The wake means “child turn ended,” not “graph result is valid.” The manager still runs `validate-result` and `decide-next` before any advancement.

## Result retry

If validation fails and policy retry budget remains, run `reprompt-child` with the validation error file. It pastes the canonical QRSPI parser correction prompt into the same active child pane/session; do not create a new child ID/session. If retry budget is exhausted, stop and ask the human.

## Cleanup and recovery

- Invalid result: keep active child pane/session and reprompt in place while retry remains.
- Human gate, blocked, error, or retry exhaustion: keep pane/session for inspection and human steering.
- Valid transition with `startNext=true`: mark old child pending cleanup; start next child; kill old pane only after the new active child is saved.
- Next-child launch failure or cleanup failure: preserve refs in manager state for recovery.

## Human gates

Ask the human one direct question. Preserve graph decision, latest result, and any human answer in manager session context. Do not rewrite workflow state by hand.
