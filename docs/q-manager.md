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

q-manager child sessions load a local Pi extension plus CLI validation loop. A normal parent wake is a validated manager-needed event, not raw `agent_end`. The extension may still write child `status.json` and touch `done` as diagnostics, but those files are not authoritative manager triggers.

Wake YAML includes validation state and resolved child result context:

```yaml
q_manager_child_wake:
  validated: true
  manager_needed: true
  retry_exhausted: false
  stage: "<node>"
  status: "<status>"
  outcome: "<outcome>"
  artifact: "thoughts/..."
  state_file: "<state-file>"
  reason: "validated graph result"
  next:
    steps:
      - action: "run_command"
        param: "vamos qrspi continue --state-file <state-file>"
```

Intermediate invalid/missing `qrspi_result` turns, parser retries, and Codex/SSE header noise are suppressed from manager chat while deterministic repair remains possible. Retry exhaustion emits one manager-needed wake with `validated=false`, `retry_exhausted=true`, attempt/limit context, child refs, and deterministic-recovery-first guidance.

The manager normally runs `continue`, which validates the active child session JSONL, reprompts the same child when retry remains, persists the canonical graph decision for valid results, starts the graph-selected next child when safe, and cleans the old pane only after the next child exists. `start-next` / `continue` may accept explicit manager usage flags (`--manager-usage-percent` or `--manager-usage-tokens` + `--manager-usage-window`); when usage is above 80%, q-manager writes an operational handoff, marks delivery `compacting`, and queues child wakes until `manager-ready` flushes them.

Default `continue` output is concise text for manager chat, not the raw validate/decide NDJSON dump:

```text
validated: review-implementation complete
outcome: ready-for-human-review
artifact: thoughts/.../review.md
next: verify
started child: verify (%144)
```

Blocked/error/human stops stay short, include exact child/artifact refs, and preserve the child pane for inspection:

```text
validated: verify blocked
artifact: thoughts/.../verify.md
stop: result blocked
next: diagnose artifact/session; steer or continue if deterministic before asking human
```

Human gates and repairable failures are surfaced as structured manager action cards. Cards include `kind`, evidence, recommended action, safe command, optional continue command, and for human gates a concise review summary to present to the human. Human gates should be summarized to the human, then sent back to the same child with `vamos qrspi steer-child --state-file <state> --feedback-file <answer.md>`. Blocked/error states should be diagnosed first; ask the human only when intent, product/safety judgment, workspace replacement, merge policy, or external authority is truly required.

Self-heal commands are deterministic control-plane repairs, not durable artifact truth:

```bash
vamos qrspi repair-state --state-file <state> --align-active-child
vamos qrspi mark-child-active --state-file <state> --child-id <id> --reason manual-reprompt
```

Use `repair-state` when active child/session/artifact evidence proves the workflow cursor is stale. Use `mark-child-active` after manual child steering/reprompting so queued wakes from an older child generation are superseded and `manager-ready` waits for the newer completion.

## Session metadata boundary

Do not require Pi session metadata schema/API changes. q-manager assigns exact child `--session-id` values inside manager-owned `--session-dir` directories and treats the resulting Pi session JSONL as the authoritative child result source. tmux/stdout transcripts and plaintext result files are diagnostics only; `--result-file` is a deprecated debug fallback, not the manager default.

## Deterministic reload sources

Reload from this manifest, `.pi/skills/q-manager/SKILL.md`, `.pi/skills/qrspi-planning/SKILL.md`, plan `AGENTS.md`, latest stage artifact/result, and manager state file. Manager state `ActiveChild` refs are the recovery anchor for pane, transcript, session JSONL, done marker, and status marker.

## Recovery and cleanup policy

- Invalid result: reprompt the same child pane/session while retry budget remains; do not create a replacement child and do not wake the manager.
- Retry exhaustion: wake once with `validated=false`, `manager_needed=true`, `retry_exhausted=true`, failure reason, attempts, child refs, and deterministic-recovery-first guidance.
- Human gate, blocked, error, or retry exhaustion: keep the child pane and session refs for inspection and recovery.
- Valid transition with `startNext=true`: mark the old child pending cleanup, launch the next graph-selected child, save the new active child, then kill the old pane.
- Recoverable stale manager state/result mismatch: emit a structured action card, normalize state with `repair-state` when evidence is deterministic, append a local validation-recovery log, and continue instead of blocking the manager.
- Next launch failure: preserve the old pane/session and pending cleanup refs.
- Cleanup failure: keep the new active child and retain pending cleanup state for later recovery.

## Manual tmux smoke path

1. Start the manager Pi session inside tmux.
1. Start/resume q-manager with the exact parent pane:
   ```bash
   vamos qrspi start-next --plan-dir <plan> --project-root "$PWD" --manager-pane "$TMUX_PANE"
   ```
   Optional: pass explicit parent usage (`--manager-usage-percent 82` or token/window flags). Missing usage skips compaction; q-manager does not guess.
1. Confirm the child pane is visible and launch refs include `--session-id`, `--session-dir`, and `--extension`.
1. Confirm no parent wake appears for invalid/missing result turns while retry remains, including header-like SSE noise.
1. When the child reaches a valid graph result or retry exhaustion, confirm the parent pane receives one buffered wake prompt with validation fields and `param: "vamos qrspi continue --state-file <state>"`. If the manager was compacting, confirm no immediate paste occurs until `vamos qrspi manager-ready --state-file <state> --manager-pane "$TMUX_PANE"` flushes the queued wake.
1. Run the exact `continue --state-file <state>` command from the wake.
1. Confirm concise output and next child start or stop reason.
1. If a human gate appears, confirm `action: human_gate` includes artifact/question summary, write the answer to a file, and run `vamos qrspi steer-child --state-file <state> --feedback-file <answer.md>`.
1. If a repairable failure appears, confirm action cards include evidence and a safe command such as `repair-state --align-active-child && continue`, without launching duplicate children.
1. If the graph starts a next child, confirm the old pane is killed only after the new pane exists.

## Verification and merge habits

Use `go test` for CLI/runtime helpers, fake tmux in unit tests, and manual tmux smoke only after unit coverage. Finish Vamos runtime work through normal QRSPI review/verify and `/vamos-merge`.
