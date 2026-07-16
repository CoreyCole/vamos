---
name: q-resume
description: Resume work within a QRSPI planning pipeline from a handoff document
---

## QRSPI mode contract

QRSPI has a canonical advancement mode plus separate review/retry policy:

- `advanceMode=discuss`: do not advance after valid YAML. Keep chatting in the current session; show the validated next action and explicit continue/start button. Not default.
- `advanceMode=guided`: default. Auto-continue graph-safe non-human edges; stop at explicit human gates. Current `auto_mode=false` behavior.
- `advanceMode=autopilot`: auto-continue graph-safe non-human edges and auto-approve only human gates marked auto-approvable. Current `auto_mode=true` behavior.
- Legacy compatibility: until runtime persists `advanceMode`, map `auto_mode=false` to `guided` and `auto_mode=true` to `autopilot`. `discuss` needs a distinct runtime policy value.
- All modes still stop on `needs_human`, `blocked`, `error`, invalid artifact, disallowed transition, run failure, YAML retry exhaustion, or explicit safety gate.
- `enable_plan_reviews=true`: run planning `/q-review` after outline and plan. Do not run `/q-review` immediately after design; design advances directly to `/q-outline`.
- `enable_plan_reviews=false`: skip planning `/q-review`; final implementation `/q-review` always runs.
- Research never has its own human stop. Humans evaluate research in design/outline review.
- Emit the QRSPI YAML result as a fenced `yaml` block with top-level `qrspi_result` code block for every completed QRSPI stage result so it is syntax highlighted, then add only the mandatory concise human summary after it.

## QRSPI YAML summary contract

The `summary` element is used by humans to understand workflow state before asking follow-up questions or advancing. It must be structured, specific, self-contained, not a generic completion label. Use these child elements inside `summary`:

- `plan_goal`: overall plan/workflow goal in plain language; not just current stage label.
- `stage_completed`: what this stage/session did and how it moves toward the goal. Extremely concise; sacrifice grammar for concision.
- `key_decisions`: direction we are headed; significant tradeoffs, risks, open questions, follow-up, or why next step is safe. Use `None.` only when truly none.

Keep each child element short: 1-2 concise lines max.

For review stages, always include both: (1) what the entire implementation/plan now does as a whole, and (2) what this review session checked and changed. Do not write vague summaries like `review complete`, `implementation review result`, `done`, or `summary of findings` without the concrete details a human would need to ask informed questions.

## QRSPI footer instructions

When more than one artifact is relevant, keep `artifact` as the primary next-command artifact and also include `artifacts` with every important artifact path, including review records, done summaries, handoffs, ADRs, and follow-up questions.

Do not duplicate artifact lists or machine-control details in prose outside the YAML result result. For normal QRSPI stage completion, the response must be the fenced `yaml` `qrspi_result` block followed by a mandatory concise human summary; make both summaries specific enough for humans.

When resuming implementation, use the runtime node being completed, not a synthetic `resume` stage. Final implementation completion uses `` stage`implement`stage ``, `` status`complete`status ``, `` outcome`complete`outcome ``, and `next.steps` steps that read `qrspi-planning`, read `q-review`, read design/outline/plan, read the final handoff, then start `/q-review`. For non-final checkpoints, write a handoff artifact and continue through `/q-resume` in normal chat context; do not emit a completed workflow-node result unless intentionally stopping with `blocked`/`needs_human`.

```yaml
qrspi_result:
  project: "github.com/CoreyCole/vamos"
  related_projects: []
  stage: "[canonical node id]"
  status: "complete"
  outcome: "complete"
  workspace: "[absolute active QRSPI plan/ticket directory before q-workspace; omit after implementation workspace exists]"
  workspace_metadata:
    plan_workspace: "[absolute active QRSPI plan/ticket directory]"
    implementation_workspace: "[absolute implementation workspace when known]"
    trunk_branch: "main"
    stack_bottom_branch: ""
    parent_branch: ""
    current_branch: ""
  policy:
    advance_mode: "guided"
    auto_mode: false
    enable_plan_reviews: true
    invalid_result_retry_limit: 1
  summary:
    plan_goal: "[overall goal]"
    stage_completed: "[specific work completed]"
    key_decisions: "[decisions, risks, follow-up, or why next step is safe]"
  artifact: "thoughts/..."
  artifacts:
    - role: "related"
      path: "thoughts/..."
  next:
    steps:
      - action: "read_skill"
        param: ".pi/skills/qrspi-planning/SKILL.md"
      - action: "read_skill"
        param: ".pi/skills/[concrete next-stage]/SKILL.md"
      - action: "read_artifact"
        param: "thoughts/..."
      - action: "start_stage"
        param: "[concrete next-stage]"
```

# Resume Pipeline Handoff

> **Pipeline overview:** `.pi/skills/qrspi-planning/SKILL.md`

## Runtime YAML contract

Every response that completes a QRSPI workflow node must include a fenced `yaml` block with top-level `qrspi_result`, followed by a mandatory concise human summary. Do not use prose-only `Artifact` / `Summary` / `Next` completion responses.

Required shape:

```yaml
qrspi_result:
  project: "github.com/CoreyCole/vamos"
  related_projects: []
  stage: "[canonical node id]"
  status: "complete"
  outcome: "complete"
  workspace: "[absolute active QRSPI plan/ticket directory before q-workspace; omit after implementation workspace exists]"
  workspace_metadata:
    plan_workspace: "[absolute active QRSPI plan/ticket directory]"
    implementation_workspace: "[absolute implementation workspace when known]"
    trunk_branch: "main"
    stack_bottom_branch: ""
    parent_branch: ""
    current_branch: ""
  policy:
    advance_mode: "guided"
    auto_mode: false
    enable_plan_reviews: true
    invalid_result_retry_limit: 1
  summary:
    plan_goal: "[overall goal]"
    stage_completed: "[specific work completed]"
    key_decisions: "[decisions, risks, follow-up, or why next step is safe]"
  artifact: "thoughts/..."
  artifacts:
    - role: "related"
      path: "thoughts/..."
  next:
    steps:
      - action: "read_skill"
        param: ".pi/skills/qrspi-planning/SKILL.md"
      - action: "read_skill"
        param: ".pi/skills/[concrete next-stage]/SKILL.md"
      - action: "read_artifact"
        param: "thoughts/..."
      - action: "start_stage"
        param: "[concrete next-stage]"
```

`status` is lifecycle. `outcome` selects the graph branch. After `/q-workspace`, omit top-level `workspace` and keep both `plan_workspace` and `implementation_workspace` inside `workspace_metadata`. `workspace_metadata` records workspace identity plus branch context for humans and runtime handoff/debugging: `trunk_branch` is usually `main`; `stack_bottom_branch` is the lowest Graphite branch above trunk; `parent_branch` is the branch immediately below the chunk of work just completed; `current_branch` is the branch created/updated for the chunk. Use empty elements when not in a Graphite repo or the value is unknowable. `next.steps` is an ordered instruction block containing only `step` children. Complete results must include `outcome`. Review stages must use explicit node IDs (`review-outline`, `review-plan`, or `review-implementation`), never `review`.

You are resuming work within a QRSPI planning pipeline. A previous session created a handoff document with context about where things stand. Your job is to load that context and continue working.

## Process

### 1. Read the handoff

If a path was provided as an argument, read it. If not, ask the user for the path.

The handoff will be at:

```
[plan_dir]/handoffs/YYYY-MM-DD_HH-MM-SS_[stage]-handoff.md
```

### 2. Load context

Read `.pi/skills/qrspi-planning/SKILL.md` (pipeline overview), then load artifacts based on the stage you're resuming:

| Exact graph node | Read focused skill, then load these artifacts |
|-------|---------------------|
| `question` | `q-question`; `[plan_dir]/AGENTS.md`, existing `questions/*.md`, relevant `context/question/*.md`, `prds/*` |
| `research` | `q-research`; relevant `questions/*.md`, `context/research/*.md` |
| `design` | `q-design`; `AGENTS.md`, `questions/*.md`, `research/*.md`, `adrs/*.md`, `prds/*`, relevant `context/{research,design}/*.md` |
| `outline` | `q-outline`; `AGENTS.md`, `design.md`, optional `design-product.md`, `research/*.md`, `prds/*`, relevant `context/{design,design-product,outline}/*.md` |
| `review-outline` | `q-review`; `AGENTS.md`, `design.md`, optional `design-product.md`, `outline.md`, the active outline review directory and relevant review context |
| `research-for-review-outline` | `q-research-for-review`; active outline-review `review.md`, `questions/*.md`, and relevant review `context/research/*.md` |
| `address-review-research-outline` | `q-address-review-research`; parent `design.md`, optional `design-product.md`, `outline.md`, active review `review.md`, questions, and follow-up research |
| `plan` | `q-plan`; `AGENTS.md`, `design.md`, optional `design-product.md`, `outline.md`, `research/*.md`, `prds/*`, relevant `context/{outline,plan}/*.md` |
| `review-plan` | `q-review`; `AGENTS.md`, `design.md`, optional `design-product.md`, `outline.md`, `plan.md`, the active plan review directory and relevant review context |
| `research-for-review-plan` | `q-research-for-review`; active plan-review `review.md`, `questions/*.md`, and relevant review `context/research/*.md` |
| `address-review-research-plan` | `q-address-review-research`; parent `design.md`, optional `design-product.md`, `outline.md`, `plan.md`, active review `review.md`, questions, and follow-up research |
| `workspace` | `q-workspace`; reviewed `plan.md`, latest plan review, workspace metadata, and plan `AGENTS.md` |
| `implement` | `q-implement`; `AGENTS.md`, `design.md`, optional `design-product.md`, `outline.md`, `plan.md`, relevant research/PRDs and latest `context/implement/*.md` |
| `review-implementation` | `q-review`; `AGENTS.md`, design/outline/plan, final implementation handoff, active implementation review directory, and implementation workspace metadata |
| `verify` | `q-verify`; implementation handoff, canonical implementation `review.md`, project verification guide, and implementation workspace metadata |

Do not collapse exact review/helper IDs to generic `review`, and never emit synthetic stage `resume`. Implement, implementation-review, and verify resumes stay in the original implementation workspace.

### 3. Continue working

Based on the handoff's **Status** and **Next** sections, continue where the previous session left off.

**Implementation-stage rule:** when resuming an `implement` handoff, stay inside the handoff-driven loop. Complete at most one planned work chunk, then create the next implement handoff via `/q-handoff` before stopping. For tracked-edit Graphite work, implement and verify on the current top branch, run `gt create`/`gt modify`, then write the handoff on that new/current branch and amend it into the same commit so the handoff records final branch metadata. Handoff content should use `Done: ... ([finished]/[total])` and `Next: ...`, not reference slice number. The progress suffix belongs on `Done:` only and is computed from completed implementation slices/checkpoints in `plan.md` after the handoff. During implementation, the canonical continuation path is always the newly created handoff document, so successful implement responses should point to `/q-resume [new handoff path]` until the final work hands off to `/q-review`.

**Workspace rule for implementation resumes:** never resume implementation in a `git worktree`. Use the fresh filesystem copy created/repaired by `/q-workspace` and named for the plan directory or ticket slug. If the handoff does not identify an existing implementation workspace, stop and ask the user to run `/q-workspace [plan.md]`; do not create an ad-hoc copy from `/q-resume`. Run `git status --short` in that directory before branch or code changes. The workspace is the isolation boundary; branch creation is repo-specific, not automatic.

- If `status: in_progress` - continue the current stage from where it left off. You are working on the `[stage]` stage.
  - For `stage: implement`, read and follow `.pi/skills/q-implement/SKILL.md` before editing. `q-implement` owns implementation branch creation, Graphite commit timing, Conventional Commit subject format, and the QRSPI YAML result block requirements.
  - Apply the repository submission model from `q-implement`, `[plan_dir]/AGENTS.md`, and the handoff. In `cn-agents`, use the recorded fresh workspace plus Graphite slice branches and leave final integration to `/cn-agents-merge`.
  - If the next unchecked implementation slice is verification-only (`Files: no additional source files expected`, final validation, grep/build-only, or no planned edits), follow `q-implement`'s verification-only/no-branch rule.
- If `status: complete` and `next_stage` is set, start the corresponding QRSPI skill (for example, `design` → `/q-design`, `review` → `/q-review`). Product-design review is not a valid QRSPI `next_stage`; it runs only by explicit human invocation. For `review`, prefer passing the exact implement handoff path you just read. For other stages, pass the `plan_dir` from the handoff frontmatter.
- If `status: complete` and `next_stage` is null - the pipeline is complete. Tell the user.

Apply any **Learnings**, **User Decisions**, and referenced **Context Artifacts** from the handoff as you work.

Do not present an analysis or ask for confirmation. Just continue working.

## Response Format

When resuming work produces a user-facing QRSPI completion response, emit the fenced `yaml` `qrspi_result` footer followed by the mandatory concise human summary described above. Do not duplicate artifact lists or next command in prose; encode the primary artifact, comprehensive YAML summary, workspace paths, workspace branch metadata, and next command in YAML. After `/q-workspace`, omit top-level `workspace` and include `plan_workspace` plus `implementation_workspace` as the first children of `workspace_metadata`; for implementation resumes after Graphite branch creation, populate `trunk_branch`, `stack_bottom_branch`, `parent_branch`, and `current_branch` from the post-commit stack, and otherwise use empty elements for unknown values.

For `implement` resumes, `artifact` should normally be the newly created handoff file, not just `plan.md`, because implementation always checkpoints via handoff after each verified slice.

If the handoff indicates the next stage should begin immediately, continue directly rather than stopping to explain the handoff.

When a parent/orchestrator delegates that continuation to a Pi background process, include the complete fenced `qrspi_result` YAML from the previous stage/handoff verbatim in the next prompt. Do not pass only paths or prose summaries; downstream stages need prior `workspace_metadata`, `policy`, `artifact(s)`, and `next.steps` to preserve graph state across isolated Pi sessions.

During implementation, prefer `/q-resume [new handoff path]` as the next command after each non-final slice. Use `/q-review [handoff path]` only for the final implementation completion handoff.
