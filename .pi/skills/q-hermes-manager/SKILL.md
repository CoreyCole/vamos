---
name: q-hermes-manager
description: Hermes-managed QRSPI orchestration using background Pi processes instead of tmux panes. Use when asked for q-hermes-manager, Hermes to manage QRSPI, background QRSPI manager, or auto-advance QRSPI stages from Hermes. Carries full qrspi_result YAML between stages and pauses only for true gates.
---

# q-hermes-manager

Manage a QRSPI workflow from Hermes by launching each graph-safe stage as a tracked background Pi process. This is a wrapper/orchestration skill, not the tmux `q-manager` runtime.

## Step 1: Establish mode and load context

State the mode clearly before launching work:

- Mode: Hermes-managed background Pi processes.
- Not mode: tmux q-manager visible child panes.
- Readiness signal: background process exits successfully, then full process log contains valid fenced `qrspi_result` YAML.

Load these before acting:

1. `.pi/skills/qrspi-planning/SKILL.md`.
1. `.pi/skills/qrspi-planning/references/background-pi-stage-delegation.md`.
1. Target plan `AGENTS.md` when present.
1. Latest QRSPI result artifact or user-provided fenced `qrspi_result` YAML.
1. For a concrete example of notification parsing, artifact-path preservation, and direct-outline missing-artifact pitfalls, see `references/webhook-forwarding-run-2026-07-02.md`.
1. For a concrete example of repeated implementation handoff auto-advance, mid-run lead-engineer corrections, and cross-repo reviewer-assignment planning/implementation, see `references/reviewer-context-routing-2026-07-08.md`.
1. For a concrete example of a long direct-outline workflow with seven implementation handoffs, prompt-file reuse, truncated completion snippets, and exact next-target carry-forward, see `references/deterministic-morning-sync-cli-2026-07-14.md`.

Do not use `vamos qrspi start-next` as the primary path. That command belongs to the tmux `q-manager` skill.

## Step 2: Choose the next stage

Use the latest validated `qrspi_result` as the source of truth for routing.

- Preserve the full previous fenced YAML verbatim for the next prompt.
- Follow `next.steps` and the QRSPI graph intent.
- Use `workspace_metadata.implementation_workspace` as cwd after `/q-workspace` when implementation/review/verify semantics require it.
- Use the plan workspace before `/q-workspace` and for planning artifacts.
- For implementation `status: handoff`, route to `/q-resume` using the handoff artifact; do not pause just because a handoff exists.

Pause only for:

- `needs_human`, `blocked`, or `error`.
- Invalid/missing `qrspi_result` YAML.
- Failed background process exit.
- Invalid artifact or impossible graph transition.
- Real safety, lost-work, merge, manual-test, or product judgment decision.

## Step 3: Write the child prompt file

Put large prompts in `/tmp/...` and invoke Pi with the prompt file so the prior YAML is not truncated.

Prompt must include:

````text
You are the [stage] stage subagent for a QRSPI workflow.

Run from cwd: [absolute cwd]

Task: [specific stage task and primary artifact path]

Follow these skills exactly:
- First read /Users/coreycole/dotfiles/context/vamos/.pi/skills/qrspi-planning/SKILL.md
- Then read /Users/coreycole/dotfiles/context/vamos/.pi/skills/[stage]/SKILL.md
- Load required artifacts from the recorded workspace.

User instruction for this run:
- This workflow is managed by Hermes background orchestration.
- Do not request approval merely to advance graph-safe stages.
- Ask only for real human context, safety, blocker, lost-work, merge, or manual-test decisions.
- Preserve workspace_metadata, artifact paths, policy, and next.steps from the previous result.

IMPORTANT previous stage result:

```yaml
[full previous qrspi_result YAML verbatim]
```

When complete, emit the required fenced yaml qrspi_result followed by the concise stage summary.

````

For the first stage when no previous result exists, include the user's request, plan directory, repo cwd, desired starting stage, and the same QRSPI skill-loading requirements.

If the user asks to "delegate the remaining review/implementation" after a QRSPI result was produced earlier in the conversation, reconstruct the latest valid fenced `qrspi_result` from the chat and launch the next graph stage immediately. Example: after `stage: outline` with `next.steps` ending in `review-outline`, write a review-outline prompt that includes the full outline result YAML, run from the project repo cwd, and start the background Pi process without asking for another approval.

## Step 4: Launch the background Pi process

From the selected cwd, start Pi in a Hermes background process with completion notification. In Hermes, use `terminal(background=true, notify_on_complete=true)` or the equivalent background-process tool:

```bash
pi -p @/tmp/q-hermes-manager-[stage]-prompt.md
```

If the user's environment has a known absolute Pi path, prefer it over relying on cron/shell PATH. On swarm machines, use:

```bash
/Users/swarm/.npm-global/bin/pi -p @/tmp/q-hermes-manager-[stage]-prompt.md
```

Track completion with the returned process ID using process polling/log tools such as `process(action="poll")` and `process(action="log")` when available.

Track and report:

- Process ID.
- Cwd.
- Prompt file path.
- Stage.
- Readiness signal: process exit plus full log parsed.

Do not claim a tmux child pane or q-manager wake exists. This mode does not use `q_manager_child_wake`.

## Step 5: On completion, parse the full log

When a process completes:

1. Treat Hermes' `[IMPORTANT: Background process ... completed]` user-delivered notification as a real completion signal, but do not trust the truncated notification body as the whole result.
1. Read the full process log through the process log tool, not just the notification snippet.
1. If the completion notification arrives in the same user turn as a lead-engineer correction or clarification, treat that correction as active guidance for the next stage. Preserve the prior full `qrspi_result`, but include the new clarification verbatim in the next prompt as settled alignment or as a required design constraint before launching the next graph-safe stage.
1. Confirm the process exited successfully.
1. Extract the complete fenced `qrspi_result` YAML.
   - If the full log contains a complete top-level `qrspi_result:` block but the wrapper stripped the opening fence, accept it only when indentation is coherent and all required fields are present.
1. Validate that stage/status/outcome/artifact/next route are coherent enough to continue.
1. Preserve artifact paths exactly from the YAML when constructing the next prompt. Do not reconstruct paths from memory or shorten them; a single missing `reviews/` segment or plan-directory segment can send the next agent to the wrong artifact.
1. Before launching the next prompt, sanity-check each artifact path against `workspace_metadata.plan_workspace`: relative paths should resolve under the plan workspace, and absolute paths should point at the recorded workspace. If a copied path is inconsistent, preserve the full previous YAML verbatim and add a prompt note telling the child to prefer the actual discovered path under `plan_workspace` and emit corrected exact paths in its new result.
1. If `next.steps` references optional artifacts that may not exist in direct-outline workflows (especially `design.md`), keep the step in the preserved YAML but explicitly tell the next Pi prompt not to block if that optional artifact is absent and the plan/outline/handoff are sufficient.
1. If valid and graph-safe, immediately launch the next background Pi process before giving a long prose update.

If invalid or failed, stop and summarize:

- Process ID and cwd.
- Failure category.
- Last valid artifact/result if any.
- One recommended next action.

## Step 6: Continue implementation loops correctly

For implementation stages:

- `/q-implement` and `/q-resume` should perform one unchecked implementation checkpoint per process.
- Intermediate `status: handoff` is a recovery checkpoint, not a human gate.
- Start the next `/q-resume` process with the full handoff YAML and exact handoff artifact path.
- If the workflow spans multiple repositories, keep `implementation_workspace` as the primary workspace from q-workspace but allow the next-stage prompt to use cross-repo checkouts only when the plan/handoff explicitly calls for it. Tell the child to run checkout safety checks and preserve unrelated work before editing the related repo.
- When a background implementation handoff names the next target (for example “docs/runbooks”, “regression matrix”, “bridge/wiring”, “Slack rendering/lint”, or “selective document publication”), include that exact target in the next prompt so the child does not waste context rediscovering the next checkpoint.
- For long repeated `/q-resume` loops, it is acceptable to reuse a stable `/tmp/q-hermes-manager-q-resume-prompt.md` path only if you overwrite it with the newest full prior YAML, newest handoff path, and exact next target before each launch. Track continuity by process ID and handoff artifact, not by prompt filename.
- If a direct-outline workflow intentionally lacks `design.md`, keep telling resume/review prompts not to block on absent optional design artifacts when AGENTS.md, outline, plan, and handoffs are sufficient.
- Stop only when implementation is complete/review-ready, blocked, invalid, failed, or needs human input.

## Step 7: Incorporate mid-run human corrections

If the user replies while orchestration is in progress with a clarification or correction:

- Treat it as settled alignment for subsequent stages unless it conflicts with safety or plan invariants.
- Patch the very next child prompt with the exact correction and label it as lead-engineer guidance.
- Do not restart completed stages merely to restate the correction; carry it forward into research/design/plan/implementation prompts and artifacts.
- Example: if the user clarifies an assignment invariant after q-question, pass it into q-research as settled alignment so design and implementation inherit it.

## Step 8: Report concisely

After starting or advancing a stage, report only useful manager state:

```text
q-hermes-manager: started [stage]
Process: [id]
Cwd: [cwd]
Prompt: [path]
Ready when: process exits and full log contains valid qrspi_result YAML.
```

After a valid completion and next launch:

```text
Completed: [stage] -> [status/outcome]
Artifact: [artifact]
Started next: [next stage]
Process: [id]
```

## Boundaries

- Use this skill only for Hermes background-process orchestration.
- Use `.pi/skills/q-manager/SKILL.md` when the user wants tmux panes, `vamos qrspi start-next`, manager wakes, or the q-manager CLI/runtime loop.
- Do not mix the two modes in one run unless the human explicitly asks to switch.
- Keep durable QRSPI truth in artifacts and fenced `qrspi_result` YAML. Keep process IDs and prompt files in manager prose only.
