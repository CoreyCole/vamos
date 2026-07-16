---
name: q-manager
description: Manage QRSPI stage sessions from a main Pi/tmux manager session. Use when asked to run q-manager, supervise a QRSPI plan, auto-advance QRSPI stages in tmux, or continue a QRSPI workflow from a manager session.
---

# q-manager

## Purpose

Supervise QRSPI from a main Pi manager session. Launch focused child Pi sessions in visible tmux panes, capture the child result, validate through canonical Vamos QRSPI graph helpers, then advance or stop according to graph decision and QRSPI policy.

## Manager startup input

Treat `/q-manager` as a conversational manager startup entrypoint. `start-next` and `continue` are optional direct-operation forms, not required startup syntax.

- `/q-manager` with no arguments starts or resumes management from the current conversation, latest durable QRSPI result, manager wake, or operational handoff.
- `/q-manager <fenced qrspi_result YAML>` accepts the complete pasted child result as startup input. Natural-language context may accompany it.
- Never return a usage error merely because the first argument is not `start-next` or `continue`. Do not make the human translate a child result into q-manager flags.
- Infer the plan directory, project root, implementation cwd, current graph node, and intended transition from the pasted/latest result plus durable artifacts. Validate that inference through the canonical graph and manager state; do not infer the transition from `next.steps` alone.
- If the input contains a `q_manager_child_wake` or local `state_file`, inspect that state and choose `continue` or the graph-safe recovery action. If no manager state exists, initialize/start the manager at the canonical next node and seed it with the complete latest result through the CLI result-input seam.
- Ask the human only when plan identity, intent, safety, or another human-owned judgment remains ambiguous after inspecting available context.
- Once startup has resolved the concrete operation, use the direct `/q-manager start-next ...` or `/q-manager continue ...` parent wrapper when operator invocation is available. Raw `vamos qrspi ...` remains the tool-driven debug/recovery seam.

## `/q-hermes-manager` / Hermes-managed orchestration

When the user invokes `/q-hermes-manager` or asks Hermes to manage a QRSPI flow, treat that as Hermes-managed background orchestration, not true Pi q-manager/tmux mode. Hermes should run tracked background Pi processes, parse full process logs for fenced `qrspi_result` YAML, and pass the complete previous YAML verbatim into the next graph-safe stage.

- If the user explicitly names an initial stage such as “delegate `/q-question` first”, launch that stage first before starting research/design/implementation.
- For research-only or document-only requests, stop after the requested artifact exists and provide a thoughts-viewer Markdown link; do not continue into design/outline/implementation unless the user requested a full QRSPI pipeline.
- Use the plan result’s `next.steps` to choose the next stage, but respect explicit user scope such as “create a research document” as the terminal human-review point.

## Required context load

1. Read `.pi/skills/qrspi-planning/SKILL.md`.
1. Read `docs/q-manager.md` when present for manager behavior only; do not stuff manager instructions into child stage prompts.
1. Read the target plan `AGENTS.md`.
1. Read the latest QRSPI result artifact or user-provided result YAML. A pasted result is sufficient startup input; do not require an action verb.
1. After resolving startup intent, use parent Pi `/q-manager start-next|continue` for normal operator-driven launch/resume so live usage is sampled and native parent compaction can run safely. Use raw `vamos qrspi start-next|continue` only for tool-driven debug/manual fallback. Use default concise text output for normal manager commands; reserve `--output ndjson` for debug/recovery when structured output is specifically needed. Use `init`, `render-prompt`, `run-child`, `validate-result`, `decide-next`, and `reprompt-child` only for debug/recovery.

## General guiding principles

- Capture generalized manager learnings in this q-manager skill. Capture project-specific manager policy, escalation preferences, or domain workflow rules in the target project’s `docs/q-manager.md` instead.
- Prefer deterministic self-recovery over human escalation when the failure is mechanical and evidence is sufficient. If a child did the right stage work but emitted invalid QRSPI YAML (for example wrong outcome label, invalid `next.steps` action, missing required field), inspect the child session/artifact, identify the canonical graph-valid correction, and unblock through CLI-managed validation/continuation, action-card safe commands, or by steering the same child to emit the corrected result. Do not ask the human to debug parser/graph mechanics.
- If manager state seems invalid or desynced, inspect it and help the pipeline recover instead of blocking. Compare `activeChild.stage`, child session/result, `workflow.current_node_id`, latest durable artifact, and graph intent. If evidence is deterministic (for example active child is `implement` and the child emitted an implementation handoff while graph cursor is stale at `review-plan` after a manual skip), repair local/ephemeral manager state or use the closest CLI recovery path, then continue. Do not mutate durable QRSPI artifacts to hide manager-state bugs.
- Escalate to the human only when intent, product judgment, safety, workspace replacement, merge policy, or project-specific decision is ambiguous. Do not escalate merely because the manager needs to map an obvious child result to a canonical outcome.
- Keep self-recovery evidence-based: cite the child artifact/session and the graph rule being corrected in manager prose/diagnostics. Do not invent stage results without durable child work/artifacts to back them.
- Log q-manager recovery incidents, but do not block the pipeline merely to write a perfect report. CLI repair paths append local recovery records under the manager state directory when available. Incident logs are local/ephemeral diagnostics; keep durable artifacts focused on workflow truth.

## Linear ticket artifact comments

When the active QRSPI plan is operating on a Linear ticket, move the ticket to In Progress and add concise progress comments as stage artifacts are produced.

1. Detect the Linear issue from durable plan context, preferring `[plan_dir]/context/question/linear/issue.json`, then `ticket.md`, `AGENTS.md`, or prior captured QRSPI artifacts. Do not guess an issue ID when none is recorded.
1. Detect the preferred Linear comment thread from durable plan context when present, preferring an explicitly recorded project-planning comment/thread ID or URL in `[plan_dir]/context/question/linear/`, `ticket.md`, `AGENTS.md`, or prior captured QRSPI artifacts. If the recorded URL has a fragment like `#comment-c04ee576`, resolve it to the canonical Linear comment ID required by the CLI/API before using it as a parent. Do not guess a parent comment when none is recorded or resolvable.
1. Before launching the first child stage for that ticket, move the issue to In Progress. Use `linear-cli issues update <ISSUE_ID> --state "In Progress"`; use `linear-cli issues start <ISSUE_ID>` only when assigning the ticket to the current Linear user is also intended.
1. After `continue` validates a child result, summarize the stage in 1-4 bullets and include markdown links to the produced/updated artifacts from `qrspi_result.artifact` and `qrspi_result.artifacts`.
1. Use durable links only: provider/web URLs when available, or thoughts-relative artifact paths formatted as markdown links. Do not link machine-local absolute paths, q-manager state files, tmux panes, session JSONL, prompt files, or other ephemeral control refs.
1. Post with `linear-cli comments create <ISSUE_ID> --body "$(cat /tmp/q-manager-linear-comment.md)"`. If a preferred project-planning/manager thread parent is explicitly recorded and resolvable, include `--parent-id <COMMENT_ID>` so status updates stay in that thread; otherwise create a normal ticket comment.
1. Keep comments human-readable and non-spammy. One comment per validated stage result is enough; include stage, status/outcome, artifact links, and next stage. Do not paste full `qrspi_result` YAML unless the ticket workflow explicitly requires it.
1. If Linear CLI/auth fails for state move or comment posting, report the failure and continue QRSPI unless the human made Linear updates a hard requirement.

Comment shape:

```markdown
QRSPI update: `q-plan` complete

- Wrote implementation plan: [plan.md](thoughts/.../plan.md)
- Review notes: [plan review](thoughts/.../reviews/.../review.md)
- Next: `/q-workspace` now
```

## Rules

- Existing QRSPI graph is canonical. Do not infer transitions from YAML text alone.
- Use `cmd/vamos-runtime` helpers, not a new binary.
- Child work must be visible and interruptible in tmux. No hidden background child runner as primary UX when using the q-manager runtime/CLI flow.
- Non-tmux/background-process orchestration is out of scope for this skill. Use a dedicated wrapper skill for background managers; q-manager itself should use the tmux `start-next` flow and visible child panes.
- Do not manually launch child stages with raw `tmux split-window` or `pi -p` as a substitute for `vamos qrspi start-next`. Raw splits bypass q-manager state, child session wiring, validation, and wake delivery. `pi -p` is non-interactive and is not the visible child-pane UX.
- If a raw tmux split is required only for emergency debugging, always target the manager pane explicitly with `tmux split-window -t "$TMUX_PANE" ...`; otherwise tmux may split a different selected window/pane than the manager. Label it diagnostic, do not expect a q-manager wake, and recover through `rebind-child` / `validate-latest` / `recover-manual` before continuing.
- Manager state is disposable control state under user state dir, keyed by canonical `plan_dir`; never use repo-local `.vamos/q-manager`.
- QRSPI artifacts and fenced `qrspi_result` YAML remain durable truth.
- Respect manager-owned policy. `guided` is the default. `advanceMode`: `discuss` stops after valid result; `guided` auto-resumes graph-authorized agent-node `status: handoff` checkpoints in a fresh same-node q-resume child; `autopilot` does the same and can auto-approve only graph-marked safe gates. Plan reviews are controlled independently by `enablePlanReviews`. Child-emitted policy and `next.steps` are informational only; neither authorizes movement.
- Stop on human gates, implementation complete/review-ready results, blocked/error results, invalid result retry exhaustion, lock conflict, or ambiguous project judgment named by `docs/q-manager.md`. A graph-authorized handoff is not a stop in guided/autopilot: `child-complete` durably launches the replacement before sending an informational wake. When `continuation_started=true`, do not run `continue` or launch a duplicate child. Discuss mode validates the handoff but waits.
- Treat Pi `agent_settled` as the normal child completion boundary. Interruption chat is not completion and consumes no result-repair budget. A normal wake is validated manager-needed state, not raw child turn end. Expect `q_manager_child_wake.validated=true` for graph-valid states or `retry_exhausted=true` when automated result repair failed. Ignore raw `agent_end`, `done`, and `status.json` as normal manager triggers.
- Wake identity is child generation plus evidence content. Same generation/content suppresses; changed content or managed steering wakes. `steer-child` persists a new cursor-anchored generation before feedback delivery. Wake intent is persisted before tmux paste; explicit `manager-ready` may redeliver once after a crash between paste and finalize, preferring a possible duplicate over a lost wake.
- A graph-valid post-cursor result may outrank a later provider/context failure only when its declared primary artifact is a regular file inside the canonical current plan/review tree. Missing, absolute, unrelated, directory, `..`, or symlink-escaping paths keep provider recovery manager-actionable. Artifact existence never repairs invalid YAML.
- `q_manager_request` is operational evidence only. A valid nested-plan follow-up request creates a deduplicated inspect/action card; it never mutates the graph or launches a child. Natural-language follow-up requests have no plan-path authority. Only canonical `qrspi_result` transitions or explicit audited `repair-state --set-node ...` operations advance state.
- Before raising a blocked result to the human, diagnose the blockage from deterministic evidence: read the blocker artifact, reproduce the failing command when safe, compare against a clean/main baseline when feasible, distinguish stack-caused vs baseline/environment failures, and report a concise summary plus recommended next action. Do not simply echo `status: blocked`.
- After `/q-workspace`, run implementation/review/verify child stages in `workspace_metadata.implementation_workspace` when graph semantics require implementation cwd.
- For implementation-review follow-up/review-dir plans that already have `workspace_metadata.implementation_workspace`, do not imply a fresh workspace/copy/reset. Prompts for `/q-plan` and planning review must state that implementation should stack in the existing implementation workspace on the reviewed head. If the current graph forces a `workspace` node anyway, that node must preserve/reaffirm the existing workspace and continue to implementation; do not create a new copy.
- Pi session metadata redesign is out of scope; q-manager assigns exact child `--session-id` / `--session-dir` using current Pi flags. Child Pi sessions belong in the plan workspace `.sessions/pi/` directory, not the local manager state directory, so humans can easily inspect and `pi --resume` a stage session from the workspace.
- Child session JSONL is authoritative for result parsing. tmux transcript/output is diagnostic.
- Stay high-level as manager. When an active child has adequate stage context, do not edit that child’s plan/design/outline artifacts yourself; gather human feedback and steer the same child to apply it. Manager-owned edits should be limited to manager operational notes/skills unless explicitly asked otherwise.
- Treat human feedback as first-class child input. If the human says important context, corrections, priorities, approvals, or objections to the manager, decide whether it is relevant to the active child’s current task. If relevant, enrich it with minimal routing context and forward it to the same child via the q-manager CLI steering path. Do not keep important task context trapped in the manager transcript.
- YAML/result formatting errors are normally CLI/extension work. The q-manager CLI and Pi child extension own detection, correction prompts, retries, and parser-specific feedback for invalid `qrspi_result` YAML. The manager should run `continue` and report concise retry/exhaustion state; it should not handcraft parser correction prompts or mix parser correction with human/task feedback. If retry support fails or is exhausted but the correction is deterministic from child artifact + graph (for example `review-plan` used `outcome: complete` but graph requires `ready-for-workspace`), the manager may self-recover by steering the same child to emit the canonical corrected YAML or by using a future CLI correction/apply-result helper, without human intervention.
- Never paste multiline manager prompts into an interactive child pane as raw tmux keystrokes. Newlines can submit as separate child prompts. For initial stage prompts, use `start-next`; it writes a prompt file and launches the child. For follow-up steering, use `steer-child` with a feedback file. Do not silently fall back to direct tmux as the normal path.
- Do not poll or sleep on child `done` as the normal control loop. `done`/`status.json` are recovery diagnostics; the primary manager trigger is the child wake pasted into the parent pane.
- Do not put manager `stateFile`, run IDs, pane IDs, session dirs, or other disposable q-manager control refs in durable `qrspi_result` YAML. Report them in manager prose/diagnostics only. Durable YAML should keep plan/workspace/artifact identity, not machine-local manager state.
- When testing the runtime CLI from a Vamos checkout, use `go run ./cmd/vamos-runtime ...` in place of installed `vamos ...`.
- Child prompts should be stage-work prompts, not manager runbooks. The primary child context should be the previous stage's fenced `qrspi_result` YAML plus minimal routing metadata needed to read planning docs and start the selected stage. The CLI should pass that YAML directly to the child prompt from manager state/session JSONL; do not paste the YAML into the parent manager chat.
- Manager-specific instructions from `docs/q-manager.md` are for the parent manager/CLI. Do not embed the full manager manifest in every child prompt. If the CLI needs manifest-derived child context, render a small normalized child-safe summary, not raw docs.
- q-manager may accept extra operator context for a child, but that context should be explicit and additive. A valid previous `qrspi_result` should normally be sufficient for the child to read the plan docs and proceed.
- Keep manager context lean. Do not request verbose machine output such as `--output ndjson` on normal `start-next`, `continue`, or `steer-child` commands. Use the default text output in the wake-driven loop; switch to NDJSON only for targeted debugging, parser/graph recovery, or when a recovery command explicitly requires structured fields.
- If the human wants a faster/different child model, pass `--model` on `start-next`, `continue`, or `run-child`. Prefer provider-qualified IDs such as `openai-codex/gpt-5.4` instead of bare `gpt-5.4`; bare names may route to the wrong provider and trigger auth errors (for example Azure API key errors).

## Wake-driven manager loop

Startup may begin from a bare `/q-manager`, a pasted child `qrspi_result`, a manager wake, or an explicit direct operation. Resolve that input first; the steady-state loop below still uses the canonical `start-next` / `continue` operations internally.

Primary loop: launch/resume child with `start-next`, then wait for a validated pasted wake. Do **not** block this manager session in `sleep`/poll loops. The extension wake is the normal event; marker files are only fallback diagnostics.

1. Resolve plan dir and project root.
1. Start or resume the graph-selected child with one parent Pi command:
   ```text
   /q-manager start-next --plan-dir <plan-dir> --project-root <repo-root> --manager-pane "$TMUX_PANE"
   ```
   This is the happy-path child launch. It must be run from the manager Pi/tmux pane so `$TMUX_PANE` identifies the pane to split beside and wake later. If `$TMUX_PANE` is missing or stale, stop and fix the manager tmux context instead of falling back to raw `tmux split-window`.
   For fast outline-first work where the human aligns on the outline and then q-manager should go straight through plan -> implement with plan reviews off, launch with `--node outline --policy-preset fast`.
   If the human requests a specific child model, add `--model <provider/model>` such as `--model openai-codex/gpt-5.4`.
   Add `--node <node>` / `--implementation-cwd <cwd>` only when deliberately resuming or testing a specific implementation, review, or verify stage. The parent wrapper samples `ctx.getContextUsage()` and passes fresh `--manager-usage-*` flags to the CLI; the Go CLI must not scan parent Pi JSONL for usage. Fresh parent usage `>=90%` makes the CLI write an operational handoff, save delivery `compacting`, and print the exact `manager-ready` command; only after that stable signal does the wrapper call native `ctx.compact()`. Raw `vamos qrspi start-next ... --manager-usage-*` remains a debug/manual seam. Missing usage skips compaction; do not guess from token totals alone. If the parent already has a latest fenced result, pass it with `--latest-result-stdin` or `--latest-result-file`; the CLI validates/persists it before prompt embedding.
1. Capture `stateFile` and active child refs from the concise default text output. The CLI writes the child prompt file atomically and launches the visible child next to the manager pane; do not hand-render prompts, paste prompts, run `pi @prompt` yourself, or launch raw tmux panes on the happy path. Do not add `--output ndjson` on the happy path; it bloats the manager context.
1. Stop issuing commands and wait for the child extension/CLI to deliver a validated `q_manager_child_wake`. If manager delivery is `compacting`, the wake queues; after parent reset/restart, run the printed `vamos qrspi manager-ready --state-file "$STATE" --manager-pane "$TMUX_PANE"` command once to mark ready and flush exactly one lineage-current wake. The wake includes `continuation_started`. A successful handoff auto-resume is informational (`manager_needed=false`), identifies the same-node q-resume child, and deliberately has no continue action. Inspect/repair only a failure card or queued delivery; never duplicate the continuation.
1. For wakes without `continuation_started=true`, run the single normal parent Pi continuation command from the wake's state file:
   ```text
   /q-manager continue --state-file "$STATE"
   ```
   Raw `vamos qrspi continue --state-file "$STATE"` is the debug/manual fallback and does not invoke native parent compaction by itself. If the human requested a model override for future children, keep passing it here too, for example `--model openai-codex/gpt-5.4`.
   `continue` validates the active child session JSONL, reconciles active-child health from tmux/status/session evidence before YAML retry, reprompts the same child while retry remains, persists canonical graph decisions for valid results, launches graph-selected next child when safe, and reports concise stop reasons. Repairable failures return action cards with evidence and exact safe commands such as `repair-state --align-active-child && continue`, `repair-state --clear-failed-child --relaunch`, or context-exhaustion recovery; run those commands when evidence is deterministic. Do not paste raw validate/decide NDJSON into manager chat, and do not handcraft child correction prose.

For review-dir / implementation-review follow-up plans, same-workspace routing should come from previous `qrspi_result.workspace_metadata` and plan docs. If the CLI detects and summarizes it, keep the summary child-safe and minimal: do not create a new implementation copy or reset to trunk; stack follow-up implementation on the existing implementation workspace/head.

### Manual/debug lower-level commands

Use these only for recovery or debugging when `start-next` / `continue` is insufficient. Prefer `run-child` over raw `tmux split-window`; it preserves q-manager refs and wake behavior.

```bash
vamos qrspi init --plan-dir <plan-dir> --project-root <repo-root> --manager-pane "$TMUX_PANE"
vamos qrspi doctor --state-file "$STATE" --output text
vamos qrspi render-prompt --state-file "$STATE" --node <node> --plan-dir <plan-dir> > /tmp/child-prompt.md
vamos qrspi run-child --state-file "$STATE" --plan-dir <plan-dir> --stage <node> --cwd <cwd> --prompt-file /tmp/child-prompt.md --split right --timeout 0
vamos qrspi validate-result --state-file "$STATE" --stage <node> --plan-dir <plan-dir>
vamos qrspi reprompt-child --state-file "$STATE" --plan-dir <plan-dir> --stage <node> --attempt <n> --error-file <validation-error-file>
vamos qrspi repair-state --state-file "$STATE" --align-active-child
vamos qrspi repair-state --state-file "$STATE" --set-node <result-source-node> --from-result <result-file> --implementation-cwd <cwd> --reason <operator-reason>
vamos qrspi repair-state --state-file "$STATE" --set-node <result-source-node> --from-session <jsonl> --implementation-cwd <cwd> --reason <operator-reason>
vamos qrspi repair-state --state-file "$STATE" --clear-failed-child --relaunch
vamos qrspi mark-child-active --state-file "$STATE" --child-id <id> --reason manual-reprompt
vamos qrspi set-policy --state-file "$STATE" --preset guided
vamos qrspi set-policy --state-file "$STATE" --preset autopilot
vamos qrspi set-policy --state-file "$STATE" --preset autopilot-no-plan-reviews
vamos qrspi set-policy --state-file "$STATE" --preset fast
vamos qrspi set-policy --state-file "$STATE" --advance-mode autopilot --enable-plan-reviews=true
vamos qrspi inspect --state-file "$STATE" --sessions --latest
vamos qrspi find-latest-child --state-file "$STATE" --stage <node>
vamos qrspi rebind-child --state-file "$STATE" --session-file <jsonl> --stage <node> --reason manual-new
vamos qrspi validate-latest --state-file "$STATE" --stage <node> --apply-rebind
vamos qrspi recover-manual --state-file "$STATE" --mode latest-session --continue
vamos qrspi decide-next --state-file "$STATE" --plan-dir <plan-dir>
```

Manual/debug overrides: `--session-file <jsonl>` validates a specific child session JSONL. `--result-file <path>` is deprecated fallback for plaintext result files only when no active child session refs are available. Prefer latest-session recovery commands over state-file edits when a human chatted in the same child pane or used child `/new`.

### Runtime CLI testing with `go run`

When the user asks to test the runtime CLI before the installed `vamos` binary includes a command, prefix commands with `go run ./cmd/vamos-runtime`. Keep the same wake-driven shape:

```bash
go run ./cmd/vamos-runtime qrspi start-next --plan-dir "$PLAN" --project-root "$PWD" --manager-pane "$TMUX_PANE" --node <node> --implementation-cwd "$PWD"
```

After `start-next`, do not poll. Wait for the validated pasted wake, then run `go run ./cmd/vamos-runtime qrspi continue --state-file "$STATE"`.

## Child wake contract

q-manager loads a project-local child Pi extension only for q-manager child sessions. The extension/CLI should wake the manager only after validated manager-needed state exists: a graph-valid result, a human/block/error stop, a safe next action, or retry exhaustion. Intermediate invalid/missing YAML, parser retries, and Codex/SSE header noise are local child/CLI retry state, not manager wakes.

The manager CLI/extension owns the exact wake text so it stays deterministic, testable, and versioned with runtime behavior. The Pi child extension invokes Go `qrspi child-complete`; Go is responsible for parsing, graph decisions, deterministic positive outcome normalization, `validation-status.json`, handoff continuation launch, and deliver/queue/suppress decisions. The wake is one atomic parent prompt with validation flags, `continuation_started`, stage/status/outcome/artifact, and local `state_file`. Non-auto-resumed wakes point to continue. Successful auto-resume wakes contain no continue action because the fresh same-node q-resume child is already durable.

`retry_exhausted=true` means automated correction failed. The manager should inspect child output/artifacts, recover or steer deterministically when evidence is sufficient, and ask the human only when intent, safety, product judgment, workspace replacement, merge policy, or external authority is required.

The wake may include `state_file` because that is ephemeral manager control context needed to continue the local run. This value belongs in the wake/manager transcript, not in durable QRSPI artifacts or `qrspi_result` YAML. A multiline wake must be pasted as one buffered/atomic prompt (the same style q-manager uses when injecting blocks into child panes), not sent line-by-line as raw tmux keystrokes.

## Result retry

If validation fails and policy retry budget remains, `continue`/CLI retry support should run `reprompt-child` with the validation error file. It pastes/injects the canonical QRSPI parser correction prompt into the same active child pane/session as one atomic prompt; do not create a new child ID/session and do not manually paste extra multiline correction prose. If the only problem is deterministic positive wording, the CLI should normalize before retrying. If retry budget is exhausted, emit one manager-needed retry-exhausted notice with deterministic-recovery-first guidance; inspect/steer/recover before asking the human.

## Cleanup and recovery

Supported manual interaction modes: normal managed child, `steer-child`, same-child chat, child `/new`, manual completion, retry exhaustion recovery, and stale wake supersession. Use recovery commands before state-file edits.

- Invalid result: keep active child pane/session and reprompt in place while retry remains.
- Human gate, blocked, error, or retry exhaustion: keep pane/session for inspection and human steering.
- Action cards are the first-class manager UX for repairable failures: `state_desync`, `state_alignment`, `graph_outcome_mismatch`, `workspace_moved`, `active_child_conflict`, `human_gate`, `invalid_child_yaml`, `manual_child_steer`, `superseded_queued_wake`, `child_followup_request`, `pi_compatibility_failed`, `child_launch_failed`, `child_context_exhausted`, `invalid_handoff_artifact`, `handoff_continuation_failed`, `manager_delivery_failed`, and `pending_child_cleanup_failed`.
- Run `vamos qrspi doctor --state-file "$STATE"` when launch compatibility, tmux, state-root, or active-child health is unclear; it summarizes Pi compatibility, manager state root writability, tmux health, active-child health, latest status, and safe recovery command.
- If `repair-state --align-active-child` is offered, use it only for its narrow active-child cursor repair. Prefer `repair-state --set-node <source> --from-result|--from-session ... --reason ...` when completed result evidence exists: it validates the evidence against the named source node, applies the parsed transition decision, records pending/applied provenance, and supersedes stale child/wake state. Never set the cursor back to a source stage whose result already completed.
- If `repair-state --clear-failed-child --relaunch` is offered for `child_launch_failed`, use it only when the card/doctor proves a terminal failed child (nonzero status/done/no `qrspi_result`); it clears the failed active child and relaunches the same graph node. `start-next --force` may replace only terminal failed children and must still protect running/unknown children.
- If a human manually continued the active child or used child `/new`, run `inspect --latest` or `find-latest-child`, then `validate-latest --apply-rebind` or `recover-manual --mode latest-session --continue`. Do not edit manager JSON with jq/python.
- If an action card reports `child_context_exhausted`, distinguish it from no-wake. No-wake with valid result uses latest-session recovery. Context-exhausted/no-result children keep refs, use child `/compact` or `pi --resume <session>` only when context-limit evidence is real, steer/resume/rebind to obtain valid YAML when possible, or relaunch the same graph node only after no trustworthy result/artifact can be salvaged. Never invent YAML or advance from artifacts alone.
- `steer-child` and managed parser correction automatically begin a new evidence generation and supersede stale queued/pending wakes before sending input. Use `mark-child-active` only after out-of-band/manual pane input that bypassed those managed paths; `manager-ready` should then wait for newer completion instead of flushing stale payload.
- Normal valid transition with `startNext=true`: mark old child pending cleanup; start next child; kill old pane only after the new active child is saved.
- Graph-authorized handoff in guided/autopilot: persist decision/claim, start and save a fresh same-node q-resume child, write source validation status, deliver or durably queue an informational wake, then clean the old pane. Discuss mode does not launch.
- Queued continuation wake remains current while its delivery ID matches the running replacement lineage. Paste failure retries paste+submit; Enter failure retries submit-only on the same pane or re-pastes after pane adoption.
- Handoff artifact/launch failure emits a deterministic manager action card and retains source refs. Cleanup failure keeps the replacement active and pending cleanup retryable; an already-absent old pane is success.

## Manager session handoff

Use this when the parent manager Pi context is getting full, before auto-compaction or session loss. This is separate from a QRSPI implementation handoff: it transfers manager control context, not implementation reasoning.

A manager handoff has two outputs, in this order:

1. A markdown handoff artifact under the active plan directory, usually `[plan_dir]/handoffs/YYYY-MM-DD_HH-MM-SS_q-manager-operational-handoff.md`.
1. A fenced `yaml` `qrspi_result` response in the current manager chat that points at that handoff artifact.

Do not rely on chat history. The markdown handoff must contain enough local recovery refs for a fresh manager session to resume from deterministic sources. Include:

- Plan dir absolute path and `thoughts/...` relative path.
- Project root / source checkout cwd.
- Implementation cwd when known.
- Current graph node and last completed stage.
- Latest durable `qrspi_result` YAML or path to the artifact containing it.
- Manager `stateFile` absolute path, explicitly labeled local/ephemeral.
- Active child refs from state when a child is running: stage, pane ID, session ID, session dir/path, status path, done path, output/transcript path.
- Whether the manager is waiting for child wake, should run `qrspi continue`, needs a lower-level recovery command, or is stopped at a human gate.
- Exact next command, using `go run ./cmd/vamos-runtime ...` when testing from checkout.

Manager handoff may include `stateFile` because it is an operational recovery note for the same local machine. Do not put `stateFile`, pane IDs, session dirs, or run IDs in durable `qrspi_result` YAML. In the markdown handoff, label those fields as local/ephemeral and keep durable plan identity (`thoughts/...` paths, artifact paths, latest result) separate from local recovery refs. Auto-compaction handoffs are manager operational handoffs: resume from the handoff, then run `manager-ready` so any queued validated child wake flushes to the current parent pane.

The final manager response must be a normal QRSPI-style YAML block so the next manager can discover the handoff artifact. Use `status: handoff`, omit `outcome`, set `artifact` to the manager handoff path, and include `next.steps` that read q-manager, read the handoff, then start `q-manager continue`. The YAML may mention `stateFile` in `summary.key_decisions` only as prose if needed, but must not include machine-local refs as structured fields.

Prefer a dedicated `q-manager-handoff` skill/helper over overloading `/q-handoff`. `/q-handoff` is a QRSPI stage artifact for work continuity and should stay portable. A manager handoff is control-plane recovery and intentionally includes machine-local refs. If no dedicated helper exists, create the markdown note yourself under the plan's `handoffs/`, run `just sync-thoughts` when appropriate, then emit the required fenced `qrspi_result` pointing at it.

Fresh manager resume shape:

```bash
# read q-manager skill, docs/q-manager.md, plan AGENTS.md, manager handoff first
STATE=<stateFile-from-manager-handoff>
go run ./cmd/vamos-runtime qrspi continue --state-file "$STATE"
```

If the handoff says “waiting for child wake,” do not continue/validate until wake arrives unless manually inspecting recovery state. If no active child exists, resume by rendering and running the graph-selected/current node from the saved state.

## Human gates

Ask the human one direct question. Preserve graph decision, latest result, and any human answer in manager session context. Do not rewrite workflow state by hand.

Before every approval question, provide a concise decision summary sufficient for the human to approve or correct the proposal. Never ask a bare `Approve?` or `Vamos?`. For outline alignment, summarize the proposed outline’s scope, parts or vertical slices, key invariants/tradeoffs, and explicit exclusions in a short bullet list, then ask `Vamos?`. When `outline.md` does not exist yet, summarize the child’s proposed outline/alignment from its latest assistant text and say that approval authorizes writing it; do not imply the artifact already exists.

If a child stops for a graph-valid human gate, keep the child pane/session active. Summarize the child’s question and approval context to the human, then steer the same child with the answer. If an approval-seeking child turn produced no `qrspi_result`, inspect the authoritative child session, surface the same concise approval summary, and treat it as a human gate rather than exposing parser mechanics to the human. Use the CLI helper that injects one atomic prompt:

```bash
vamos qrspi steer-child --state-file "$STATE" --feedback-file /path/to/human-feedback.md
```

The steering command is human/task feedback first-class, separate from YAML validation retry. It preserves active child refs, accepts a feedback file, injects one atomic child prompt, and lets the child update artifacts or ask follow-up. Do not edit the child’s design/outline/plan artifacts directly when the child can incorporate the feedback. Do not use parser `reprompt-child` for human feedback.
