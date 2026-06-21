---
name: q-handoff
description: Create a handoff document to carry context forward within a QRSPI planning pipeline. Use "continue" arg to advance to the next stage; implement completion hands off to `/q-review` only after all slices are complete.
---

# Create Pipeline Handoff

> **Pipeline overview:** `.pi/skills/qrspi-planning/SKILL.md`

## Runtime YAML contract

Every response that completes, checkpoints, blocks, or hands off a QRSPI workflow node must include a fenced `yaml` block with top-level `qrspi_result`, followed by exactly one concise natural-language summary line or 1-3 short bullets. Do not use prose-only `Artifact` / `Summary` / `Next` completion responses.

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

`status` is lifecycle. `outcome` selects the graph branch. Before `/q-workspace`, top-level `workspace` is the absolute active QRSPI plan/ticket directory. After `/q-workspace`, omit top-level `workspace` and record both `plan_workspace` and `implementation_workspace` inside `workspace_metadata`. `workspace_metadata` records workspace identity plus branch context for humans and runtime handoff/debugging: `trunk_branch` is usually `main`; `stack_bottom_branch` is the lowest Graphite branch above trunk; `parent_branch` is the branch immediately below the chunk of work just completed; `current_branch` is the branch created/updated for the chunk. Use empty elements when a value is unknowable. For resume handoffs, `next.steps` is an ordered instruction block: read `qrspi-planning`, read `q-resume`, read `design.md`, read `outline.md`, read `plan.md`, read the handoff, then start `/q-resume` immediately unless blocked. Runtime transitions remain graph-authoritative and may validate/rewrite the steps. Complete results must include `outcome`. Review stages must use explicit node IDs (`review-outline`, `review-plan`, or `review-implementation`), never `review`.

Every `/q-handoff` session starts by reading `.pi/skills/qrspi-planning/SKILL.md`, then this skill, then immediately writing the handoff. A later continuation session must read `qrspi-planning`, then the skill named by `next.steps`, then start that stage immediately unless a safety/human gate blocks.

You are creating a handoff document to preserve your working context within a QRSPI planning pipeline. This handoff will be used by a future session to continue working on the same stage, or to pick up at the next stage.

**Handoff mode is stop-work mode.** Once this skill is invoked, do not continue implementation, debugging, refactoring, verification fixes, or artifact edits in the current session. Reads and status/inspection commands are allowed only to gather accurate context for the handoff. The priority is to pass the important context, risks, current state, next-edit instructions, and durable lessons learned to the next agent so that agent can resume code changes in a fresh session and avoid repeating known mistakes.

## When Invoked

0. **Load context and write the handoff immediately:**
   - Read `.pi/skills/qrspi-planning/SKILL.md` (pipeline overview)
   - Read this `q-handoff` skill

## Arguments

- **(no argument)** — checkpoint the current stage as `in_progress`.
- **`continue`** — mark the current stage as `complete` and move to the next stage.

## Stage order

| # | Stage | Skill | Produces |
|---|-------|-------|----------|
| 1 | question | `/q-question` | `questions/*.md` |
| 2 | research | `/q-research` | `research/*.md` |
| 3 | design | `/q-design` | `design.md` |
| 4 | design-product | `/q-design-product` | `design-product.md` |
| 5 | outline | `/q-outline` | `outline.md` |
| 6 | plan | `/q-plan` | `plan.md` |
| 7 | implement | `/q-implement` | code changes |
| 8 | review-implementation | `/q-review` | `reviews/.../review.md` |
| 9 | verify | `/q-verify` | `verify.md` |

`review` is the post-implementation handoff target, not a core planning stage. Only when `implement` is fully complete should `continue` create a review-ready handoff and point to `/q-review`. Intermediate implementation checkpoints must stay on `/q-resume`. `/q-review` writes the canonical review artifact to `[plan_dir]/reviews/`; clean review routes to `/q-verify` before the final human implementation gate.

**Human handoff wording hard rule:** the lead engineer does not care about slice numbers. In all human-facing handoff prose, never write `slice 1`, `slice 2`, `next slice`, or similar. Use exactly `Done:` and `Next:`. Describe behavior/files/outcome only. If the plan checkbox has a slice number, translate it into what changed and what remains.

Implementation handoffs must not create branches. If the next implementation work is verification-only (`Files: no additional source files expected`, final validation, grep/build-only, or no planned edits), record that it should run in the current implementation workspace/current top branch instead of creating a placeholder branch. GitHub/Graphite cannot create PRs for empty branches.

Implementation handoffs must preserve the fresh-directory rule, but keep it terse. Record one line only: `Workspace: [abs path]; Branch: [branch@commit]`. If unknown, write `Workspace: unknown; run /q-workspace first.` Never use `git worktree`.

Implementation handoffs must record the repository submission model and current state, not redefine implementation policy. For `cn-agents`, write explicitly: implementation uses a fresh workspace plus Graphite branches; record current branch/commit, `Done: ...`, `Next: ...`, and final integration via `/cn-agents-merge`. For other Graphite repos, record current branch/commit plus `Done: ...` and `Next: ...`. `/q-resume` loads `q-implement` for exact branch and commit rules.

Implementation handoffs should be created only after the implementation agent has run the completed work's `just check ...` command for the changed Go/templ files so formatting/lint issues are cleaned up before handoff. This lint cleanup requirement applies only to the implementation phase; do not run implementation lint/build cleanup during question, research, design, outline, or plan handoffs.

Implementation handoffs must record branch/commit state, not define commit-message policy. For implementation stages, point the next agent back to `/q-resume [handoff]`; `/q-resume` loads `q-implement` for the current branch and commit rules. For tracked-edit Graphite work, the implementation agent should create/modify the branch first, then write the handoff on that branch so it can record final `current_branch`, `parent_branch`, `stack_bottom_branch`, and `trunk_branch`; the handoff is then staged and amended into the same commit with `gt modify --no-interactive`. Record any commit command already run, current branch, commit hash, `Done: ...`, and `Next: ...`. Do not amend solely to chase the final self-referential commit hash inside the handoff; the final branch-head hash is reported in the QRSPI YAML result block.

## When to use

- Before context reset mid-stage (no argument)
- At end of a stage before handing off (`continue`)
- Any time you want a checkpoint

## Process

### 0. Stop editing and inspect only

- Stop all implementation work immediately.
- Do **not** edit code, tests, generated files, stage artifacts, or plans while creating the handoff.
- Do **not** fix lint/test failures discovered during handoff creation; record them clearly for the next agent.
- For implementation-stage handoffs, if the completed work's `just check ...` verification/lint cleanup was not run before entering handoff mode, record that gap clearly instead of starting new fixes during handoff creation. This does not apply to planning-stage handoffs.
- Use read-only inspection (`git status`, `git diff`, `git log`, file reads, test output already available) to understand what needs to be handed off.
- The only required write is the handoff document itself. Update `[plan_dir]/AGENTS.md` only when a durable gotcha is critical for future sessions and cannot be safely captured in the handoff alone.

### 1. Gather metadata

Run `~/dotfiles/spec_metadata.sh` and use it as the source of truth for the handoff filename timestamp and frontmatter fields (`date`, `researcher`, `branch`, `repository`). For `git_commit`, use the current hash for read-only/planning handoffs or already-committed implementation checkpoints. For tracked-edit implementation handoffs, run this after `gt create`/`gt modify` so `branch` is the current implementation branch. If the handoff will be amended into the current commit, use the current pre-handoff branch hash and note that the branch is amended after handoff creation; do not try to embed the final hash of the same commit inside itself.

For implementation-stage QRSPI YAML, also gather workspace branch metadata after any `gt create` / `gt modify` is complete:

- `current_branch`: `git branch --show-current`
- `parent_branch`: branch immediately below the just-created/current implementation branch in `gt log short` (or `gt parent` when it reports the stack parent)
- `stack_bottom_branch`: lowest Graphite branch above trunk in `gt log short`
- `trunk_branch`: trunk branch from the bottom of `gt log short`, usually `main`

If Graphite is unavailable or this is a verification-only/no-branch checkpoint, preserve the current branch and use empty elements for unknown Graphite-only values.

### 2. Identify the plan directory

Use the current plan directory:

```
thoughts/[git_username]/plans/[timestamp]_[plan-name]/
```

If unknown, ask the user.

### 3. Determine mode

- `continue`: set `status: complete`, compute `next_stage`
  - For `question` through `plan`, point the user to `/q-resume` so the next QRSPI stage can begin immediately. The next session must read `qrspi-planning`, read the next stage skill from the resumed graph state, and start that stage; do not say “ready to proceed.”
  - For `implement`, use this only when all implementation slices are complete; then set `next_stage: review` and point the user directly to `/q-review`, which should start immediately.
- checkpoint: set handoff document frontmatter `status: in_progress`.
  - For QRSPI result YAML during non-final implementation checkpoints, use runtime `status: handoff` (not `blocked`, not `complete`, no `outcome`). This is valid for the `implement` node and keeps the workflow resumable without marking it blocked/error.
  - Use this for any non-final implementation checkpoint so the next step remains `/q-resume`.

### 4. Capture key learnings and refresh long-term memory only if essential

Every handoff must include a footer section named `## Key Learnings and Notes to Future Agents`. Before writing, deliberately identify anything from this session that future agents should know when working in this project or with QRSPI in general:

- what went wrong, broke, surprised you, or cost time
- project-specific gotchas, invariants, command quirks, or workflow traps
- QRSPI process learnings, YAML/routing issues, handoff/resume pitfalls, or review/verify lessons
- decisions or rejected paths that are easy to accidentally revisit
- verification gaps, flaky checks, environment constraints, or manual-test caveats

Prefer recording new context in the handoff footer. Update `[plan_dir]/AGENTS.md` only if this stage uncovered durable context that future agents should remember before reading any handoff, such as:

- approved decisions or scope boundaries
- important tradeoffs or rejected paths
- non-obvious invariants, gotchas, or review learnings

Keep both curated. Do **not** dump transient notes or duplicate the full artifact. If there are no meaningful learnings, write `None.` under the footer rather than omitting it.

### 5. Write the handoff

Create:

```
[plan_dir]/handoffs/YYYY-MM-DD_HH-MM-SS_[stage]-handoff.md
```

Use this template:

```markdown
---
date: [ISO datetime with timezone]
researcher: [git_username]
last_updated_by: [git_username]
git_commit: [current commit hash; for implementation handoffs amended into the current commit, use the pre-handoff branch hash and note the final hash is reported in YAML]
branch: [current branch]
repository: [repository name]
stage: [question|research|design|design-product|outline|plan|implement]
ticket: "[ticket reference if any]"
plan_dir: "thoughts/[git_username]/plans/[timestamp]_[plan-name]"
status: [in_progress|complete]
next_stage: [next stage name, `review`, or null if in_progress or pipeline complete]
---

# [Stage] Handoff

## Status
[For implementation handoffs, use exactly this concise shape. No slice numbers in prose.
Done: [completed behavior/files/outcome] ([finished]/[total])
Next: [next behavior/files/outcome; say `verify-only/no branch` only when true]
State repo model only when needed: `cn-agents` workspace Graphite + `/cn-agents-merge`, or other Graphite stack.]

## Workspace
[Implementation only, one line: `Workspace: [abs path]; Branch: [branch@commit]`. If workspace unknown: `Workspace: unknown; run /q-workspace first.` Omit for non-implementation handoffs.]

## Learnings
[Important findings not fully captured in artifacts, with file:line references where relevant.]

## User Decisions
[User approvals/rejections/changes that matter downstream.]

## Context Artifacts
[Exact paths to relevant context files under `[plan_dir]/context/` that the next session should load first.]

## Verification
[Relevant verification evidence when known, or `not run` with a short reason. For implementation handoffs, include the `just check ...` command run for changed Go/templ files, plus any focused tests. Planning-stage handoffs do not need implementation lint/build cleanup.]

## Next
[For implementation handoffs, keep this short. No slice numbers in prose.
Resume: `/q-resume [this handoff]`
Done: [completed work] ([finished]/[total])
Next: [next work; say `verify-only/no branch` only when true]
Branch: [branch@commit]
For implement-complete handoffs: `Review: [area]. Verified: [commands].` Include final progress suffix, e.g. `(5/5).`]

## Key Learnings and Notes to Future Agents
[Footer. Required for every handoff. Capture anything learned or anything that went wrong that future agents should know for this project or QRSPI generally. Include concise bullets with file:line references, commands, exact symptoms, and corrected behavior when useful. If none, write `None.`]
```

### 6. Sync

For non-implementation stages, run `just sync-thoughts` from the normal source checkout.

For implementation handoffs from a fresh copied repo, first check the current branch:

````bash
git branch --show-current
git status --short
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
```yaml
qrspi_result:
  stage: "[canonical node id]"
  status: "complete"
  outcome: "complete"
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
      - action: "read_artifact"
        param: "thoughts/..."
      - action: "start_stage"
        param: "[concrete next-stage]"
````

Line-quality requirements still apply inside `summary`:

- Before `/q-workspace`, top-level `workspace` must be present and point at the absolute active plan/ticket directory.
- After `/q-workspace`, top-level `workspace` must be omitted; `workspace_metadata` must include both `plan_workspace` and `implementation_workspace` before branch metadata.
- `workspace_metadata` must be present. For implementation handoffs after Graphite branch creation, fill `trunk_branch`, `stack_bottom_branch`, `parent_branch`, and `current_branch`; for non-Graphite/planning contexts, include empty elements except `current_branch` when known.
- `stage_completed` must describe the actual work, not generic `stage complete`.
- `key_decisions` must include verification evidence when known, or say why verification was not run.
- Never abbreviate artifact paths.

Next routing:

- For `continue` mode from `implement`, use `status: complete`, `outcome: complete`, `/q-review` in YAML `next.steps`, and make `summary.key_decisions` say `Next stage should start immediately: /q-review ...`.
- For non-final implementation checkpoints, use `status: handoff`, omit `outcome`, use `/q-resume` in YAML `next.steps`, and make `summary.key_decisions` say the next session should read `qrspi-planning`, then `/q-resume`, then exact `design.md`, exact `outline.md`, exact `plan.md`, exact handoff path, then resume implementation in the same workspace.
- For all other handoffs, use `/q-resume` in YAML `next.steps` and make `summary.key_decisions` say the next session should read `qrspi-planning`, then `/q-resume`, then exact `design.md`, exact `outline.md`, exact `plan.md`, exact handoff path, then start the resumed stage immediately.

Never abbreviate paths.
