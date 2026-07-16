# q-manager Manifest

## Manager mission

q-manager supervises QRSPI stage sessions from a main Pi session while keeping child stage contexts focused, visible, and graph-authoritative.

## Authority boundaries

Use the canonical QRSPI graph and `qrspi_result.policy` to decide advancement. q-manager may start graph-safe non-human next stages in guided/autopilot modes. q-manager must stop for human gates, blocked/error results, invalid-result retry exhaustion, lock conflicts, or judgment that the project manifest marks human-owned.

## QRSPI policy and graph authority

`pkg/agents/workflows/qrspi.Definition`, QRSPI parser/converter, artifact validation, and `runtime.DecideTransition` are authoritative. q-manager must not hand-roll transitions from YAML text or duplicate policy rules.

## Human escalation preferences

Escalate irreversible workflow changes, project philosophy changes, unsafe workspace replacement, hidden child execution, ambiguous merge policy, or any request to edit Pi metadata/session schema.

## Workspace/copy boundary

Before `/q-workspace`, child stages run in the planning/source checkout. After `/q-workspace`, implementation/review/verify child stages run in `workspace_metadata.implementation_workspace`. q-manager control state lives outside copied repos under user state dir and is disposable.

## Visible child-session rule

Child QRSPI work runs in a visible tmux pane, usually a right split. Humans must be able to watch, interrupt, and steer. Recovery refs must identify the pane/transcript plus `sessionId`, `sessionDir`, `sessionPath` when resolved, `donePath`, and `statusPath`.

## Child wake contract

q-manager child sessions load a local Pi extension plus CLI validation loop. A normal parent wake is a validated manager-needed event, not raw `agent_end`. The extension invokes Go `qrspi child-complete`; Go reads the child session JSONL, validates/normalizes the result, generates `validation-status.json`, and delivers/queues the wake. The extension may still write child `status.json` and touch `done` as diagnostics, but those files are not authoritative manager triggers.

Wake YAML includes validation state and resolved child result context. Normal results include a continue action. A successful handoff continuation is informational instead:

```yaml
q_manager_child_wake:
  validated: true
  manager_needed: false
  continuation_started: true
  retry_exhausted: false
  stage: "research"
  status: "handoff"
  artifact: "thoughts/.../handoffs/research-handoff.md"
  state_file: "<state-file>"
  reason: "handoff_auto_resumed"
  next_child:
    stage: "research"
    skill: ".pi/skills/q-resume/SKILL.md"
    cwd: "<source-or-implementation-cwd>"
```

There is no continue action when `continuation_started=true`; the replacement child is already durable. Graph decision and manager policy authorize this path, never child `next.steps`.

Intermediate invalid/missing `qrspi_result` turns, parser retries, and Codex/SSE header noise are suppressed from manager chat while deterministic repair remains possible. Retry exhaustion emits one manager-needed wake with `validated=false`, `retry_exhausted=true`, attempt/limit context, child refs, and deterministic-recovery-first guidance. Do not hand-author `validation-status.json`; it is generated runtime state for child-side logging and manager wake gating.

For non-auto-resumed wakes, the manager normally runs `/q-manager continue`, which samples live parent Pi context usage, then delegates to CLI `continue`. For a graph-authorized handoff in guided/autopilot, `child-complete` instead persists the same-node decision, validates the exact in-plan handoff, starts a fresh q-resume child, saves replacement lineage, writes source validation status, delivers or queues the informational wake, and only then cleans the old pane. Discuss mode validates but does not launch. The CLI otherwise validates the active child session JSONL, reprompts the same child when retry remains, persists canonical graph decisions, starts graph-selected children when safe, and cleans old panes only after replacement durability. Slight positive wording mistakes are normalized only when deterministic from node/status/workspace context, such as `review-outline` + `status: complete` + `outcome: complete` becoming `ready-for-plan`; ambiguous, negative, human, blocked, or follow-up outcomes still reprompt or stop. The parent Pi `/q-manager start-next|continue` wrapper samples `ctx.getContextUsage()` and passes explicit `--manager-usage-*` flags to the Go CLI; the CLI does not scan parent Pi JSONL for usage. Raw CLI usage flags remain a debug/manual seam. When fresh parent usage is `>=90%`, q-manager writes an operational handoff, saves `Delivery.Status=compacting`, emits `q-manager-parent-compact: started`, and only then the parent wrapper calls native `ctx.compact()`. Child wakes during parent compaction queue until the fresh manager runs `manager-ready` once.

Default `continue` output is concise text for manager chat, not the raw validate/decide NDJSON dump:

```text
validated: review-implementation complete
outcome: ready-for-human-review
artifact: thoughts/.../review.md
policy: autopilot, plan reviews on, retries 1
next: verify
next child: verify
working on: Run verification, inspect artifacts, and produce verify.md.
started child: verify (%144)
```

Blocked/error/human stops stay short, include exact child/artifact refs, and preserve the child pane for inspection:

```text
validated: verify blocked
artifact: thoughts/.../verify.md
stop: result blocked
next: diagnose artifact/session; steer or continue if deterministic before asking human
```

Human gates and repairable failures are surfaced as structured manager action cards. Handoff-specific kinds include `invalid_handoff_artifact`, `handoff_continuation_failed`, `manager_delivery_failed`, and `pending_child_cleanup_failed`. Cards include `kind`, evidence, recommended action, safe command, optional continue command, and for human gates a concise review summary to present to the human. `pi_compatibility_failed` means Pi/tmux/state preflight failed before launch state should be trusted; run `vamos qrspi doctor --state-file <state>` or the card's safe command. `child_launch_failed` means active-child diagnostics prove a terminal child process failure before a durable `qrspi_result`; run the card's `repair-state --clear-failed-child --relaunch` safe command only when evidence is deterministic. `child_context_exhausted` means the child ended with context-limit/no-result evidence; preserve refs, inspect latest session, compact/resume the same child only when the evidence is real, or relaunch the same graph node after salvage is impossible. `provider_context_error` is the deterministic child-context variant where the latest Pi JSONL terminal assistant message has `stopReason: "error"` plus a provider context-window `errorMessage`; latest terminal session evidence outranks older `validation-status.json` and older valid `qrspi_result` text in the same session. The action card and wake include session path/id, line/timestamp, evidence ID, provider error, inspect command, latest-session continue command, and optional `recover-summary` command; these are recovery refs only and never a fabricated `qrspi_result`. Human gates should be summarized to the human, then sent back to the same child with `vamos qrspi steer-child --state-file <state> --feedback-file <answer.md>`. Blocked/error states should be diagnosed first; ask the human only when intent, product/safety judgment, workspace replacement, merge policy, or external authority is truly required.

Self-heal commands are deterministic control-plane repairs, not durable artifact truth:

```bash
vamos qrspi doctor --state-file <state> --output text
vamos qrspi repair-state --state-file <state> --align-active-child
vamos qrspi repair-state --state-file <state> --clear-failed-child --relaunch
vamos qrspi mark-child-active --state-file <state> --child-id <id> --reason manual-reprompt
vamos qrspi set-policy --state-file <state> --preset guided
vamos qrspi set-policy --state-file <state> --preset autopilot
vamos qrspi set-policy --state-file <state> --preset autopilot-no-plan-reviews
vamos qrspi set-policy --state-file <state> --preset fast
vamos qrspi set-policy --state-file <state> --advance-mode autopilot --enable-plan-reviews=true
vamos qrspi inspect --state-file <state> --sessions --latest
vamos qrspi find-latest-child --state-file <state> --stage <node>
vamos qrspi validate-latest --state-file <state> --stage <node> --apply-rebind
vamos qrspi recover-manual --state-file <state> --mode latest-session --continue
vamos qrspi recover-summary --state-file <state> --session-file <child.jsonl>
```

Use `doctor` when launch compatibility, state-root writability, tmux health, latest status, or active-child health is unclear. Use `repair-state --align-active-child` when active child/session/artifact evidence proves the workflow cursor is stale. Use `repair-state --clear-failed-child --relaunch` only for terminal failed active children proven by status/done/output/session evidence; it clears local active-child state and relaunches the same graph node, not a new graph transition. Use `mark-child-active` after manual child steering/reprompting so queued wakes from an older child generation are superseded and `manager-ready` waits for the newer completion. Use latest-session recovery for same-child chat, child `/new`, manual completion, retry exhaustion inspection, no-wake recovery, and stale wake supersession before editing manager JSON. `validate-latest --apply-rebind` with or without `--continue` must surface a latest `provider_context_error` instead of accepting stale older YAML. `recover-summary` is an optional helper for context-window failures: it writes a prompt under the local q-manager state `prompts/` directory and a same-stage recovery note target under `context/recovery/` from failed session evidence. Use `--dry-run` to write a deterministic placeholder note without launching Pi. The helper must not emit `qrspi_result`, advance the graph, or edit code. For child context exhaustion, do not invent YAML or advance from artifacts alone; recover a valid child result or relaunch the same node.

## Session metadata boundary

Do not require Pi session metadata schema/API changes. q-manager assigns exact child `--session-id` values and stores child Pi JSONL under the plan workspace `.sessions/pi/` directory, not under the local manager state directory, so humans can discover and `pi --resume` stage sessions from the workspace. q-manager treats the resulting Pi session JSONL as the authoritative child result source. tmux/stdout transcripts and plaintext result files are diagnostics only; `--result-file` is a deprecated debug fallback, not the manager default.

## Deterministic reload sources

Reload from this manifest, `.pi/skills/q-manager/SKILL.md`, `.pi/skills/qrspi-planning/SKILL.md`, plan `AGENTS.md`, latest stage artifact/result, and manager state file. Manager state `ActiveChild` refs are the recovery anchor for pane, transcript, session JSONL, done marker, and status marker.

## Recovery and cleanup policy

- Invalid result: reprompt the same child pane/session while retry budget remains; do not create a replacement child and do not wake the manager.
- Retry exhaustion: wake once with `validated=false`, `manager_needed=true`, `retry_exhausted=true`, failure reason, attempts, child refs, and deterministic-recovery-first guidance.
- Human gate, blocked, error, or retry exhaustion: keep the child pane and session refs for inspection and recovery.
- Normal valid transition with `startNext=true`: mark the old child pending cleanup, launch the next graph-selected child, save the new active child, then kill the old pane.
- Valid agent-node handoff in guided/autopilot: require exact-node `status: in_progress` frontmatter under the mapped plan `handoffs/`; reject file or directory symlink escapes; start a fresh same-node q-resume child before informational wake and old-pane cleanup. Discuss waits.
- Duplicate source callbacks reuse replacement lineage. Launch failure retains source refs and emits `handoff_continuation_failed` rather than false success.
- Direct wake delivery is two-phase: paste failure queues paste+submit; Enter failure queues submit-only for that pane. Pane adoption re-pastes once. A matching running replacement does not stale its queued informational wake.
- Old-pane cleanup is idempotent. Missing pane succeeds; kill/layout partial failure retains pending cleanup so a later manager operation converges without harming the replacement.
- Recoverable stale manager state/result mismatch: emit a structured action card, normalize state with `repair-state --align-active-child` when evidence is deterministic, append a local validation-recovery log, and continue instead of blocking the manager.
- Pi compatibility/preflight failure: stop before creating/trusting active-child state, emit `pi_compatibility_failed`, and use `doctor` evidence/safe command before retrying launch.
- Terminal child launch failure: emit `child_launch_failed` with pane/status/exit/output-tail/full-output evidence; use `repair-state --clear-failed-child --relaunch` or `start-next --force` only when health classification proves terminal failure and no durable `qrspi_result` exists.
- Next launch failure: preserve the old pane/session and pending cleanup refs.
- Cleanup failure: keep the new active child and retain pending cleanup state for later recovery.

## Manual tmux smoke path

1. Start the manager Pi session inside tmux.
1. Start/resume q-manager from the parent Pi session with the exact parent pane:
   ```text
   /q-manager start-next --plan-dir <plan> --project-root "$PWD" --manager-pane "$TMUX_PANE"
   ```
   Normal parent Pi path samples live usage with `ctx.getContextUsage()` and triggers native parent compaction only after the CLI saves queue-safe `compacting` state. Debug/manual fallback is the raw CLI with explicit usage flags, for example `vamos qrspi start-next --plan-dir <plan> --project-root "$PWD" --manager-pane "$TMUX_PANE" --manager-usage-percent 90`; missing usage skips compaction and q-manager does not guess.
1. Confirm the child pane is visible and launch refs include `--session-id`, `--session-dir`, and `--extension`; `--session-dir` should point at the plan workspace `.sessions/pi/` directory.
1. Confirm no parent wake appears for invalid/missing result turns while retry remains, including header-like SSE noise.
1. When the child reaches a normal valid graph result or retry exhaustion, confirm the parent pane receives one buffered wake with the continue command. For a valid guided handoff, confirm a fresh same-node q-resume child appears first and the informational wake says `continuation_started: true` with no continue action. If compacting, `manager-ready` must flush the matching replacement wake even while that child is running.
1. If the original parent pane was replaced or is unavailable, run the raw CLI recovery command from the intended new parent tmux pane: `vamos qrspi continue --state-file <state> --manager-pane "$TMUX_PANE"`. If `continue` or `start-next --state-file` reports `manager_pane_adoption_required`, the stored parent pane is still live and differs from current `$TMUX_PANE`; rerun the safe command printed in the action card only from the intended parent pane.
1. If child completion queued a wake because the selected manager pane was unavailable, run `vamos qrspi manager-ready --state-file <state> --manager-pane "$TMUX_PANE"`, then follow the flushed wake or run `vamos qrspi continue --state-file <state>` from that parent pane.
1. Parent Pi `/q-manager` wrapper remains the preferred live path because it samples `ctx.getContextUsage()` for native compaction. Plain `vamos qrspi continue/start-next --manager-pane "$TMUX_PANE"` is the recovery/debug path and safely adopts parent pane ownership when the stored pane is stale or explicit operator intent is supplied.
1. Run the wrapper continuation (`/q-manager continue --state-file <state>`) or the exact raw CLI `continue --state-file <state>` command from the wake when debugging.
1. Confirm concise output and next child start or stop reason.
1. If a human gate appears, confirm `action: human_gate` includes artifact/question summary, write the answer to a file, and run `vamos qrspi steer-child --state-file <state> --feedback-file <answer.md>`.
1. If launch compatibility is suspect, run `vamos qrspi doctor --state-file <state>` and confirm Pi compatibility, state-root, tmux, active-child health, latest status, and safe command are concise.
1. If a repairable failure appears, confirm action cards include evidence and a safe command such as `repair-state --align-active-child && continue`, `repair-state --clear-failed-child --relaunch`, or child context-exhaustion recovery commands, without launching duplicate children or advancing without valid YAML.
1. If a terminal failed child is present, confirm `start-next --force` replaces it but still protects running/unknown children.
1. If the graph starts a next child, confirm the old pane is killed only after the new pane exists.

### Provider context-window smoke

Given a q-manager state with active child/session refs whose latest JSONL ends in provider context-window evidence after older YAML:

```bash
vamos qrspi inspect --state-file <state.json> --sessions --latest
vamos qrspi validate-latest --state-file <state.json> --apply-rebind
vamos qrspi child-complete --state-file <state.json> --child-id <child-id> --output json
vamos qrspi continue --state-file <state.json>
```

Expected: all paths surface `provider_context_error` / `child_context_exhausted`, include session/evidence fields and safe recovery commands, write fresh terminal validation status, bypass any older blocked-result delivery ID once, and do not advance graph from stale older `qrspi_result`.

## Verification and merge habits

Use `go test` for CLI/runtime helpers, fake tmux in unit tests, and manual tmux smoke only after unit coverage. Finish Vamos runtime work through normal QRSPI review/verify and `/vamos-merge`.
