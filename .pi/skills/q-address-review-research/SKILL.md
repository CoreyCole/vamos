---
name: q-address-review-research
description: Applies q-review-plan findings after follow-up research. Use after q-research-for-review answers planning-review questions to update parent design.md, design-product.md, outline.md, and plan.md based on review.md plus the research doc.
---

# Address Planning Review Research

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

`status` is lifecycle. `outcome` selects the graph branch. ``next.steps`` is an ordered instruction block containing only `step` children: read `qrspi-planning`, read the next stage skill, read the artifact(s) needed by that stage, then start the next stage immediately unless blocked by an explicit human/safety gate. Runtime transitions are graph-authoritative. Complete results must include ``outcome``. Review stages must use explicit node IDs (`review-outline`, `review-plan`, or `review-implementation`), never `review`.

> **Planning review skill:** `.pi/skills/q-review-plan/SKILL.md`

Use this after a planning review created `needs_codebase_research` questions and `/skill:q-research-for-review` answered them. Read the review artifact and research doc, then update the parent planning documents directly.

This skill is only for pre-implementation planning-review follow-up. Implementation review follow-up uses a full QRSPI loop inside the implementation review directory.

## When Invoked

1. Read `.pi/skills/qrspi-planning/SKILL.md`.
1. Read `.pi/skills/q-review-plan/SKILL.md`.
1. Resolve inputs:
   - Preferred: `/skill:q-address-review-research [review.md path] [research.md path]`
   - If only a research doc path is provided, resolve the review directory from its parent and read `[review_dir]/review.md`.
   - If only a review directory path is provided, read `[review_dir]/review.md` and the newest `research/*.md` under it.
1. If required inputs are missing, respond:

```text
I'll apply planning review follow-up research to the parent design/product-design/outline/plan docs.

Please provide:
- the planning review artifact, e.g. `thoughts/[git_username]/plans/.../reviews/..._plan-review/review.md`
- and the research doc from `/skill:q-research-for-review`, e.g. `thoughts/[git_username]/plans/.../reviews/..._plan-review/research/YYYY-MM-DD_HH-MM-SS_topic.md`
```

Then wait for input.

## Load Context

Read:

- `[review_dir]/review.md`
- the provided research doc fully
- `[review_dir]/questions/*.md` relevant to the research
- the parent plan docs referenced by the review frontmatter:
  - `[parent_plan_dir]/AGENTS.md`
  - `[parent_plan_dir]/design.md` when present
  - `[parent_plan_dir]/design-product.md` when present
  - `[parent_plan_dir]/outline.md` when present
  - `[parent_plan_dir]/plan.md` when present
- code files explicitly referenced by the research doc when you need to verify how the doc update should be worded

Do not load unrelated parent-plan context unless needed to make the edit accurately.

## Process

1. Confirm the review is a planning review, not an implementation review.
1. Identify the `needs_codebase_research` findings from `review.md` and the questions answered by the research doc.
1. For each researched finding:
   - Determine whether the research resolves the uncertainty.
   - Edit the parent `design.md`, `design-product.md`, `outline.md`, and/or `plan.md` directly when the right planning-doc fix is now clear.
   - Preserve stage boundaries: design captures technical what/why, product design captures PRD coverage and product gates, outline captures structure and slices, plan captures exact implementation steps.
1. If the research shows the original finding is invalid, update `review.md` to mark it resolved as `no_doc_change_needed` and explain why.
1. If the research reveals another factual gap, write a new neutral questions doc under `[review_dir]/questions/` and make `/q-research` the next step.
1. If the research reveals a genuine business/product decision, ask via `/answer`; after the answer, apply the decision to the parent docs.
1. Re-read edited docs for consistency.
1. Update the same `[review_dir]/review.md` with:
   - research doc path
   - findings addressed
   - doc edits applied
   - findings dismissed by research
   - remaining research or human-decision follow-up
   - recommended next step
1. If durable decisions or review learnings should survive context resets, update `[parent_plan_dir]/AGENTS.md`.

## Edit Guidance

Apply the smallest doc changes that resolve the researched findings:

- Update `design.md` for changed goals, constraints, architecture choices, invariants, rejected paths, or approach rationale.
- Update `design-product.md` for PRD coverage, product Critical Findings, accepted non-goals/overrides, user/demo implications, or E2E edge cases.
- Update `outline.md` for slice boundaries, interfaces, signatures, sequencing, test checkpoints, rollout steps, or integration shape.
- Update `plan.md` for exact file edits, implementation order, test commands, migration commands, rollback instructions, or status checklist changes.

Do not create a nested design/outline/plan under the planning review directory. That review directory is only a research workspace for this follow-up path.

## Response Shapes

All response shapes must be a fenced YAML ``qrspi_result`` block followed by the mandatory concise human summary. Use the exact helper stage ID provided by the runtime prompt: `address-review-research-outline` or `address-review-research-plan`.

If all researched findings are addressed:

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

If more codebase research or human judgment is needed, use ``status`blocked`status`` or ``status`needs_human`status`` and summarize the unresolved findings. Do not create a nested design/outline/plan under the planning review directory.

## Rules

- Only use this skill for planning-review research follow-up.
- Read both the review artifact and research doc before editing.
- Edit parent planning docs directly when research resolves the finding.
- Keep stage boundaries clear between design, product design, outline, and plan.
- Do not edit implementation code.
- Do not create a full nested QRSPI plan in the planning review directory.
- Update the original review artifact instead of creating a second review artifact.
- Always return exact artifact and next-step paths.
