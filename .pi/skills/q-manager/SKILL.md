---
name: q-manager
description: Manage QRSPI stage sessions from a main Pi/tmux manager session. Use when asked to run q-manager, supervise a QRSPI plan, auto-advance QRSPI stages in tmux, or continue a QRSPI workflow from a manager session.
---

# q-manager

## Purpose

Supervise QRSPI from a main Pi manager session. Launch focused child Pi sessions in visible tmux panes, capture the child result, validate through canonical Vamos QRSPI graph helpers, then advance or stop according to graph decision and QRSPI policy.

## Required context load

1. Read `.pi/skills/qrspi-planning/SKILL.md`.
1. Read `docs/q-manager.md` when present for manager behavior only; do not stuff manager instructions into child stage prompts.
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
- For implementation-review follow-up/review-dir plans that already have `workspace_metadata.implementation_workspace`, do not imply a fresh workspace/copy/reset. Prompts for `/q-plan` and planning review must state that implementation should stack in the existing implementation workspace on the reviewed head. If the current graph forces a `workspace` node anyway, that node must preserve/reaffirm the existing workspace and continue to implementation; do not create a new copy.
- Pi session metadata redesign is out of scope; q-manager assigns exact child `--session-id` / `--session-dir` using current Pi flags.
- Child session JSONL is authoritative for result parsing. tmux transcript/output is diagnostic.
- Never paste multiline manager prompts into an interactive child pane as raw tmux keystrokes. Newlines can submit as separate child prompts. For initial stage prompts, write a prompt file and launch via `run-child --prompt-file`. For follow-up steering, prefer a CLI helper that injects one bracketed/atomic prompt; if unavailable, paste a single-line instruction pointing at a file path the child should read.
- Do not poll or sleep on child `done` as the normal control loop. `done`/`status.json` are recovery diagnostics; the primary manager trigger is the child wake pasted into the parent pane.
- Do not put manager `stateFile`, run IDs, pane IDs, session dirs, or other disposable q-manager control refs in durable `qrspi_result` YAML. Report them in manager prose/diagnostics only. Durable YAML should keep plan/workspace/artifact identity, not machine-local manager state.
- When testing the runtime CLI from a Vamos checkout, use `go run ./cmd/vamos-runtime ...` in place of installed `vamos ...`.
- Child prompts should be stage-work prompts, not manager runbooks. The primary child context should be the previous stage's fenced `qrspi_result` YAML plus minimal routing metadata needed to read planning docs and start the selected stage. The CLI should pass that YAML directly to the child prompt from manager state/session JSONL; do not paste the YAML into the parent manager chat.
- Manager-specific instructions from `docs/q-manager.md` are for the parent manager/CLI. Do not embed the full manager manifest in every child prompt. If the CLI needs manifest-derived child context, render a small normalized child-safe summary, not raw docs.
- q-manager may accept extra operator context for a child, but that context should be explicit and additive. A valid previous `qrspi_result` should normally be sufficient for the child to read the plan docs and proceed.

## Wake-driven manager loop

Primary loop: launch child, then wait for pasted wake. Do **not** block this manager session in `sleep`/poll loops. The extension wake is the normal event; marker files are only fallback diagnostics.

1. Resolve plan dir and project root.
1. Initialize or resume graph state and capture `stateFile` from JSON:
   ```bash
   STATE=$(vamos qrspi init --plan-dir <plan-dir> --project-root <repo-root> --manager-pane "$TMUX_PANE" | jq -r '.ref.stateFile')
   ```
   Add `--node <node>` / `--implementation-cwd <cwd>` only when deliberately resuming or testing a specific implementation, review, or verify stage.
1. Render prompt for the current graph node to a prompt file:
   ```bash
   PROMPT="$(dirname "$STATE")/<node>-prompt.md"
   vamos qrspi render-prompt --state-file "$STATE" --node <node> --plan-dir <plan-dir> > "$PROMPT"
   ```
   The rendered prompt should include the previous stage's `qrspi_result` YAML as the canonical handoff context when available. If the user provided latest result YAML in chat, pass it as latest result context. Do not hand-infer graph transitions from it.
   For review-dir / implementation-review follow-up plans, same-workspace routing should come from the previous `qrspi_result.workspace_metadata` and plan docs. If the CLI detects and summarizes it, keep the summary child-safe and minimal: do not create a new implementation copy or reset to trunk; stack follow-up implementation on the existing implementation workspace/head.
1. Start the visible child and return immediately:
   ```bash
   vamos qrspi run-child --state-file "$STATE" --plan-dir <plan-dir> --stage <node> --cwd <cwd> --prompt-file "$PROMPT" --split right --timeout 0
   ```
1. Stop issuing commands and wait for the child extension to paste the parent wake:
   ```text
   q-manager child finished: <node>
   state_file: <state-file>
   ```
   The wake means “child turn ended,” not “valid result.”
1. Validate from the active child session JSONL:
   ```bash
   vamos qrspi validate-result --state-file "$STATE" --stage <node> --plan-dir <plan-dir>
   ```
1. Invalid result with retry budget: save the validation error, then reprompt the same pane/session:
   ```bash
   vamos qrspi reprompt-child --state-file "$STATE" --plan-dir <plan-dir> --stage <node> --attempt <n> --error-file <validation-error-file>
   ```
1. Valid result: decide through the canonical graph:
   ```bash
   vamos qrspi decide-next --state-file "$STATE" --plan-dir <plan-dir>
   ```
1. If `startNext=true`, render and run the graph-selected next child. Old child pane cleanup happens only after the new child pane starts successfully. If the decision stops, report the concise stop reason and next human action.

Manual/debug overrides: `--session-file <jsonl>` validates a specific child session JSONL. `--result-file <path>` is deprecated fallback for plaintext result files only when no active child session refs are available.

### Runtime CLI testing with `go run`

When the user asks to test the runtime CLI before the installed `vamos` binary includes a command, prefix commands with `go run ./cmd/vamos-runtime`. Keep the same wake-driven shape:

```bash
STATE=$(go run ./cmd/vamos-runtime qrspi init --plan-dir "$PLAN" --project-root "$PWD" --manager-pane "$TMUX_PANE" --node <node> --implementation-cwd "$PWD" --force | jq -r '.ref.stateFile')
PROMPT="$(dirname "$STATE")/<node>-prompt.md"
go run ./cmd/vamos-runtime qrspi render-prompt --state-file "$STATE" --node <node> --plan-dir "$PLAN" > "$PROMPT"
go run ./cmd/vamos-runtime qrspi run-child --state-file "$STATE" --plan-dir "$PLAN" --stage <node> --cwd "$PWD" --prompt-file "$PROMPT" --split right --timeout 0
```

After `run-child`, do not poll. Wait for the pasted wake, then validate/decide with the same `go run ./cmd/vamos-runtime ...` prefix.

## Child wake contract

q-manager loads a project-local child Pi extension only for q-manager child sessions. On `agent_end`, the extension writes `status.json`, touches `done`, and pastes the wake text to the captured parent tmux pane. The wake means “child turn ended,” not “graph result is valid.” The manager still runs `validate-result` and `decide-next` before any advancement.

The manager CLI/extension owns the exact wake text so it stays deterministic, testable, and versioned with runtime behavior. The skill should only define the semantic contract: wake is one parent prompt/one line, includes the finished node, includes enough local recovery context to find the manager state (for example `state_file`), and points to the single continue command. Do not let the skill become the source of truth for copy/paste wake templates.

The wake may include `state_file` because that is ephemeral manager control context needed to continue the local run. This value belongs in the wake/manager transcript, not in durable QRSPI artifacts or `qrspi_result` YAML. The wake must not paste multiline blocks into the parent manager session; multiline wake text can split into multiple manager prompts and pollute context.

## Result retry

If validation fails and policy retry budget remains, run `reprompt-child` with the validation error file. It pastes/injects the canonical QRSPI parser correction prompt into the same active child pane/session as one atomic prompt; do not create a new child ID/session and do not manually paste extra multiline correction prose. If retry budget is exhausted, stop and ask the human.

## Cleanup and recovery

- Invalid result: keep active child pane/session and reprompt in place while retry remains.
- Human gate, blocked, error, or retry exhaustion: keep pane/session for inspection and human steering.
- Valid transition with `startNext=true`: mark old child pending cleanup; start next child; kill old pane only after the new active child is saved.
- Next-child launch failure or cleanup failure: preserve refs in manager state for recovery.

## Manager session handoff

Use this when the parent manager Pi context is getting full, before auto-compaction or session loss. This is separate from QRSPI stage handoff: it transfers manager control context, not implementation work.

Do not rely on chat history. Write a short manager handoff doc or pasteable handoff block that contains enough local recovery refs for a fresh manager session to resume from deterministic sources.

Include:

- Plan dir absolute path and `thoughts/...` relative path.
- Project root / source checkout cwd.
- Implementation cwd when known.
- Current graph node and last completed stage.
- Latest durable `qrspi_result` YAML or path to the artifact containing it.
- Manager `stateFile` absolute path.
- Active child refs from state when a child is running: stage, pane ID, session ID, session dir/path, status path, done path, output/transcript path.
- Whether the manager is waiting for child wake, needs `validate-result`, needs `decide-next`, or is stopped at a human gate.
- Exact next command, using `go run ./cmd/vamos-runtime ...` when testing from checkout.

Manager handoff may include `stateFile` because it is an operational recovery note for the same local machine. Do not put `stateFile` in durable `qrspi_result` YAML. If writing the handoff under `thoughts/`, label these fields as local/ephemeral and keep durable plan identity (`thoughts/...` paths, artifact paths, latest result) separate from local recovery refs.

Prefer a dedicated `q-manager-handoff` skill/helper over overloading `/q-handoff`. `/q-handoff` is a QRSPI stage artifact for work continuity and should stay portable. A manager handoff is control-plane recovery and intentionally includes machine-local refs. If no dedicated helper exists, create a concise markdown note under the plan's `handoffs/` or paste it into the next manager session with a clear title: `q-manager operational handoff`.

Fresh manager resume shape:

```bash
# read q-manager skill, docs/q-manager.md, plan AGENTS.md, latest result/handoff first
STATE=<stateFile-from-manager-handoff>
go run ./cmd/vamos-runtime qrspi validate-result --state-file "$STATE" --stage <node> --plan-dir <plan-dir>
go run ./cmd/vamos-runtime qrspi decide-next --state-file "$STATE" --plan-dir <plan-dir>
```

If the handoff says “waiting for child wake,” do not validate until wake arrives unless manually inspecting recovery state. If no active child exists, resume by rendering and running the graph-selected/current node from the saved state.

## Human gates

Ask the human one direct question. Preserve graph decision, latest result, and any human answer in manager session context. Do not rewrite workflow state by hand.
