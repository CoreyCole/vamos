---
name: q-review-implementation
description: LLM review for completed QRSPI implementation code. Use after q-implement hands off to review; applies straightforward code fixes as a final stacked slice and creates a review-directory QRSPI plan for deeper follow-up work.
---

# QRSPI Implementation Review

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

`status` is lifecycle. `outcome` selects the graph branch. After `/q-workspace`, omit top-level `workspace` and record both `plan_workspace` and `implementation_workspace` inside `workspace_metadata`. `next.steps` is an ordered instruction block for the next agent: read QRSPI guidance, read q-verify, read design, read design-product if it exists, read outline, read plan, read the review artifact, read relevant repository verification docs/guidance for integration and E2E test context, then start verification immediately unless blocked. Runtime transitions are graph-authoritative and may validate/rewrite the steps. Complete results must include `outcome`. Review stages must use explicit node IDs (`review-outline`, `review-plan`, or `review-implementation`), never `review`.

> **Review rubric:** `~/.pi/agent/skills/review-rubric/SKILL.md`

Review the completed implementation against the plan, codebase reality, and verification evidence. Straightforward code fixes can be made immediately as a final review-fix slice. Deeper issues become a new QRSPI plan rooted in the timestamped review directory so follow-up branches stack on top of the exact implementation workspace/head that was reviewed. Do not create a separate workspace for implementation-review follow-up work.

## Finding Classification

Classify every real finding into exactly one bucket:

| Bucket | Meaning | Action |
|---|---|---|
| `straightforward_fix` | The bug, patch, and verification are clear and localized. | Apply it immediately in the review context as a final stacked slice, then verify. |
| `needs_followup_qrspi` | The issue needs research, design tradeoff analysis, multi-slice work, broad refactoring, rollout planning, or unclear ownership. | Create a review-directory QRSPI plan seeded by neutral questions. |

Do not downgrade deep issues into quick fixes just because a small patch is possible. If the right fix depends on facts or design judgment, use `needs_followup_qrspi`.

## Artifact Locations

Create one timestamped implementation review directory:

```text
[plan_dir]/reviews/YYYY-MM-DD_HH-MM-SS_[plan-name]_implementation-review/
  review.md
  AGENTS.md
  prds/
  questions/
  research/
  context/
    brainstorms/
    question/
    research/
    design/
    outline/
    plan/
    implement/
  adrs/
  outline.md
  design.md
  plan.md
  handoffs/
  reviews/
```

The canonical review artifact is:

```text
[review_dir]/review.md
```

When `needs_followup_qrspi` findings exist, the same `review_dir` is the follow-up QRSPI plan directory. Seed it with:

```text
[review_dir]/questions/YYYY-MM-DD_HH-MM-SS_[plan-name]_implementation-review-followup-questions.md
```

Later stages write `design.md`, optional `design-product.md`, `outline.md`, and `plan.md` inside `review_dir`, not in the parent plan. `/q-workspace` and `/q-implement` then use that review-dir `plan.md` to add follow-up slices on top of the already-reviewed implementation head in the same implementation workspace recorded by the parent implementation/review YAML, even if the reviewed stack later merges to trunk.

## Load Context

1. Read `.pi/skills/qrspi-planning/SKILL.md`.
1. Read `~/.pi/agent/skills/review-rubric/SKILL.md`.
1. Resolve `plan_dir` and the implement-complete handoff:
   - If a handoff path was provided, use it.
   - If a plan directory was provided, use the newest complete implement handoff in `[plan_dir]/handoffs/`.
1. Read:
   - `[plan_dir]/AGENTS.md`
   - the implement-complete handoff
   - `[plan_dir]/plan.md`
   - code files changed by implementation, using handoff sections, `git status`, `git diff`, `git show`, or the known branch range
   - each changed concrete file with the read tool (or the nearest existing neighboring file for newly created paths) so path-scoped `AGENTS.md` context is loaded before judging or applying review fixes
   - verification evidence from the handoff
   - relevant project guidance surfaced by the focused project-guidance lane, including root/package `AGENTS.md`, `.agents/rules/`, `.agents/skills/`, and docs referenced by the plan or changed files
   - doc health findings surfaced by the focused docs-health lane, including docs that should be corrected, simplified, or made more concise
1. Read `design.md`, optional `design-product.md`, `outline.md`, `questions/*.md`, `context/brainstorms/*.md`, `research/*.md`, PRDs/tickets, and planning context as needed to clarify intent and alignment. The primary review target is code plus verification evidence.

## Focused Review Lanes

Call `subagent({ action: "list" })` and confirm `scout` and `reviewer` are executable and non-disabled. Lane Markdown files are embedded prompts, not registered agents.

First run one fresh-context `scout` using `q-review/agents/q-review-lane-selector.md`. Give it the plan, implementation handoff, actual changed files/hunks or exact stack ranges, verification evidence, and applicable project guidance. Save its report to:

```text
[review_dir]/context/lane-selection.md
```

Read that report before launching lanes. Implementation selection must include `q-review-correctness`, `q-review-simplicity`, and `q-review-project-guidance`. There is no fixed lane maximum: launch every materially relevant specialist that owns a distinct question, and reject overlapping lanes unless the selector explains their non-overlapping responsibilities. Reject unknown lane IDs or selection based only on keywords/file extensions. If selection is malformed, rerun once; if still malformed, run only the three mandatory lanes and record the fallback.

For each selected lane, embed its `q-review/agents/q-review-*.md` prompt in a fresh-context `reviewer` task and save the report to `[review_dir]/context/lanes/[lane-id].md`. Run independent lanes in parallel. Reports are advisory: read and verify every candidate before including it. Do not claim a selector or lane ran unless its persisted report exists.

## Process

1. Run `~/dotfiles/spec_metadata.sh` before creating `review_dir` or writing markdown.
1. Create `review_dir` and write `review.md` there.
1. Build understanding from the handoff, changed files, verification evidence, and relevant plan requirements.
1. Enumerate changed file paths from git and handoff evidence; read each changed file or nearest neighboring file so all applicable `AGENTS.md` guidance for those paths is loaded before review.
1. Summarize the implemented behavior at a high level and check alignment with PRDs, ticket text, question docs, `context/brainstorms/`, research findings, design/outline/plan, and approved plan-memory constraints.
1. Review actual code for correctness, regressions, security, invariants, tests, operations, maintainability, doc health, and compliance with relevant project guidance (`AGENTS.md`, `.agents/rules/`, `.agents/skills/`, and docs). Explicitly verify the implementation follows the repo guidance loaded for the changed paths. Preserve conflicting relevant guidance as `IMPORTANT: needs human attention`; do not silently choose between conflicting instructions.
1. Run the selector's focused lanes with `subagent`; read every report under `[review_dir]/context/lanes/`; verify candidate findings yourself.
   - Treat a lane output as failed if it is empty, only contains raw tool-call markup/JSON such as `<tool_call>` or `{"cmd": ...}`, lacks the required lane report sections, or contains no evidence for its findings.
   - Rerun each failed lane once with the same task plus an explicit reminder to actually use tools and return only the markdown lane report.
   - If the rerun still fails, record the lane as unavailable in `review.md` and continue with your own targeted verification instead of trusting it.
1. For every lane finding, record a disposition: `fix`, `ignore`, or `needs_followup_qrspi`, with a concise rationale tied to code evidence and declared requirements. Never silently omit a lane finding.
1. Classify findings into `straightforward_fix` and `needs_followup_qrspi`. Treat conflicting relevant project guidance as `needs_followup_qrspi` unless it can be resolved by a clearly more-specific local instruction; label it `IMPORTANT: needs human attention` in `review.md` and seed neutral follow-up questions that ask which source is authoritative.
1. Write the initial `review.md` before applying code fixes or creating follow-up docs.
1. Apply all `straightforward_fix` findings directly when safe:
   - Create or reuse a final review-fix slice on top of the implementation stack when tracked source/test/doc files change.
   - If the repo uses Graphite and you are not already on a dedicated review-fix branch, create a branch on top of the current implementation head using the existing ticket slug plus `review-fixes` or another obvious final-slice suffix.
   - Never make review fixes directly on `develop`.
   - Keep fixes limited to the verified straightforward findings.
   - Run the specific verification command for each fix.
   - Commit only files changed by these fixes when project workflow expects committed slices.
1. For `needs_followup_qrspi` findings, initialize `review_dir` as a normal QRSPI plan:
   - copy `AGENTS.md` from `.pi/skills/qrspi-planning/_AGENTS.md` if missing
   - create `prds/`, `questions/`, `research/`, `adrs/`, `handoffs/`, `reviews/`, and `context/{brainstorms,question,research,design,design-product,outline,plan,implement}/`
   - write `prds/source-review.md` pointing to `review.md`
   - write neutral research questions under `questions/`
1. Update `review.md` with applied fixes, commits/branches if any, verification results, and the follow-up question doc path.
1. If durable review learnings should survive, update `[plan_dir]/AGENTS.md`; for follow-up work, also update `[review_dir]/AGENTS.md` with the current focus and source review link.

## Review Artifact Template

````markdown
---
date: [ISO datetime with timezone]
reviewer: [git_username]
git_commit: [current commit hash]
branch: [current branch]
repository: [repository name]
plan_dir: [exact parent plan dir path]
review_dir: [exact review dir path]
review_mode: implementation
reviewed_artifact: [exact implement handoff path]
status: complete
type: implementation_review
verdict: [correct|needs_attention]
---

# Implementation Review: [plan name]

## Summary
[Short assessment of the implementation after any straightforward fixes.]

## Current Implementation
[High-level summary of what the implementation does now.]

## Requirements Alignment
- PRD/ticket requirements: [aligned/gaps, with refs]
- Brainstormed requirements and decisions: [aligned/gaps, with refs to `context/brainstorms/`]
- Design/outline/plan commitments: [aligned/gaps, with refs]
- Verification evidence: [what proves alignment and what remains unproven]

## Findings Summary
- [Finding summary, or `None.`]

## Findings
### Finding 1: [Title]
- Classification: [straightforward_fix|needs_followup_qrspi]
- Priority: [P0|P1|P2|P3]
- References: [code refs]
- Issue: [What is wrong.]
- Example: [Concrete runtime or maintenance scenario showing why it matters.]
- Resolution: [Applied fix and verification, or follow-up QRSPI questions.]

## Simplifications Applied
- Removed/collapsed/narrowed: [changes or `None.`]
- Complexity retained: [item and requirement/risk that requires it, or `None.`]

## Focused Review Lanes
- [Lane report path and concise result for every invoked lane.]

## Lane Finding Decisions
| Lane finding | Decision | Rationale |
|---|---|---|
| [report ref] | fix / ignore / needs_followup_qrspi | [evidence-based reason] |

## Conflicting Guidance
- IMPORTANT: needs human attention — [conflict summary with exact source refs and decision needed, or `None.`]

## Applied Straightforward Fixes
- `[path]` — [what changed, branch/commit if applicable, verification]

## Follow-up QRSPI Plan
- Plan dir: [review_dir or `None.`]
- Questions doc: [path or `None.`]
- Findings included: [finding numbers or `None.`]

## Verification
- [Commands and outcomes, including the changed files read to load applicable `AGENTS.md` guidance and whether the implementation follows it.]

## Recommended Next Steps
[Next command.]
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

If deeper follow-up QRSPI work is needed, keep the primary artifact as `review.md`, include a follow-up plan or questions artifact, and route back to QRSPI question/research in the review-dir context. `implementation_workspace` must remain the same original implementation workspace and `plan_workspace` must identify the review-dir plan workspace; downstream follow-up planning must not create a separate workspace:

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
```

If straightforward fixes were attempted but verification still fails, use `` status`blocked`status `` or `` status`error`status `` with the review artifact and verification failure in `summary`.

## Rules

- Review code and verification evidence, not just planning docs.
- Write `review.md` before applying code fixes or creating follow-up plans.
- Apply only `straightforward_fix` code changes directly.
- Treat direct code fixes as a final review-fix slice stacked on top of the implementation, not as parent planning-doc edits.
- Never edit the parent plan's `design.md`, `design-product.md`, `outline.md`, or `plan.md` for implementation-review follow-up work.
- Put all deeper implementation follow-up planning artifacts in the timestamped `review_dir` as a fresh QRSPI plan with its own `design.md`, optional `design-product.md`, `outline.md`, and `plan.md`; do not create a fresh filesystem workspace for that follow-up. Implementation stays in the same original implementation workspace.
- Seed deeper follow-up with neutral research questions; do not copy review recommendations as settled solutions.
- Do not ask whether to create the follow-up QRSPI plan; create it automatically for `needs_followup_qrspi` findings.
- Do not create or route agents toward a separate implementation workspace for follow-up work. Preserve the original implementation workspace and reviewed head in YAML and handoffs so follow-up branches stack on top of the reviewed implementation.
- Surface conflicting relevant project guidance as `IMPORTANT: needs human attention` with exact source refs and the decision needed; do not apply code fixes based on one side of the conflict until it is resolved.
- Prefer a short, verified review over speculative findings.
- In both `review.md` and the user response, summarize the current implementation at a high level and state how it aligns with PRDs, tickets, brainstormed requirements, research findings, design/outline/plan commitments, and verification evidence.
- Always summarize the canonical review artifact and exact next command.
