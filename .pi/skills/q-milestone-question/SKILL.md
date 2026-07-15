---
name: q-milestone-question
description: Create milestone-level QRSPI research questions for nested project planning. Use when starting a milestone-plan directory, planning what tickets should exist for a milestone, or running /q-milestone-question. Focuses on milestone ownership, current-state discovery, user stories, architecture inputs, and ticket-shaping questions.
---

# Milestone Question — What Must This Milestone Learn?

Use this as the Question stage for milestone-level QRSPI. It mirrors `/q-question` mechanics, but its goal is milestone planning: decide what research is needed before shaping implementation/spec tickets.

## Step 1: Load baseline workflow

Read:

1. `.pi/skills/qrspi-project-planning/SKILL.md`
1. `.pi/skills/q-question/SKILL.md`
1. `.pi/skills/q-milestone-question/references/milestone-planning-common.md`
1. project plan `AGENTS.md`
1. milestone `milestone.md`, if present
1. existing milestone-plan `AGENTS.md`, if present
1. relevant parent status/routing artifacts named by project `AGENTS.md`

Do not bulk-load unrelated project docs.

## Step 2: Establish artifact ownership

Confirm the active directory is a `milestone-plan/` directory or create/use one under a milestone directory.

Use the common reference ownership model:

- project plan directory owns cross-milestone truth
- milestone directory owns local milestone truth
- milestone-plan directory owns milestone-level QRSPI
- ticket directories own ticket-level QRSPI

## Step 3: Interview for milestone alignment

Ask one question at a time unless the user asks for a batch. Explore docs/code first when the answer is factual.

Resolve:

- whether this milestone is a vertical product path/scenario; if not, why a horizontal/enabling milestone is unavoidable; milestone names should use product/domain language, not "Vertical"
- milestone owns / does not own
- why this milestone exists
- smallest credible testable/demoable outcome
- product outcomes and user-visible success
- who the users are, including engineer-as-user cases
- demo/review scenarios humans care about
- what current-code accuracy is required
- which source docs are canonical
- expected architecture-spec inputs
- likely ticket-shaping boundaries
- what must be deferred to ticket-level QRSPI
- taxonomy/dependency concerns

## Step 4: Write question artifacts

Create the normal q-question outputs under the milestone-plan directory:

- `context/brainstorms/YYYY-MM-DD_HH-MM-SS_[slug].md`
- `questions/YYYY-MM-DD_HH-MM-SS_[slug].md`

Use `~/dotfiles/spec_metadata.sh` before writing.

The question doc must ask for research that can support milestone design/ticket-set:

1. Vertical slice hypothesis: named product path/scenario/user path, smallest testable/demoable outcome, and why this sequence comes before broader variants.
1. Product Outcome Hypotheses and user-visible success criteria to validate.
1. Current code/system state relevant to this milestone.
1. Requirement/source-doc state, with canonical paths.
1. Current support for product outcomes: supported / partial / missing.
1. Gap map candidates: current support / partial / missing.
1. User stories to support, including engineer-as-user stories only when outcome/architecture-enabling.
1. Architecture-spec inputs and cross-cutting decisions.
1. Proposed ticket boundary evidence.
1. Cross-milestone dependencies.
1. Taxonomy change risks.
1. Details to defer to ticket-level QRSPI.

## Step 5: Update milestone-plan memory

Create or update `milestone-plan/AGENTS.md` with routing-only durable memory:

- current focus
- canonical project/milestone pointers
- approved scope boundaries
- durable terms/ambiguities
- next command

Keep it short. Do not copy full requirements.

## Response

Use fenced YAML `qrspi_result` blocks for all stage results. Required fields: `project`, `related_projects`, `stage`, `status`, `outcome` for complete results, `workspace`, `workspace_metadata`, `policy`, `summary`, `artifact`, `artifacts`, and structured `next.steps`.

Completed stage example:

```yaml
qrspi_result:
  project: "github.com/CoreyCole/vamos"
  related_projects: []
  stage: "milestone-question"
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
    enable_plan_reviews: false
    invalid_result_retry_limit: 1
  summary:
    plan_goal: "Plan milestone tickets from requirements."
    stage_completed: "Milestone question complete."
    key_decisions: "Next stage should start immediately: /q-milestone-research."
  artifact: "thoughts/.../question doc path"
  artifacts:
    - role: "primary"
      path: "thoughts/.../question doc path"
  next:
    steps:
      - action: "read_skill"
        param: ".pi/skills/qrspi-project-planning/SKILL.md"
      - action: "read_skill"
        param: ".pi/skills/q-milestone-research/SKILL.md"
      - action: "read_artifact"
        param: "thoughts/.../question doc path"
      - action: "start_stage"
        param: "q-milestone-research"
```
