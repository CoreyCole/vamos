---
name: qrspi-planning
description: Ticket-level QRSPI pipeline — Question, Research, Design, Outline, Plan, Workspace, Implement, Review, Verify. Use for concrete tickets; use qrspi-project-planning for projects or milestones.
---

## Runtime YAML contract

Every response that completes a QRSPI workflow node must include a fenced `yaml` block with top-level `qrspi_result`, followed by a mandatory concise human summary. Do not use prose-only `Artifact` / `Summary` / `Next` completion responses.

The user should not have to ask for this YAML. Return it automatically whenever QRSPI stage work completes, hands off, blocks, or errors. When a user says “the correct response”, “now the response”, “what’s the response?”, “give me the result”, or asks for the QRSPI response/result after QRSPI stage work, they mean the YAML contract below plus the required post-YAML concise summary. Return the fenced `yaml` `qrspi_result` block first, not a prose recap in place of YAML. A handoff markdown file is only an artifact referenced from the YAML result; creating a handoff does not replace the required YAML response.

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

Summary: [Ultra-concise human update. Sacrifice perfect grammar for concision.]

`status` is lifecycle. `outcome` selects the graph branch. `project` is the singular primary project owner and mirrors frontmatter `project`. `related_projects` mirrors frontmatter `related_projects`; related projects are plan participation metadata only and do not imply multiple execution cwd values. Before `/q-workspace`, include top-level `workspace` immediately after project participation metadata and set it to the absolute active QRSPI plan/ticket directory where the next planning stage should run. Once `/q-workspace` creates or repairs an implementation workspace, omit top-level `workspace` and instead record both paths inside `workspace_metadata` as `plan_workspace` and `implementation_workspace`. `workspace_metadata` records workspace identity plus branch context for humans and runtime handoff/debugging: `plan_workspace` is the plan/ticket directory, `implementation_workspace` is the fresh implementation workspace after `/q-workspace`, `trunk_branch` is usually `main`, `stack_bottom_branch` is the lowest Graphite branch above trunk, `parent_branch` is the branch immediately below the chunk of work just completed, and `current_branch` is the branch created/updated for the chunk. Use empty elements when a value is unknowable. `next.steps` is an explicit ordered instruction block for the next agent: read `qrspi-planning`, read the next stage skill, read the appropriate artifact, then start the next stage immediately unless a named human/safety gate blocks. Runtime transitions remain graph-authoritative and may validate/rewrite the steps. Complete results must include `outcome`. Review stages must use explicit node IDs (`review-outline`, `review-plan`, or `review-implementation`), never `review`.

## QRSPI mode contract

QRSPI has a canonical advancement mode plus separate review/retry policy. Use these names in UI, docs, and plans:

- `advanceMode=discuss`: do not advance the workflow after valid YAML. Keep the current chat open so the human and agent can ask follow-up questions, inspect the result, or revise context. The card should still show the validated next action and an explicit continue/start button. This is not the default.
- `advanceMode=guided`: default. Auto-continue graph-safe non-human edges; stop at explicit human gates. Current `auto_mode=false` behavior.
- `advanceMode=autopilot`: auto-continue graph-safe non-human edges and auto-approve only human gates marked auto-approvable. Current `auto_mode=true` behavior.
- When delegating QRSPI stages to Pi/background agents, every next-stage prompt must include the full fenced `qrspi_result` YAML from the immediately previous stage, verbatim. Do not summarize or cherry-pick the fields; downstream agents need the exact `workspace_metadata`, `policy`, `artifact`, `artifacts`, and `next.steps` context.
- If the user has asked for delegated/autonomous advancement, start graph-safe next stages in fresh Pi/background agents without asking for approval; pause only for explicit human-context blockers, safety/lost-work risks, invalid artifacts, or stage rules that require manual confirmation. See `references/delegated-pi-background-orchestration.md` for a prompt skeleton and implementation-loop pattern.
- Legacy YAML/runtime compatibility: until the runtime persists `advanceMode`, map `auto_mode=false` to `guided` and `auto_mode=true` to `autopilot`. `discuss` requires a distinct runtime policy value; do not pretend it is `auto_mode=false`.
- All advance modes still stop on `needs_human`, `blocked`, `error`, invalid artifact, disallowed transition, run failure, YAML retry exhaustion, or an explicit safety gate.
- `enable_plan_reviews=true`: run planning `/q-review` after outline and plan. Do not run `/q-review` immediately after design; design advances directly to `/q-outline`.
- `enable_plan_reviews=false`: skip planning `/q-review`; final implementation `/q-review` always runs.
- Research never has its own human stop. Humans evaluate research-derived direction in design/outline review, but research must loop to another `/q-research` pass when new code-answerable factual questions materially inform design.
- Emit the QRSPI YAML result as a fenced `yaml` block block with top-level `qrspi_result` for every completed QRSPI stage result so it is syntax highlighted, then add only the mandatory concise human summary after it.

## Immediate continuation contract

For the detailed Hermes → Pi background-process orchestration pattern, including carrying the prior full `qrspi_result` into the next prompt, see `references/background-pi-stage-delegation.md`.

Every ticket-level QRSPI stage session must start by reading this `qrspi-planning` skill, then reading the skill named by the active stage or by the prior result's `next.steps`, then immediately executing that stage. Do not answer “ready to proceed” when the graph has a valid next stage. Start the next stage now unless a documented human gate, `needs_human`, `blocked`, `error`, invalid artifact, safety check, or missing required input stops execution.

When orchestrating QRSPI from Hermes/background Pi processes for this user, be active: after a background stage finishes, inspect the full result log, parse the YAML handoff, and start the next graph-safe stage immediately before giving a prose update. Do not stop with “outline is next”, “I’ll start q-plan”, or “ready to proceed” when tools can start the next stage now. Intermediate implementation `handoff` results are recovery checkpoints, not human gates by themselves; if the user has asked for auto-advance and no safety/human stop applies, launch the next `/q-resume` from the handoff and then relay a concise Done/Next summary with the new process/session handle. Only pause for actual human input, design decisions/tradeoffs, mandatory approval gates, safety/lost-work checks, `needs_human`, `blocked`, `error`, invalid artifact, or failed run.

When delegating a QRSPI next stage to a Pi/background process, include the full fenced `qrspi_result` YAML from the immediately previous stage verbatim in the prompt, before the next-stage task details. Do this even when the artifact paths are also passed separately; the YAML carries outcome, policy, workspace metadata, and transition intent that later agents need to preserve.

One normal post-result pause exists after planning begins: before `/q-outline` writes `outline.md`, the `/q-outline` session first summarizes the key decisions from `design.md`, optional `design-product.md`, and ADRs, then asks the human to approve outline writing. If the human replies with approval such as `go`, `vamos`, `yes`, `approved`, or equivalent, the `/q-outline` session must write the outline in that same session; do not require a second nudge. If the user explicitly instructs the workflow to delegate/auto-advance stages and not request approval unless human context is genuinely needed, treat this outline alignment gate as pre-approved for that workflow and pass the override into the Pi/background prompt; still stop for `needs_human`, safety, blockers, or unresolved human-context decisions.

Other human alignment happens inside the active stage before it emits a complete result, or via explicit `needs_human`; a completed result should route onward immediately. `/q-plan` does not ask for another human approval; it reads relevant code files and writes `plan.md` immediately from the reviewed outline.

For all other complete results, including `review-plan` and `workspace`, `summary.key_decisions` must explicitly say that the next stage should start immediately and name it, e.g. `Next stage should start immediately: /q-workspace ...`. The `next.steps` block must spell out ordered steps for the next agent: read `qrspi-planning`, read the named stage skill, read each required artifact in its own step, read `design-product.md` when it exists and design context is needed, then start the next stage immediately. Do not use ambiguous “or” alternatives inside emitted YAML; choose the concrete next stage for the current outcome. The post-YAML human summary must not say “ready to proceed”; say `Next: start ... now.` when the next graph node should run immediately.

## QRSPI YAML summary contract

The `summary` element is used by humans to understand workflow state before asking follow-up questions or advancing. It must be structured, specific, self-contained, not a generic completion label. Use these child elements inside `summary`:

- `plan_goal`: overall plan/workflow goal in plain language; not just current stage label.
- `stage_completed`: what this stage/session did and how it moves toward the goal. Extremely concise; sacrifice grammar for concision.
- `key_decisions`: direction we are headed; significant tradeoffs, risks, open questions, follow-up, or why next step is safe. Use `None.` only when truly none.

Keep each child element short: 1-2 concise lines max.

For review stages, always include both: (1) what the entire implementation/plan now does as a whole, and (2) what this review session checked and changed. Do not write vague summaries like `review complete`, `implementation review result`, `done`, or `summary of findings` without the concrete details a human would need to ask informed questions.

## Post-YAML human summary contract

After every fenced `qrspi_result` block, add exactly one mandatory concise human summary line or short bullet list.

Style is strict: caveman clear. Few words. Most important words only. Sacrifice grammar hard for concision.

- Put it after the YAML result result result result, never before.
- Keep it shorter than the YAML `summary`; 1-3 short bullets or one `Summary:` sentence.
- Prefer fragments over sentences when clearer.
- Say only what human needs now.
- Do not restate full artifact lists, branch metadata, or machine-control details already encoded in YAML.

Stage-specific post-YAML summary content:

- `question`: one research question per line, as few words as possible. Caveman speak. Example:
  ```text
  Questions:
  - Auth path?
  - Data shape?
  - Failure modes?
  - Tests?
  ```
- `research`: key findings + direct answers, one answer per line when multiple. Example:
  ```text
  Findings:
  - Auth in middleware.
  - Data from X.
  - Risk Y.
  ```
- `design`: key direction only. Example: `Design: reuse X; add Y adapter; no schema change.`
- `outline`: as concise as possible; one line per slice/part. Example:
  ```text
  Outline:
  - Model: add X.
  - API: wire Y.
  - Tests: cover Z.
  ```
- `plan`: summarize implementation plan and how each ADR is reflected. Example: `Plan: 4 parts. ADR-001 => adapter seam. ADR-002 => no migration.`
- `review-outline`, `review-plan`, `review-implementation`: say what review found and fixed. For normal parent-plan `review-plan`, also include immediate workspace start. Example: `Found: stale API assumption. Fixed: plan uses current handler. Next: start /q-workspace now.` Clean normal plan review: `Found: clean. Next: start /q-workspace now.` For implementation-review follow-up `review-plan`, route directly to implementation: `Found: clean. Next: start /q-implement now.` For `review-outline`, route directly to planning: `Found: clean. Next: start /q-plan now.`

## QRSPI result footer

When more than one artifact is relevant, keep `artifact` as the primary next-command artifact and also include `artifacts` with every important artifact path, including review records, done summaries, handoffs, ADRs, and follow-up questions.

Do not duplicate artifact lists or machine-control details in prose outside the YAML result result result result. For normal QRSPI stage completion, the response must be the fenced `yaml` `qrspi_result` block followed by a mandatory concise human summary; make both summaries specific enough for humans.

Every primary QRSPI stage and review/helper that completes a workflow transition must include a visible fenced `yaml` QRSPI result block. Always include `outcome` for complete results. Before `/q-workspace`, include `workspace` immediately after `outcome`; after an implementation workspace exists, omit top-level `workspace` and include both workspace paths inside `workspace_metadata`:

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

Summary: Design captured approved direction; next /q-outline summarizes decisions for approval before writing outline.

Statuses: `complete`, `handoff`, `needs_human`, `blocked`, `done`, `error`.
Before `/q-workspace`, `workspace` appears immediately after `outcome` for complete results and points at the absolute active QRSPI plan/ticket directory. `/q-workspace` creates or repairs the fresh implementation workspace; from that result onward, do not emit top-level `workspace`. Instead, put `plan_workspace` and `implementation_workspace` as the first children of `workspace_metadata` and preserve both in later implementation, resume, review, and verify results. Non-complete results that omit `outcome` follow the same rule: use top-level `workspace` only before an implementation workspace exists; otherwise use metadata paths.
`workspace_metadata` always appears immediately after `workspace` before `/q-workspace`, or immediately after `outcome`/`status` after top-level `workspace` is omitted. For implementation results in Graphite repos, fill `trunk_branch`, `stack_bottom_branch`, `parent_branch`, and `current_branch` after `gt create`/`gt modify`; for planning/non-Graphite contexts, include empty elements for unknown values and preserve `current_branch` when known.
`next.steps` is an ordered instruction block for the next agent; runtime validates and may rewrite it from latest persisted policy before starting another run. Include only `step` children in execution order, ending with the immediate start instruction or the explicit outline-review approval prompt instruction. Do not include a separate `command` child; the final step names the next stage. Handoff/resume results must include separate read steps for `q-resume`, exact `design.md`, exact `outline.md`, exact `plan.md`, and exact handoff path before the start step.

## Project participation metadata

For cross-project plans, preserve machine-readable frontmatter and YAML project metadata:

- `project`: singular primary project owner.
- `related_projects`: zero/many supporting project IDs.
- `project` in `qrspi_result` mirrors frontmatter `project`.
- `related_projects` mirrors frontmatter `related_projects`.
- Related projects are plan participation metadata only. They do not imply multiple execution cwd values.
- `workspace_metadata.plan_workspace` and `workspace_metadata.implementation_workspace` remain singular until a workflow node explicitly supports plural execution workspaces.

## Project-planning handoff

This skill is for individual ticket work. For epics/projects, milestone planning, or planning tickets whose purpose is to create future tickets, start with `.pi/skills/qrspi-project-planning/SKILL.md`; it owns the project-planning skill landscape and routes into milestone-specific skills. Do not force normal `/q-question`, `/q-research`, `/q-design`, `/q-outline`, or `/q-plan` onto milestone meta-planning.

Milestone-specific skills:

- `.pi/skills/q-milestone-question/SKILL.md`
- `.pi/skills/q-milestone-research/SKILL.md`
- `.pi/skills/q-milestone-design/SKILL.md`
- `.pi/skills/q-milestone-create-tickets/SKILL.md`

Legacy milestone skills `/q-milestone-outline` and `/q-milestone-plan` are retired and are not migrated into Vamos for new milestone planning.

# QRSPI Planning Pipeline

A structured approach to non-trivial coding tasks. Each stage produces artifacts in a plan directory that grows over time. Separate context windows keep each stage focused and avoid the dumb zone.

## Stages

| # | Stage | Skill | Produces | Gate |
|---|-------|-------|----------|------|
| 1 | Question | `/q-question` | `questions/*.md` | Human alignment on goals, scope, tradeoffs, and research agenda; YAML summary includes the research questions |
| 2 | Research | `/q-research` | `research/*.md` | Answer open factual questions before design; loop to more research if new code-answerable design facts are needed |
| 3 | Design | `/q-design` | `design.md` + `adrs/*.md` | Human approves technical direction |
| 4 | Outline | `/q-outline` | `outline.md` | LLM review via `/q-review [outline.md]` |
| 5 | Plan | `/q-plan` | `plan.md` | LLM review via `/q-review [plan.md]` |
| 6 | Workspace | `/q-workspace` | prepared implementation workspace + synced plan dir | Base/stack safety gate before implementation |
| 7 | Implement | `/q-implement` | code changes + verified commits + review handoff | LLM code review via `/q-review [handoff.md]` |
| 8 | Review | `/q-review` | implementation `review.md` | Routes clean review to `/q-verify`; deeper findings create follow-up QRSPI |
| 9 | Verify | `/q-verify` | `verify.md` | Project-specific verification evidence before human approval |
| 10 | Human Review | runtime human gate | approval decision | Final human implementation approval |
| 11 | Done | runtime completion | `done.md` or final summary | Terminal whole-plan completion summary |

`/q-review` is a router:

- Before code exists, it loads `q-review-plan`.
- After implementation is complete, it loads `q-review-implementation`.

## Review Loops

### Planning review before implementation

Run planning review after `outline.md` and again after `plan.md`:

```text
# default path
/q-outline [plan_dir]/design.md
/q-review [plan_dir]/outline.md
/q-plan [plan_dir]/outline.md
/q-review [plan_dir]/plan.md
/q-workspace [plan_dir]/plan.md
/q-implement [plan_dir]/plan.md

```

Product-design review is a standalone, human-invoked helper, not a QRSPI stage or automatic gate. QRSPI must never suggest, require, or route to it. If a human explicitly creates `design-product.md`, later stages consume that artifact without changing the graph.

Planning review findings are handled in three ways:

1. `obvious_doc_fix` — edit `design.md`, `design-product.md`, `outline.md`, or `plan.md` directly during review.
1. `needs_codebase_research` — create a research questions doc under the timestamped planning review directory, then run `/skill:q-research-for-review` on it. After research, run `/skill:q-address-review-research` to update the parent planning docs from `review.md` plus the research doc.
1. `needs_human_judgment` — ask through `/answer`, then update the planning docs from the decision. This should be rare when `/q-question` did its job.

Planning-review research directories are lightweight research workspaces. They do not get their own `design.md`, `design-product.md`, `outline.md`, or `plan.md`; the researched fixes apply back to the parent planning docs.

### Implementation review after code exists

Implementation review examines actual code and verification evidence.

- `straightforward_fix` findings can be fixed immediately as a final review-fix slice stacked on top of the implementation.
- When no findings remain, final implementation review writes the canonical implementation `review.md`, emits outcome `ready-for-human-review`, and routes to `/q-verify` before the final human implementation gate. It must not create stale terminal `done.md` before verification evidence exists.
- Deeper findings become a full QRSPI follow-up plan inside the timestamped implementation review directory. That review directory gets its own `questions/`, `research/`, `design.md`, `design-product.md`, `outline.md`, `plan.md`, `handoffs/`, and nested `reviews/`. After review-dir `plan.md` passes planning review, skip `/q-workspace` and run `/q-implement` directly in the original reviewed implementation workspace. New branch slices stack on top of the exact reviewed implementation head **in the same implementation workspace**, even if the reviewed stack later merges to trunk. Do not create a separate workspace for implementation-review follow-up research/design/outline/plan/implement.

Never overwrite the parent plan's `design.md`, `design-product.md`, `outline.md`, or `plan.md` for implementation-review follow-up work.

## Key Principles

- **Do not outsource the thinking.** The engineer is a critical part of the human gates. The agent dumps; the human steers.
- **LLM review edits artifacts.** Planning review runs after outline and plan, and should improve `design.md`, `design-product.md`, `outline.md`, and `plan.md` directly when fixes are clear. A passive report is not enough.
- **Human-facing planning is compressed.** For `design.md`, `design-product.md`, and `outline.md` artifacts: be extremely concise. Sacrifice grammar for the sake of concision. In `/q-question`, apply that style to the brainstorm/interview turns, not to the final research questions doc.
- **Separate context windows.** Question and Research run in fresh contexts. Research reads `AGENTS.md` and question docs for framing, stays blind to forward-looking plan artifacts, and answers questions with codebase facts.
- **Instruction budget.** Keep each stage skill focused. Do not combine stages into one mega-prompt.
- **Dumb zone.** Context windows degrade when overfilled. Load only the artifacts the stage skill names.
- **Vertical slices, not horizontal layers.** Each slice ships end-to-end with a verification checkpoint.
- **Fresh implementation directories, never worktrees.** `/q-workspace` creates or repairs a fresh filesystem copy named for the plan directory or ticket slug after final plan review. Do not use `git worktree`; use macOS `cp -ac source-dir clean-copy-dir` or Linux `cp -a --reflink=auto source-dir clean-copy-dir`.
- **Branch/submission model is repository-specific, and `cn-agents` uses workspace stack branches.** The fresh implementation workspace isolates concurrent work; branch policy follows the repo. Chestnut monorepo work normally uses Graphite slice branches. `cn-agents` QRSPI work also uses Graphite slice branches inside the fresh implementation workspace. `/q-workspace` selects the correct base: latest `main` when safe for normal plans, an unmerged stack top for continuation plans, or the exact reviewed implementation workspace/head for implementation-review follow-up plans. Then `/q-implement` runs `gt create ..._slice-N` or `..._review_plan_slice-N` for tracked edit slices, commits with Graphite, and merges the completed stack back with `/cn-agents-merge`. Do not commit QRSPI implementation slices directly to `main` in `cn-agents`.
- **Design = brain dump + brain surgery.** Capture the approved technical direction in a lean doc; keep detailed decisions in ADRs.
- **Plan = tactical machine doc.** The plan is written for the implementing agent, but it still gets an LLM review before code starts.

## The Process Is Not Linear

The stages are the typical forward flow, but loops are expected:

- **Research -> Research**: Research reveals new code-answerable factual questions that materially inform design; create another research doc before `/q-design`.
- **Research -> Question**: Research reveals the questions missed human-goal/scope alignment, not just code facts.
- **Design -> Research**: Design needs facts not covered by existing research.
- **Outline -> Design**: Structural planning reveals a technical design flaw.
- **Plan -> Outline/Design**: Implementation steps reveal a slice, requirement, or interface problem.
- **Planning Review -> Research for Review -> Address Review Research**: Review finds a factual gap; `q-research-for-review` answers it with category-aware context; `q-address-review-research` updates parent docs.
- **Implementation Review -> QRSPI follow-up**: Code review finds deeper work; the implementation review directory becomes a new plan dir for stacked follow-up slices, but the filesystem workspace stays the same original implementation workspace that was reviewed and follow-up branches stack on top of the reviewed head.

When looping before implementation, update the parent planning docs. When looping after implementation review, write new planning artifacts under the implementation review directory.

## The Plan Directory

```text
thoughts/[git_username]/plans/[timestamp]_[plan-name]/
  AGENTS.md
  prds/
  context/
    brainstorms/    # q-question interview rationale for q-design
    question/
    research/
    design/
    design-product/
    outline/
    plan/
    implement/
    INDEX.md
  questions/
  research/
  design.md
  adrs/
  design-product.md
  outline.md
  plan.md
  handoffs/
  done.md            # terminal whole-plan completion summary after final implementation review
  reviews/
    YYYY-MM-DD_HH-MM-SS_[plan-name]_outline-review/
      review.md
      questions/     # only for planning-review codebase research follow-up
      research/
      context/
        brainstorms/
        research/
    YYYY-MM-DD_HH-MM-SS_[plan-name]_plan-review/
      review.md
      questions/     # only for planning-review codebase research follow-up
      research/
      context/
        brainstorms/
        research/
    YYYY-MM-DD_HH-MM-SS_[plan-name]_implementation-review/
      review.md
      AGENTS.md      # present when this review dir hosts follow-up QRSPI work
      prds/
      context/
        brainstorms/
        question/
        research/
        design/
        design-product/
        outline/
        plan/
        implement/
      questions/
      research/
      design.md
      adrs/
      design-product.md
      outline.md
      plan.md
      handoffs/
      reviews/
```

`context/` artifacts support later stages but do not replace primary stage artifacts. `context/brainstorms/` preserves q-question active context for q-design: Language / Domain Model, Alignment, decision branches, interview rationale, and ADR candidates. Load only the context subdirectories named by the active stage skill.

The copied `AGENTS.md` in each plan directory is curated long-term memory and the plan entrypoint. Preserve only durable decisions, gotchas, invariants, review learnings, language/ambiguity notes, ADR candidates, and pointers to canonical artifacts.

## Metadata Source

Before creating a new plan directory or writing a new markdown artifact, run:

```bash
~/dotfiles/spec_metadata.sh
```

Use its output for:

- `thoughts/[git_username]/...` path selection
- timestamped directory and filename values
- frontmatter fields such as date, researcher, git commit, branch, and repository

## Handoffs

Use `/q-handoff` to checkpoint progress within or between stages. Use `/q-resume` to pick up where you left off.

- After `design.md`: next is `/q-outline [design.md]`; `/q-outline` first summarizes key design decisions for human approval, then writes `outline.md` after `go`/`vamos`/`yes`/equivalent approval.
- A human may invoke standalone product-design review at any point. Existing `design-product.md` is supporting context only and does not alter QRSPI stage routing.
- After `outline.md`: next is `/q-review [outline.md]`.
- After `plan.md`: `/q-plan` runs `just sync-thoughts`, then next is `/q-review [plan.md]`. After successful normal parent-plan review, immediately run `/q-workspace [plan.md]` in the next session; it creates/repairs the implementation workspace, records the base branch/commit in YAML and plan memory, and then immediately run `/q-implement [plan.md]` unless safety checks block. Exception: if `plan.md` is inside an implementation-review follow-up directory (`[parent]/reviews/*_implementation-review/`), any nested review directory that is already in a prepared implementation workspace, or the human explicitly says to implement in the current workspace, successful plan review must route directly to `/q-implement [plan.md]` in the existing implementation workspace; do not create a new workspace or reset to trunk.
- During implementation: intermediate handoffs resume with `/q-resume` in the same `/q-workspace`-recorded implementation workspace. The workspace is the unit of isolation; do not assume a branch exists or should be created. For Graphite edit slices, implement and verify the work, update plan status, then create/modify the slice branch with `gt create`/`gt modify`; write the handoff after branch creation so it records final branch metadata, stage it, and amend it into the same slice commit with `gt modify`. Do not create handoffs before `gt create`, and do not chase self-referential final commit hashes inside the handoff.
- Repository commit policy must be preserved in plans/handoffs: monorepo usually means Graphite slice branches; `cn-agents` means fresh workspace plus Graphite slice branches for each tracked edit slice, then `/cn-agents-merge` at the end. Do not record a `cn-agents` expectation to stay on `main` for slice commits.
- Implementation handoffs record the `/q-workspace` implementation directory; they must not instruct agents to create ad-hoc copies and must not point agents at `git worktree`.
- After all implementation slices are complete: the completion handoff advances to `/q-review [handoff.md]` in implementation mode. Clean implementation review advances to `/q-verify [review.md] [project-guide]`; successful verify advances to the final human implementation gate.

## Standard Context Loading

Every ticket-level QRSPI stage skill and every ticket-level handoff/resume continuation starts by:

1. Reading this ticket-level pipeline overview.
1. Reading the skill for the current/next stage named by the prior `next.steps` steps.
1. Reading exactly the artifacts listed in that stage skill.
1. Starting that stage immediately unless the immediate continuation contract names a human approval gate or a safety stop applies.

Do not bulk-load the whole plan directory.

## Stage Skills

Each stage skill contains the full process, templates, and rules for that step:

- `.pi/skills/q-question/SKILL.md`
- `.pi/skills/q-research/SKILL.md`
- `.pi/skills/q-design/SKILL.md`
- `.pi/skills/q-outline/SKILL.md`
- `.pi/skills/q-plan/SKILL.md`
- `.pi/skills/q-workspace/SKILL.md`
- `.pi/skills/q-implement/SKILL.md`
- `.pi/skills/q-review/SKILL.md`
- `.pi/skills/q-review-plan/SKILL.md`
- `.pi/skills/q-review-implementation/SKILL.md`
- `.pi/skills/q-research-for-review/SKILL.md`
- `.pi/skills/q-address-review-research/SKILL.md`

## Rules

- When a stage needs fresh discovery, use that stage's preferred read-only discovery/analyzer flow and write artifacts under `context/[stage]/`.
- Each stage reads artifacts from prior stages as directed by its skill. Do not skip stages for non-trivial work.
- Question, Design, and pre-outline alignment include human review before they emit the next artifact. Research has no human stop. After outline review, `/q-plan` starts immediately: read relevant code files and write the plan without another human approval prompt. Outline and plan are LLM-reviewed gates before implementation.
- `/q-review [outline.md]` and `/q-review [plan.md]` should revise planning docs toward readiness, including `design-product.md` when present, not merely report issues.
- `/q-implement` uses `/q-resume` checkpoint handoffs for intermediate slices and only hands off to `/q-review` after all slices are complete and verification passes.
- `/q-implement` and implementation-stage `/q-resume` work must happen in the fresh filesystem copy created/repaired by `/q-workspace` and recorded in `workspace_metadata.implementation_workspace`. The paired `plan_workspace` records the plan/review directory whose artifacts drive the work. For implementation-review follow-up plans, this means the same original implementation workspace that was reviewed, not a new copy, and the reviewed head must remain an ancestor of the follow-up branch stack. Never use `git worktree`.
- Branching is not automatic. Follow the target repo's submission model after entering the workspace: use Graphite slice branches in repos that use Graphite. `cn-agents` uses this model too: create a branch for each tracked edit slice in the workspace, and use `/cn-agents-merge` after implementation/review is complete. For review-fixes plans, do not create a second workspace; reuse the original implementation workspace and stack review-plan branches on top of the reviewed implementation head.
- `/q-review` must run `just sync-thoughts` after modifying planning artifacts when the recipe exists. Final normal parent-plan review advances to `/q-workspace`, which syncs the reviewed plan directory into the chosen implementation workspace before `/q-implement`. Final same-workspace review-dir or implementation-review follow-up plan review skips `/q-workspace` and advances directly to `/q-implement` in the existing implementation workspace.
- Keep `plan.md` status checkboxes updated during implementation.
- When looping back before implementation, update parent planning artifacts. When addressing implementation review follow-up, use the implementation review directory as the new plan dir, but keep using the same implementation workspace and reviewed head recorded by the parent implementation/review YAML. Do not run a separate copied workspace for review follow-up work.
- When a stage creates or updates an artifact, use `~/dotfiles/spec_metadata.sh` for timestamps and frontmatter.
- Stage completion YAML should include the full path to the created artifact in `artifact` and an ordered `next.steps` block containing only `step` children: read `qrspi-planning`, read the concrete next stage skill, read each required artifact in its own step, then start the next stage immediately. Do not emit slash-command text or bracketed alternatives in `next.steps`; resolve to the exact next stage for this result. If both `design.md` and `design-product.md` exist and the next stage uses design context, include read steps for both. For `/q-handoff` and resume handoffs, the artifact reads are explicit separate steps: read exact `design.md`, exact `outline.md`, exact `plan.md`, exact handoff path.
- Preserve the stage completion YAML after follow-ups: answer the follow-up if needed, then re-emit the fenced `yaml` `qrspi_result` with updated `summary`, `artifact`, and `next.steps`, followed by the mandatory concise human summary.
