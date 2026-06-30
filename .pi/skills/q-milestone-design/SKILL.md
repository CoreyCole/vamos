---
name: q-milestone-design
description: Create milestone-level QRSPI design for nested project planning. Use when designing a milestone-plan after research or running /q-milestone-design. Defines milestone ownership, outcomes, current-to-target direction, gap map, architecture inputs, dependencies, and proposed ticket set without creating implementation plans.
---

# Milestone Design — Where Is This Milestone Going?

Use this as the Design stage for milestone-level QRSPI. It mirrors `/q-design`, but it designs milestone ownership and ticket-shaping direction, not code implementation.

## Step 1: Load baseline workflow

Read:

1. `.pi/skills/qrspi-planning/SKILL.md`
1. `.pi/skills/qrspi-project-planning/SKILL.md`
1. `.pi/skills/q-design/SKILL.md`
1. `.pi/skills/q-milestone-question/references/milestone-planning-common.md`
1. milestone-plan `AGENTS.md`
1. all milestone-plan `questions/*.md`
1. all milestone-plan `context/brainstorms/*.md`
1. all milestone-plan `research/*.md`
1. relevant `context/research/*.md`
1. milestone `milestone.md`
1. project plan `AGENTS.md` only for canonical pointers/invariants

## Step 2: Run collaborative design interview

Milestone design is a human alignment gate. Do **not** write `design.md`, update milestone memory, or emit a complete YAML result until the interview reaches shared understanding or the human explicitly says to draft now.

Work like `/q-design` + `/grill-me`:

1. Create a timestamped design brainstorm artifact under `milestone-plan/context/design/` before asking the first design question.
1. Restate the loaded research in 3-5 bullets: current support, gaps, likely ticket-shaping pressure.
1. Walk the design tree one branch at a time. Ask exactly one direct question per turn, with your recommended answer and why.
1. If a question is answerable from code/docs/artifacts, inspect those instead of asking. Summarize the fact, then ask the next human-judgment question.
1. Update the brainstorm artifact after each confirmed decision or correction. Record prompt, user decision, rationale, and next implication. Do not raw-transcript tentative discussion.
1. Before drafting, summarize the proposed milestone direction and ask for approval to write `design.md`.
1. Only after approval, write `design.md` from the confirmed decisions.

Resolve these branches before drafting:

- vertical milestone shape: named product path/scenario/user path, smallest testable/demoable path, and why it is sequenced now; milestone name should use product/domain language, not "Vertical"
- milestone ownership and non-goals
- product outcomes and user-visible success criteria
- target users and concise user stories, including engineer-as-user stories only when outcome/architecture-enabling
- current-to-target direction
- gap map and architecture-spec inputs needed by the whole-system architecture ticket
- ticket-set structure before ticket details: name the vertical slices/workstreams, their sequence, what each proves, and what is intentionally deferred to later milestones
- proposed ticket set, with each ticket mapped to outcomes/user stories/gaps/evidence and ordered to preserve end-to-end verifiability
- cross-milestone dependency handling
- taxonomy change proposals, if any
- what must be deferred to ticket-level QRSPI

## Step 3: Write design artifacts after approval

Use `~/dotfiles/spec_metadata.sh` immediately before writing.

Write `design.md` in the milestone-plan directory only after explicit approval from Step 2. Target ~200-300 lines. Keep concise; tables/fragments preferred. Product outcomes and proposed ticket boundaries belong here as approved direction.

Required sections:

1. Executive summary.
1. Vertical slice / sequencing rationale.
1. Milestone ownership.
1. Non-goals.
1. Product outcomes / user-visible success.
1. Current state summary.
1. Target behavior as user stories.
1. Current to target direction.
1. Gap map.
1. Architecture-spec inputs.
1. Ticket-set structure / vertical workstream overview: group proposed work into the smallest coherent slices, explain sequence, what each slice proves, and what is postponed to later milestones.
1. Proposed ticket set with ticket → stories/gaps/evidence/dependencies; preserve vertical ordering and mark any unavoidable horizontal/enabling tickets with the vertical path they unlock.
1. Cross-milestone dependencies.
1. Taxonomy proposals.
1. Deferred to ticket-level QRSPI.
1. ADR candidate disposition.
1. Open questions.

Write ADRs only for accepted durable decisions that are hard to reverse or surprising without context.

## Step 4: Update memory

If approved design introduces durable invariants, update `milestone-plan/AGENTS.md` with short pointers to `design.md`, the design brainstorm artifact, or ADRs. Do not duplicate design content.

## Response

During the interview, do not emit a completion YAML block. Ask the next one-question design prompt.

Use fenced YAML `qrspi_result` blocks for completed, blocked, or needs-human stage results. Required fields for complete results: `project`, `related_projects`, `stage`, `status`, `outcome`, `workspace`, `workspace_metadata`, `policy`, `summary`, `artifact`, `artifacts`, and structured `next.steps`.

Completed stage example:

```yaml
qrspi_result:
  project: "github.com/CoreyCole/vamos"
  related_projects: []
  stage: "milestone-design"
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
    stage_completed: "Milestone design complete."
    key_decisions: "Next stage should start immediately: /q-milestone-review."
  artifact: "thoughts/.../design.md path"
  artifacts:
    - role: "primary"
      path: "thoughts/.../design.md path"
  next:
    steps:
      - action: "read_skill"
        param: ".pi/skills/qrspi-planning/SKILL.md"
      - action: "read_skill"
        param: ".pi/skills/q-milestone-review/SKILL.md"
      - action: "read_artifact"
        param: "thoughts/.../design.md path"
      - action: "start_stage"
        param: "q-milestone-review"
```
