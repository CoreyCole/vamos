---
name: q-milestone-review
description: Review milestone-level QRSPI design artifacts for nested project planning. Use when running /q-milestone-review on milestone design.md before ticket creation. Checks artifact ownership, requirement traceability, current-state evidence, architecture-spec readiness, ticket boundaries, dependencies, and readiness for q-milestone-create-tickets.
---

# Milestone Review — Is This Ready for Ticket Creation?

Use this as the automated Review stage for milestone-level QRSPI. It reviews milestone `design.md` before `/q-milestone-create-tickets`. It replaces the old outline/plan review gates for new milestone planning.

## Step 1: Load baseline workflow

Read:

1. `.pi/skills/qrspi-planning/SKILL.md`
1. `.pi/skills/qrspi-project-planning/SKILL.md`
1. `.pi/skills/q-review/SKILL.md`
1. `.pi/skills/q-review-plan/SKILL.md`
1. `.pi/skills/q-milestone-question/references/milestone-planning-common.md`
1. `design.md` artifact under review
1. prior milestone-plan artifacts needed to evaluate it
1. milestone-plan `AGENTS.md`
1. milestone `AGENTS.md` and optional `milestone.md`
1. project plan `AGENTS.md` for canonical pointers/invariants

For new milestone planning, review `design.md` only. If given legacy `outline.md` or `plan.md`, review it only to finish an in-flight old flow; do not route new work through those gates.

## Step 2: Review design

Check:

- milestone is vertically shaped around a named product path/scenario/user path, or explicitly justifies why an enabling horizontal milestone is unavoidable
- milestone name uses product/domain language and does not include process jargon like "Vertical"
- smallest credible testable/demoable outcome is clear
- milestone ownership/non-goals clear
- current-state/source-doc evidence sufficient for design
- product outcomes/user-visible success clear and expressed as concise user stories
- gap map identifies user-visible and architecture/spec gaps
- architecture-spec inputs identified at design granularity
- proposed tickets each map to approved outcomes/user stories and gaps
- proposed tickets preserve end-to-end verifiability; horizontal/enabling tickets name the vertical path they unlock
- expected evidence for each ticket is concrete enough to seed Linear descriptions
- dependencies have owner/status and blocking order is clear
- deferred details really belong to ticket-level QRSPI
- ticket boundaries are neither too broad nor too narrow
- cross-milestone dependencies surfaced
- taxonomy changes proposed, not silently applied
- implementation details not over-specified

Next after clean automated review: `/q-milestone-create-tickets [design.md]`. Do **not** require or write `review-human.md`; create-tickets handles human approval in chat by summarizing the design, review verdict, and ticket-set proposal.

## Step 3: Write review artifact

Create:

```text
reviews/YYYY-MM-DD_HH-MM-SS_[slug]_design-review/review.md
```

Use q-review-plan finding categories when useful:

- `obvious_doc_fix` — edit milestone docs directly
- `needs_codebase_research` — create follow-up research questions in the review dir
- `needs_human_judgment` — ask via `/answer`

For clear doc fixes, update parent milestone-planning docs directly and run `just sync-thoughts` when available/appropriate.

## Response

Use fenced YAML `qrspi_result` blocks for all stage results. Required fields: `project`, `related_projects`, `stage`, `status`, `outcome` for complete results, `workspace`, `workspace_metadata`, `policy`, `summary`, `artifact`, `artifacts`, and structured `next.steps`.

Completed stage example:

```yaml
qrspi_result:
  project: "github.com/CoreyCole/vamos"
  related_projects: []
  stage: "milestone-review"
  status: "complete"
  outcome: "complete"
  workspace: "/absolute/path/to/thoughts/.../milestone-plan"
  workspace_metadata:
    plan_workspace: "/absolute/path/to/thoughts/.../milestone-plan"
    implementation_workspace: ""
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
    plan_goal: "Plan milestone tickets from reviewed requirements."
    stage_completed: "Automated milestone review complete."
    key_decisions: "Next stage should start immediately: /q-milestone-create-tickets; human approval happens there."
  artifact: "thoughts/.../review.md"
  artifacts:
    - role: "primary"
      path: "thoughts/.../review.md"
    - role: "reviewed-design"
      path: "thoughts/.../design.md"
  next:
    steps:
      - action: "read_skill"
        param: ".pi/skills/qrspi-planning/SKILL.md"
      - action: "read_skill"
        param: ".pi/skills/q-milestone-create-tickets/SKILL.md"
      - action: "read_artifact"
        param: "thoughts/.../design.md"
      - action: "read_artifact"
        param: "thoughts/.../review.md"
      - action: "start_stage"
        param: "q-milestone-create-tickets"
```
