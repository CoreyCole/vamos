---
name: q-plan
description: Expand the structured outline into a detailed implementation plan from approved `design.md`, optional `design-product.md`, and `outline.md` — tactical doc for the coding agent. Sixth stage of QRSPI pipeline. Not human-reviewed, but followed by LLM planning review via `/q-review [plan.md]`.
---

## QRSPI mode contract

QRSPI has a canonical advancement mode plus separate review/retry policy:

- `advanceMode=discuss`: do not advance after valid YAML. Keep chatting in the current session; show the validated next action and explicit continue/start button. Not default.
- `advanceMode=guided`: default. Auto-continue graph-safe non-human edges; stop at explicit human gates. Current `auto_mode=false` behavior.
- `advanceMode=autopilot`: auto-continue graph-safe non-human edges and auto-approve only human gates marked auto-approvable. Current `auto_mode=true` behavior.
- Legacy compatibility: until runtime persists `advanceMode`, map `auto_mode=false` to `guided` and `auto_mode=true` to `autopilot`. `discuss` needs a distinct runtime policy value.
- All modes still stop on `needs_human`, `blocked`, `error`, invalid artifact, disallowed transition, run failure, YAML retry exhaustion, or explicit safety gate.
- `enable_plan_reviews=true`: run planning `/q-review` after outline and plan. Do not run `/q-review` immediately after design; design advances to `/q-outline` (or optional `/q-design-product`).
- `enable_plan_reviews=false`: skip planning `/q-review`; final implementation `/q-review` always runs.
- Research never has its own human stop. Humans evaluate research in design/outline review.
- Emit the QRSPI YAML result as a fenced `yaml` block with top-level `qrspi_result` code block for every completed QRSPI stage result so it is syntax highlighted, then add only the mandatory concise human summary after it.

## QRSPI YAML summary contract

The ``summary`` element is used by humans to understand workflow state before asking follow-up questions or advancing. It must be structured, specific, self-contained, not a generic completion label. Use these child elements inside ``summary``:

- ``plan_goal``: overall plan/workflow goal in plain language; not just current stage label.
- ``stage_completed``: what this stage/session did and how it moves toward the goal. Extremely concise; sacrifice grammar for concision.
- ``key_decisions``: direction we are headed; significant tradeoffs, risks, open questions, follow-up, or why next step is safe. Use `None.` only when truly none.

Keep each child element short: 1-2 concise lines max.

For review stages, always include both: (1) what the entire implementation/plan now does as a whole, and (2) what this review session checked and changed. Do not write vague summaries like `review complete`, `implementation review result`, `done`, or `summary of findings` without the concrete details a human would need to ask informed questions.

## Immediate start

Every `/q-plan` session starts by reading `.pi/skills/qrspi-planning/SKILL.md`, then this skill, then loading the required artifacts. Do not answer “ready to proceed.” Read the relevant code files and write `plan.md` in the same session. Human approval for design/outline direction happens in `/q-outline`, before `outline.md` is written; do not add another approval prompt in `/q-plan`.

## QRSPI footer instructions

When more than one artifact is relevant, keep ``artifact`` as the primary next-command artifact and also include ``artifacts`` with every important artifact path, including review records, done summaries, handoffs, ADRs, and follow-up questions.

Do not duplicate artifact lists or machine-control details in prose outside the YAML result result. For normal QRSPI stage completion, the response must be the fenced `yaml` ``qrspi_result`` block followed by a mandatory concise human summary; make both summaries specific enough for humans.

Post-YAML natural summary style: caveman clear. Few words. Most important words only. For `plan`, summarize implementation plan and how each ADR is reflected.

If `enable_plan_reviews=true`, ``next.steps`` steps route to `/q-review [plan.md]`; if false, ``next.steps`` steps route to `/q-workspace [plan.md]`. For stage completion, emit a fenced `yaml` QRSPI footer followed by the mandatory concise human summary; do not duplicate Artifact/Next machine-control details in prose:

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

# Plan — The Implementation

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

`status` is lifecycle. `outcome` selects the graph branch. Optional ``project`` and `related_projects` carry primary/related project participation metadata only; they do not change singular workspace execution rules. ``next.steps`` is an ordered instruction block for the next agent: read `qrspi-planning`, read the next stage skill, read the appropriate artifact, then start the next stage immediately unless a named human/safety gate blocks. Runtime transitions remain graph-authoritative and may validate/rewrite the steps. Complete results must include ``outcome``. Review stages must use explicit node IDs (`review-outline`, `review-plan`, or `review-implementation`), never `review`.

## Project participation metadata

For cross-project plans, preserve machine-readable frontmatter and YAML project metadata:

- `project`: singular primary project owner.
- `related_projects`: zero/many supporting project IDs.
- ``project`` in ``qrspi_result`` mirrors frontmatter `project`.
- `related_projects` mirrors frontmatter `related_projects`.
- Related projects are plan participation metadata only. They do not imply multiple execution cwd values.
- `workspace_metadata.plan_workspace` and `workspace_metadata.implementation_workspace` remain singular.
- Plan frontmatter must preserve `project` and `related_projects`; implementation plans must state how related-project work is handled until multi-project workspaces are first-class.

You are the sixth stage of the QRSPI pipeline. You expand the structured outline into a detailed, tactical implementation plan. This is a machine document — instructions for the coding agent. Human alignment happened in question, design, and outline; product design may also exist for product-critical or high-stakes work. After this file is written, it gets an LLM planning review via `/q-review [plan.md]` before implementation starts.

## When Invoked

0. **Load context and begin immediately:**
   - Read `.pi/skills/qrspi-planning/SKILL.md` (pipeline overview)
   - Read this `q-plan` skill
   - Read `[plan_dir]/AGENTS.md`
   - Read all files in `[plan_dir]/questions/`
   - Read `[plan_dir]/design.md`
   - Read `[plan_dir]/design-product.md` if present
   - Read `[plan_dir]/outline.md`
   - Read all files in `[plan_dir]/research/`
   - Read all files in `[plan_dir]/context/research/`
   - Read all files in `[plan_dir]/context/design/`
   - Read all files in `[plan_dir]/context/design-product/` if any
   - Read all files in `[plan_dir]/context/outline/`
   - Read all files in `[plan_dir]/context/plan/` if any
   - Read all files in `[plan_dir]/prds/`
1. **If a plan directory path or outline doc path was provided**, resolve the plan directory from it, load the artifacts above, then begin.
1. **If no parameters**, respond:

```
I'll expand your outline into a detailed implementation plan.

Please provide the plan directory path or outline doc path:
e.g. `/q-plan thoughts/[git_username]/plans/2026-03-29_12-26-32_feature-name`
or `/q-plan thoughts/[git_username]/plans/2026-03-29_12-26-32_feature-name/outline.md`
```

Then wait for input.

## Process

1. **Verify artifacts are loaded** from step 0: `[plan_dir]/AGENTS.md`, all `questions/*.md`, `design.md`, optional `design-product.md`, `outline.md`, all `research/*.md`, relevant context artifacts in `context/research/`, `context/design/`, `context/design-product/` when present, `context/outline/`, and `context/plan/`, and any relevant files in `prds/`.

   - Missing `design-product.md` is not a blocker for internal tools, bugfixes, refactors, or other low product-risk work.

   - Stop if `design-product.md` exists and has verdict `Blocked`, unless the user explicitly accepts the blocker/override.

   - If the task is product-critical, high-stakes, user-facing with unclear PRD coverage, compliance/security sensitive, or changes irreversible user/data behavior, stop and ask whether to run `/q-design-product` before planning.

1. **Read key files from the codebase** that the outline references — you need to see the actual code to write accurate implementation steps.

   - If the plan will edit Go files, read relevant codebase rules before writing implementation steps: `.agents/rules/go-style.md` when present and package-local `AGENTS.md`/`CLAUDE.md`. Make the plan use shared helpers (`pkg/pointers.To`, `pkg/collections.Set`, nullable `Ptr()`, `pkg/checked`) instead of hand-rolled equivalents.
   - If the current file graph, entry points, or nearby patterns are still unclear, run `codebase-locator` and, if needed, `codebase-analyzer`, then write timestamped artifact(s) under `[plan_dir]/context/plan/` before finalizing the plan.

1. **Expand each slice** from the outline into detailed implementation steps:

   - Full file paths for every change
   - Actual code to write (function bodies, not just signatures)
   - Test code
   - Commands to verify each slice
   - A dictated commit message for the slice, using the exact format below

   Each non-verification slice must include a `### Commit Message` section. Verification-only/no-change slices must say `No commit: verification-only slice.` The commit subject must follow Conventional Commits 1.0.0: ``type`[optional scope]: `description``. The subject description should still mention the implementation workspace/slice when useful, but the required machine-readable slice metadata belongs in the QRSPI YAML result block. The commit footer must include YAML wrapped in ``qrspi_commit`` with the workspace name, slice number/name, and paths to `design.md`, `outline.md`, and `plan.md`.

   Also state the repository submission model in the implementation plan. For `cn-agents`, say explicitly that `/q-workspace` selects the workspace base, then `/q-implement` or `/q-resume` implements/verifies each tracked edit slice first and only then runs `gt create ..._slice-N` or `..._review_plan_slice-N` with the final conventional commit message. Graphite commits per slice, and `/cn-agents-merge` after implementation/review is complete. Do not say `cn-agents` commits slices directly to `main` or pre-creates the next slice branch before editing. For other Graphite repos, slices with tracked edits use the same end-of-slice Graphite branch creation rule. The workspace model is always used; branching is repo-specific.

   Use this exact commit-message shape:

   ```yaml
qrspi_commit:
  plan: "thoughts/..."
  stage: "implement"
  slice: "1"
  summary: "[commit summary]"
  artifacts:
    - "thoughts/.../plan.md"
```

1. **Add status checkboxes** at the top — these are the context recovery mechanism. When the implementing agent's context window resets, it reloads this file and the checkboxes tell it where to pick up.

1. **Add an `Implementation Workspace Prep` section before the first slice.** This section documents that `/q-workspace` creates or repairs the filesystem copy after the final `/q-review [plan.md]` succeeds.

   The section must include these invariants:

   - Implementation happens in a fresh filesystem copy, never a git worktree.
   - `/q-workspace`, not `/q-plan`, chooses the final workspace base after plan-review edits are complete.
   - The workspace directory should be based on the plan slug/timestamp, for example a sibling directory named `[repo-name]-[plan-timestamp]_[plan-slug]`.
   - For normal parent plans, `/q-workspace` usually bases the copy on latest `origin/main` (or project trunk).
   - For implementation-review follow-up/review-fixes plans under `[parent_plan_dir]/reviews/*_implementation-review/`, `/q-workspace` must prove whether the parent implementation stack top is already merged into `origin/main`. If yes, base on latest `origin/main`. If no, base on the parent implementation stack top branch/commit so review fixes stack on top and no work is lost.
   - In Graphite repos, including `cn-agents`, when the parent stack is unmerged the first review-fix slice branch created later by `/q-implement` must have `gt parent` equal to the parent implementation stack top branch.
   - The workspace copy is the isolation boundary. For `cn-agents`, use Graphite slice branches during `/q-implement` for each slice with planned tracked edits; merge the finished stack back with `/cn-agents-merge`.
   - The full `[plan_dir]` contents must be present inside the workspace at the same relative `thoughts/...` path so `/q-implement [plan.md]` can load the plan, reviews, AGENTS.md memory, ADRs, questions, research, and handoffs.
   - If an existing workspace directory is present and dirty, stop and ask before replacing it. Move it aside only with an explicit backup name and only after confirming that is desired.

   Include a concise command template that points to `/q-workspace` rather than attempting to finish workspace creation inside `/q-plan`.

1. **If the plan locks in durable sequencing changes, invariants, or implementation caveats that future implementers/reviewers should remember first, update `[plan_dir]/AGENTS.md`.**

   - Keep it short and curated.
   - Point back to `plan.md`, `outline.md`, or code paths instead of copying the whole plan.

1. **Immediately before writing or updating `plan.md`, gather metadata** with `~/dotfiles/spec_metadata.sh` and use it to populate the frontmatter fields.

1. **Write the plan** directly. No human review step — alignment already happened in design and outline. The next gate is LLM planning review via `/q-review [plan.md]`.

1. **Run `just sync-thoughts` in the planning/source checkout** after writing `plan.md`. If it fails, stop with status `blocked` or `error`; do not advance to review with unsynced planning artifacts.

1. **Do not create the implementation workspace in `/q-plan`.** Workspace creation/repair is the separate `/q-workspace` stage after final `/q-review [plan.md]`, because review edits can change the plan and review-fixes plans may need to stack on an unmerged parent implementation branch.

## Output Template

Write to `[plan_dir]/plan.md`:

````markdown
---
date: [ISO datetime with timezone]
researcher: [git_username]
last_updated_by: [git_username]
git_commit: [current commit hash]
branch: [current branch]
repository: [repository name]
stage: plan
ticket: "[ticket reference if any]"
plan_dir: "thoughts/[git_username]/plans/[timestamp]_[plan-name]"
project: "[primary project id if known]"
related_projects: []
---

# Implementation Plan: [Feature Name]

## Status
- [ ] Slice 1: [Name]
- [ ] Slice 2: [Name]
- [ ] Slice 3: [Name]
...

## Implementation Workspace Prep

`/q-workspace` will create or repair the fresh filesystem copy for `/q-implement` after `/q-review [plan.md]` succeeds.

Planned workspace path:

```text
[absolute workspace path candidate]
```

Workspace base selection is deferred to `/q-workspace` so it can account for plan-review edits and unmerged parent stacks. For normal plans, the base is usually latest `origin/main`. For review-fixes plans under `reviews/*_implementation-review/`, `/q-workspace` must prove whether the parent implementation stack top is merged into `origin/main`; if not, it must base the workspace on that parent top branch/commit and record the expected `gt parent` for review-fix slice branches.

Do not use `git worktree`. This workspace is a normal copied directory created with efficient filesystem clone/reflink copy (`cp -ac` on macOS, `cp -a --reflink=auto` on Linux). If the workspace directory is dirty or missing when implementation starts, stop and ask before moving/replacing it.

Repository submission model: for `cn-agents`, implement in the workspace selected by `/q-workspace`, then create a Graphite branch for each tracked edit slice at the end of that slice (`gt create ..._slice-N` or `..._review_plan_slice-N`) after implementation and verification. Commit slices with Conventional Commit messages plus QRSPI YAML result blocks, and run `/cn-agents-merge` after implementation/review is complete. Do not commit QRSPI implementation slices directly to `main` and do not pre-create future slice branches.

## Slice 1: [Name]

### Files

- `path/to/file.ext` (new)
- `path/to/other.ext` (modify)

### Changes

**`path/to/file.ext`** (new):
[Full implementation code]

**`path/to/other.ext`** (modify):
[Specific changes with enough surrounding context to locate the edit]

### Tests

**`path/to/test_file.ext`**:
[Test code]

### Verify

[Command to run to confirm this slice works]

### Commit Message

```yaml
qrspi_commit:
  plan: "thoughts/..."
  stage: "implement"
  slice: "1"
  summary: "[commit summary]"
  artifacts:
    - "thoughts/.../plan.md"
```

______________________________________________________________________

## Slice 2: [Name]

...

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
````
