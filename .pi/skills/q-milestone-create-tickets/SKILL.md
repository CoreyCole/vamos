---
name: q-milestone-create-tickets
description: Create provider tickets from a reviewed milestone design. Use after milestone design review. Summarizes the proposal, gets human approval in chat, drafts ticket bodies one by one in the plan dir, creates Linear issues after each approved body, then updates routing/status artifacts.
---

# Milestone Create Tickets — Turn Reviewed Design Into Provider Issues

Use after `/q-milestone-review [design.md]`. This replaces the old milestone `outline -> review -> plan -> review` flow.

Goal: convert a reviewed milestone `design.md` into a small set of provider tickets. Preserve vertical delivery: each ticket either advances the named product path/scenario or is a narrow enabler for that path.

## Step 1: Load context

Read:

1. `.pi/skills/qrspi-planning/SKILL.md`
1. `.pi/skills/qrspi-project-planning/SKILL.md`
1. `.pi/skills/q-milestone-question/references/milestone-planning-common.md`
1. milestone-plan `AGENTS.md`
1. milestone-plan `design.md`
1. latest automated design review `review.md`
1. milestone `AGENTS.md` and optional `milestone.md`
1. project status/routing artifact named by project `AGENTS.md`
1. thoughts root `AGENTS.md` for host link policy
1. provider defaults/project log named by project `AGENTS.md`

Stop only if `design.md` or automated `review.md` is missing. Do **not** require `review-human.md`; human approval happens in this skill's chat flow.

Provider routing:

- Resolve Linear/project/milestone/defaults from existing docs and the milestone planning issue first.
- If any required value is missing, ask the human for the missing links/IDs.
- Drafting can proceed before provider routing is complete; Linear creation cannot.
- Created implementation/spec tickets must be assigned to the Linear project milestone and must not be children of the milestone planning ticket.

## Step 2: Get ticket-set approval

Summarize the reviewed design and automated review verdict, then present this concise structure:

```text
Ticket-set proposal:
- Spine: [named product path/scenario]
- Tickets:
  1. [type: docs/spec|implementation/test] [tentative title] — [role/proves]
  2. ...
- Deferred: [later milestones/follow-ups]
- Risk check: [horizontal/enabling tickets and the vertical path they unlock]
Approve structure, or changes?
```

Do not draft individual ticket bodies until the structure is approved. If the human changes ticket count/scope materially, edit `design.md` so it matches the approved structure before creating Linear tickets.

## Step 3: Draft tickets one by one

For each approved ticket, write the exact provider body to:

```text
milestone-plan/context/create-tickets/drafts/tkt-NN-short-slug.md
```

The draft file is the exact Linear body: no frontmatter, no title heading, no suggested next command, no agent-only notes. The provider issue title lives outside the body.

Show exactly one ticket at a time:

1. Present the exact title and exact Markdown body from the draft file.
1. Ask the human whether to approve, change, or drop it.
1. Apply requested edits to the draft file.
1. Re-show the edited title/body after material changes.
1. Human approval means: create this Linear ticket now.

### Template selection

Classify each ticket in the structure proposal.

Docs/spec ticket (`docs(...)`, strategy/spec/audit/docs-only deliverable) uses:

```markdown
## Goal

## Scope

## Acceptance criteria

## Dependencies / relations

## Docs
```

Implementation/test ticket (`feat(...)`, `fix(...)`, `test(...)`, mixed docs+code, or production/test code change) uses:

```markdown
## Vertical path

## Role in ticket-set structure

## Goal

## User stories

## Where we are today

## Gaps we need to fill

## Expected outcome

## Testing strategy

### Unit

### Integration

### E2E

## Dependencies / relations

## Docs
```

Do not include operator-only creation guards, internal planning caveats, vague architecture-consumer language, or invented implementation details. Ticket-level QRSPI owns exact design and implementation.

Use markdown links for docs/assets according to thoughts root `AGENTS.md`. Do not link local absolute paths.

## Step 4: Create each approved ticket

After a ticket body is approved:

1. Create the provider ticket directly from the approved draft file.

1. Create routing dir only after the provider issue exists:

   ```text
   tickets/pro-####-short-slug/
   ```

1. Move the draft file to `tickets/pro-####-short-slug/ticket.md`; do not leave a duplicate provider body in `milestone-plan/context/create-tickets/drafts/`.

1. Write routing-only `AGENTS.md`.

1. Write ticket-root `index.md` linking the provider issue, `ticket.md`, canonical docs, and next `/q-question` command.

1. Comment on the created ticket with fenced YAML `qrspi_result`. Set `workspace` to the ticket directory, `artifact` to `ticket.md`, and `next.steps` to read QRSPI/question skills, read `ticket.md`, then start `q-question`.

1. Update `milestone-plan/AGENTS.md` create-ticket status with title, `ticket.md` path, created issue ID/URL, routing dir, and next ticket.

1. Move to the next ticket.

## Step 5: Post-pass after all tickets exist

After all tickets are created:

1. Add blocker/related relations in a post-pass.
1. Comment on the milestone planning ticket with markdown links to created tickets, key relations, and thoughts docs/assets.
1. Move the milestone planning ticket to the configured review/status value when available; do not mark it `Done`.
1. Update project provider ticket log.
1. Update milestone planning status artifact.
1. Update milestone `AGENTS.md` and optional `milestone.md`.
1. Ensure `milestone-plan/AGENTS.md` has a create-ticket status table and pending/created relations.
1. Run `just sync-thoughts` when available.

Use `milestone-plan/AGENTS.md` as the durable create-ticket status/recovery artifact. Do not create a separate manifest unless the table becomes too large.

Suggested status section:

```markdown
## Create-tickets status

| Order | Title | Type | Ticket body | Linear issue | Status |
|---|---|---|---|---|---|
| 1 | ... | docs/spec | `../tickets/pro-1234-slug/ticket.md` | PRO-1234 | created |

Relations:
- PRO-1234 blocks PRO-1235
```

## Response

Use fenced YAML `qrspi_result` blocks for all stage results. Required fields: `project`, `related_projects`, `stage`, `status`, `outcome` for complete results, `workspace`, `workspace_metadata`, `policy`, `summary`, `artifact`, `artifacts`, and structured `next.steps`.

Final completion:

- `artifact`: `milestone-plan/AGENTS.md`
- Include artifacts for project log, milestone planning status, created ticket `index.md` files, and `ticket.md` bodies.
- `summary.key_decisions`: name the first ticket-level `/q-question` that should start immediately, or state completion if no next ticket work should start.

Completed stage example:

```yaml
qrspi_result:
  project: "github.com/CoreyCole/vamos"
  related_projects: []
  stage: "milestone-create-tickets"
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
    stage_completed: "Milestone tickets created and routing docs updated."
    key_decisions: "Next stage should start immediately: /q-question for the first created ticket."
  artifact: "thoughts/.../milestone-plan/AGENTS.md"
  artifacts:
    - role: "ticket"
      path: "thoughts/.../tickets/pro-1234-slug/index.md"
  next:
    steps:
      - action: "read_skill"
        param: ".pi/skills/qrspi-planning/SKILL.md"
      - action: "read_skill"
        param: ".pi/skills/q-question/SKILL.md"
      - action: "read_artifact"
        param: "thoughts/.../tickets/pro-1234-slug/ticket.md"
      - action: "start_stage"
        param: "q-question"
```
