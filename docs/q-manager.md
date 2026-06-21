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

q-manager child sessions load a local Pi extension that observes `agent_end`. On each completed child agent turn, the extension writes child `status.json`, touches `done`, and pastes a wake message into the captured parent manager pane:

```text
q-manager child finished: <node> | state_file=<state-file> | next=vamos qrspi continue --state-file <state-file>
```

The wake is only a turn-complete signal. It does not validate YAML, decide transitions, mutate graph state, or imply workflow success. The manager normally runs `continue`, which validates the active child session JSONL, reprompts the same child when retry remains, persists the canonical graph decision for valid results, starts the graph-selected next child when safe, and cleans the old pane only after the next child exists.

## Session metadata boundary

Do not require Pi session metadata schema/API changes. q-manager assigns exact child `--session-id` values inside manager-owned `--session-dir` directories and treats the resulting Pi session JSONL as the authoritative child result source. tmux/stdout transcripts and plaintext result files are diagnostics only; `--result-file` is a deprecated debug fallback, not the manager default.

## Deterministic reload sources

Reload from this manifest, `.pi/skills/q-manager/SKILL.md`, `.pi/skills/qrspi-planning/SKILL.md`, plan `AGENTS.md`, latest stage artifact/result, and manager state file. Manager state `ActiveChild` refs are the recovery anchor for pane, transcript, session JSONL, done marker, and status marker.

## Recovery and cleanup policy

- Invalid result: reprompt the same child pane/session while retry budget remains; do not create a replacement child.
- Human gate, blocked, error, or retry exhaustion: keep the child pane and session refs for inspection and recovery.
- Valid transition with `startNext=true`: mark the old child pending cleanup, launch the next graph-selected child, save the new active child, then kill the old pane.
- Next launch failure: preserve the old pane/session and pending cleanup refs.
- Cleanup failure: keep the new active child and retain pending cleanup state for later recovery.

## Manual tmux smoke path

1. Start the manager Pi session inside tmux.
1. Initialize q-manager with the exact parent pane:
   ```bash
   vamos qrspi init --plan-dir <plan> --project-root "$PWD" --manager-pane "$TMUX_PANE"
   ```
1. Render a small child prompt that produces a valid `qrspi_result`.
1. Launch the visible child:
   ```bash
   vamos qrspi run-child --state-file <state> --plan-dir <plan> --stage <node> --cwd "$PWD" --prompt-file <prompt> --split right --timeout 0
   ```
1. Confirm the child pane is visible and launch refs include `--session-id`, `--session-dir`, and `--extension`.
1. When the child finishes, confirm the parent pane receives `q-manager child finished: <node>` with `next=vamos qrspi continue --state-file <state>`.
1. Confirm child `status.json` has `event=agent_end` and the `done` marker exists under the child run directory.
1. Run the exact `continue --state-file <state>` command from the wake.
1. Confirm concise output and next child start or stop reason.
1. If the graph starts a next child, confirm the old pane is killed only after the new pane exists.

## Verification and merge habits

Use `go test` for CLI/runtime helpers, fake tmux in unit tests, and manual tmux smoke only after unit coverage. Finish Vamos runtime work through normal QRSPI review/verify and `/vamos-merge`.
