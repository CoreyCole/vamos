---
name: q-milestone-create-tickets
description: Create provider tickets from a milestone design. Use after milestone design. Summarizes the design, outlines the ticket set for human confirmation, drafts ticket bodies one by one in the plan dir, creates Linear issues after each approved body, then updates routing/status artifacts.
---

# Milestone Create Tickets — Turn Design Into Confirmed Provider Issues

Use after `/q-milestone-design [research.md]`.

Goal: summarize the milestone `design.md`, outline the proposed ticket set for human confirmation, then convert the approved structure into a small set of provider tickets. Preserve vertical delivery: each ticket either advances the named product path/scenario or is a narrow enabler for that path.

## Step 1: Load context

Read:

1. `.pi/skills/qrspi-project-planning/SKILL.md`
1. `.pi/skills/q-milestone-question/references/milestone-planning-common.md`
1. milestone-plan `AGENTS.md`
1. milestone-plan `design.md`
1. milestone `AGENTS.md` and optional `milestone.md`
1. project status/routing artifact named by project `AGENTS.md`
1. thoughts root `AGENTS.md` for host link policy
1. provider defaults/project log named by project `AGENTS.md`

Stop only if `design.md` is missing. Human approval happens in this skill's chat flow.

Provider routing:

- Resolve Linear/project/milestone/defaults from existing docs and the milestone planning issue first.
- If any required value is missing, ask the human for the missing links/IDs.
- Drafting can proceed before provider routing is complete; Linear creation cannot.
- Created implementation/spec tickets must be assigned to the Linear project milestone and must not be children of the milestone planning ticket.

## Step 2: Get ticket-set approval

Summarize the milestone design, then present this concise structure:

```text
Ticket-set proposal:
- Spine: [named product path/scenario]
- Tickets:
  1. [docs/spec|implementation/test] `[type(scope): imperative lowercase provider issue title]` — [role/proves]
  2. ...
- Deferred: [later milestones/follow-ups]
- Risk check: [horizontal/enabling tickets and the vertical path they unlock]
Approve structure and names, or changes?
```

Do not draft individual ticket bodies until the structure is approved. If the human changes ticket count/scope materially, edit `design.md` so it matches the approved structure before drafting or creating Linear tickets.

## Step 3: Draft tickets one by one

For each approved ticket, write the exact provider body to:

```text
milestone-plan/context/create-tickets/drafts/tkt-NN-short-slug.md
```

The draft file is the exact Linear body: no frontmatter, no title heading, no suggested next command, no agent-only notes. The provider issue title lives outside the body.

Review exactly one ticket at a time:

1. Present the exact title, draft file path, and a concise scope summary only.
1. Do not paste the full draft body unless the human explicitly asks; they can inspect the file.
1. Ask the lead engineer for an estimate and whether to approve, change, or drop the ticket. Do not create/update it without an approved estimate.
1. Apply requested edits to the draft file.
1. After material changes, re-show the title, estimate, draft path, and concise change summary only.
1. Human approval means: create or update this Linear ticket now with the approved estimate.

### Estimates

Capture one size per ticket and pass its numeric story-point value to `linear-cli` with `-e` / `--estimate`:

| Size | Lead-engineer duration | Linear CLI estimate |
| --- | --- | --- |
| XS | a few hours | `1` |
| S | 0.5 day | `2` |
| M | 1 day | `3` |
| L | 3 days | `5` |
| XL | 5 days | `8` |

Record both size and numeric estimate in create-ticket status. Never pass `XS`/`S`/`M`/`L`/`XL` to the CLI; `linear-cli` accepts story points such as `1`, `2`, `3`, `5`, and `8`.

### Title format

Provider issue titles use conventional-commit style:

```text
[type(scope): imperative lowercase summary]
```

Examples: `docs(audit-log): finalize quality bar contract`, `feat(audit-log): add carrier questions audit golden example`, `test(audit-log): prove deployed debug audit review flow`.

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

Do not include operator-only creation guards, internal planning caveats, vague architecture-consumer language, invented implementation details, or uncertainty placeholders (`if`, `likely`, `probably`, `maybe`, `where practical`, `when available`). Resolve ambiguity with the lead engineer during create-tickets, then write the ticket as decided scope. Ticket-level QRSPI owns exact design and implementation, not whether an approved deliverable exists.

### E2E / Ranger verification tickets

For any ticket whose primary purpose is E2E, Ranger, browser verification, or deployed/manual proof, make the `### E2E` strategy explicit enough to guide future ticket-level QRSPI:

- name the exact scenario/debug flow the test uses
- name setup workflows it depends on, such as ingestion testing workflow fixtures
- name fast-forward or state-advance controls needed to avoid waiting for scheduled jobs
- list the user-visible assertions Ranger must make
- list negative assertions for skipped paths, hidden tabs/actions, and no runtime errors
- state what evidence to capture and link from the ticket deliverable

If the scenario intentionally skips payout/ledger behavior, explicitly assert the payout UI is hidden/unavailable and no payout approval or ledger posting path runs.

Use markdown links for docs/assets according to thoughts root `AGENTS.md`. Do not link local absolute paths.

## Step 4: Create each approved ticket

After a ticket body is approved:

1. Create or update the provider ticket directly from the approved draft file and set the approved numeric estimate (`linear-cli i create ... -e N` or `linear-cli i update ... -e N`).

1. Create routing dir only after the provider issue exists:

   ```text
   tickets/pro-####-short-slug/
   ```

1. Move the draft file to `tickets/pro-####-short-slug/ticket.md`; do not leave a duplicate provider body in `milestone-plan/context/create-tickets/drafts/`.

1. Write routing-only `AGENTS.md`.

1. Write ticket-root `index.md` linking the provider issue, `ticket.md`, canonical docs, and next `/q-question` command.

1. Upsert the ticket's fenced YAML `qrspi_result` comment:

   - List comments first and update the existing create-tickets/QRSPI routing comment; do not append a second comment.
   - Create a comment only when no matching QRSPI comment exists.
   - If duplicates already exist, update the earliest applicable comment and delete later duplicates after verifying their content is superseded.
   - Provider-comment paths must be machine agnostic and repo-qualified: `<repo>/thoughts/...` (for example, `monorepo/thoughts/CoreyCole/...`). Never put `/Users/...`, checkout roots, or other machine-specific absolute paths in provider comments.
   - Set `workspace` and `workspace_metadata.plan_workspace` to the repo-qualified ticket directory; set `artifact`, `artifacts[].path`, and `next.steps[].param` artifact paths to repo-qualified paths too.
   - Route `next.steps` through `qrspi-project-planning`, `qrspi-planning`, and `q-question`, then read the repo-qualified `ticket.md` and start `q-question`. `qrspi-project-planning` confirms this is now a concrete ticket; `qrspi-planning` owns the individual ticket workflow.

1. Update `milestone-plan/AGENTS.md` create-ticket status with title, approved size/story points, `ticket.md` path, created issue ID/URL, routing dir, and next ticket.

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

| Order | Title | Type | Estimate | Ticket body | Linear issue | Status |
|---|---|---|---|---|---|---|
| 1 | ... | docs/spec | M / 3 | `../tickets/pro-1234-slug/ticket.md` | PRO-1234 | created |

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
    enable_plan_reviews: false
    invalid_result_retry_limit: 1
  summary:
    plan_goal: "Plan milestone tickets from requirements."
    stage_completed: "Milestone tickets created and routing docs updated."
    key_decisions: "Next stage should start immediately: /q-question for the first created ticket."
  artifact: "thoughts/.../milestone-plan/AGENTS.md"
  artifacts:
    - role: "ticket"
      path: "thoughts/.../tickets/pro-1234-slug/index.md"
  next:
    steps:
      - action: "read_skill"
        param: ".pi/skills/qrspi-project-planning/SKILL.md"
      - action: "read_skill"
        param: ".pi/skills/qrspi-planning/SKILL.md"
      - action: "read_skill"
        param: ".pi/skills/q-question/SKILL.md"
      - action: "read_artifact"
        param: "thoughts/.../tickets/pro-1234-slug/ticket.md"
      - action: "start_stage"
        param: "q-question"
```
