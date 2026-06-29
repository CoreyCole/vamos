---
name: q-milestone-create-tickets
description: Create provider tickets from a reviewed milestone design. Use after milestone design review and human approval. Summarizes each proposed ticket one by one for human refinement, writes concise provider-ready descriptions, then confirms each exact title/body before creating that ticket.
---

# Milestone Create Tickets — Turn Reviewed Design Into Provider Issues

Use this after `/q-milestone-review [design.md]` and human design approval. This replaces the old milestone `outline -> review -> plan -> review` flow.

Goal: convert the reviewed milestone design into a small set of concise provider tickets, with human refinement one ticket at a time. Tickets should preserve vertical delivery: each ticket should either move the named product path/scenario closer to end-to-end verification or be a narrowly scoped enabler for that path. Before drafting individual tickets, align with the human on the overall ticket-set structure and vertical slices/workstreams. Before creating each provider ticket, show the exact title and description body and get human approval for that ticket.

## Step 1: Load baseline workflow

Read:

1. `.pi/skills/qrspi-planning/SKILL.md`
1. `.pi/skills/qrspi-project-planning/SKILL.md`
1. `.pi/skills/q-milestone-question/references/milestone-planning-common.md`
1. milestone-plan `AGENTS.md`
1. milestone-plan `design.md`
1. latest design review and `review-human.md`
1. milestone `AGENTS.md` and optional `milestone.md`
1. project status/routing artifact named by project `AGENTS.md`
1. thoughts root `AGENTS.md` host manifest for `qrspi_ticket_provider` and `qrspi_ticket_provider_config`
1. provider defaults/project log named by project `AGENTS.md`

Stop if design review or human approval is missing.

Validate provider config before preparing mutations:

- If `qrspi_ticket_provider` is missing, emit `status: needs_human` and ask for host provider config.
- If the provider is not `linear`, emit `status: blocked` unless this skill has been extended for that provider.
- If provider is `linear`, require manifest values for `linear_base_url`, workspace/team/project routing, milestone/default fields needed by the current milestone, and thoughts-viewer link construction.
- Do not create a provider ticket until that ticket's exact title and description body have been shown to the human and approved for creation.

## Step 2: Align on ticket-set structure

Before presenting individual provider drafts, extract the whole proposed set from reviewed `design.md` and present a concise structure overview:

- milestone spine: named product path/scenario/user path this ticket set proves
- vertical slices/workstreams: grouped proposed work, sequence, and what each group proves
- ticket list: tentative titles in order, with each ticket's role in the sequence
- defer map: what belongs to later milestones, especially final E2E / readiness backstops
- risk check: any ticket that looks horizontal/enabling and the vertical path it unlocks

Ask the human to approve or adjust the structure before drafting ticket 1. If the human changes structure, update the candidate list first. Do not start one-by-one ticket refinement until this structure is approved.

## Step 3: Extract candidate tickets

From the approved structure and reviewed `design.md`, summarize each proposed ticket in the tighter provider-ready format:

- title in Conventional Commit style, e.g. `feat(domain): add ordered first scenario selection`
- goal: one or two sentences naming the concrete outcome
- scope: bullets for the specific behavior/path to cover
- acceptance criteria: testable bullets only
- dependencies: blockers/blocks/related tickets
- docs: markdown links to source design/review artifacts

Do not include user-story filler, process jargon, vague architecture-consumer language, or invented implementation details. Ticket-level QRSPI owns exact design and implementation.

## Step 4: Refine tickets one by one

Show exactly one candidate ticket at a time.

For each ticket:

1. Present the concise provider-ready title and exact Markdown body.
1. Ask the human whether to approve, change, or drop it.
1. Apply requested edits.
1. Re-show the edited title/body if changes were material.
1. Do not proceed to the next ticket until the current ticket description is approved.

Do not include operator-only creation guards, internal planning caveats, user-story filler, or "known limitations" phrasing when concrete deferrals can be named.

## Step 5: Update ticket description docs

After a ticket is approved, update the proposed ticket description docs created earlier from the design/ticket-shaping work. Prefer existing docs in their original location; do not create duplicate approved-description trees unless no docs exist yet. If no docs exist, create them under:

```text
milestone-plan/context/create-tickets/provider-ticket-descriptions/tkt-01-short-slug.md
```

These approved provider issue bodies are supporting create-ticket artifacts, so they may live under `context/create-tickets/`. Actual ticket deliverables created later must live at the ticket directory root and be linked from ticket `index.md`.

Ticket description docs must be exactly the Markdown body that goes into Linear: no frontmatter, no metadata-only title heading, no suggested next command, no agent-only notes. The provider issue title lives outside the body, in the create command/status artifact.

Each description must be concise and provider-ready, using these sections in this order:

1. Goal
1. Scope
1. Acceptance criteria
1. Dependencies
1. Docs

Use Conventional Commit style for provider issue titles, e.g. `feat(domain): add first product path` or `test(domain): verify demo flow`.

Record each ticket title, description doc path, created provider issue ID, URL, and routing dir in `milestone-plan/AGENTS.md`. Do not create a separate ticket manifest unless the milestone memory becomes too large.

Use markdown links for docs/assets according to the thoughts root `AGENTS.md` host manifest. Prefer repo-local asset paths under the milestone-plan directory; do not link to local absolute screenshot paths.

Keep the title in the provider issue title, not as a body heading. Do not include suggested next commands in the provider description body; those belong in routing-only ticket `AGENTS.md` after the issue exists.

## Step 6: Execute and update repo routing

After all ticket description docs are approved one by one, create tickets one at a time:

1. For the next ticket, show the exact provider title and the exact Markdown body from the approved description doc, then ask: "Create this provider ticket now?" Do not create it until the human approves that specific ticket.
1. Create the provider ticket directly from the approved description doc in the repo. Do not create separate `/tmp` markdown bodies, hidden transformed copies, or frontmatter-stripped temp files. The approved doc must already be exactly the provider issue body.
1. Apply project/milestone/default fields. Created implementation/spec tickets must be assigned to the Linear project milestone and must not be children of the milestone planning ticket.
1. Add relations/blockers when the related issue already exists; otherwise record the pending relation and add it after the dependent ticket is created.
1. Repeat the approval-and-create loop for each remaining ticket.
1. Comment on the milestone planning ticket with markdown links to the created tickets, key relations, and thoughts docs/assets. For Linear, bare issue IDs may not auto-expand reliably, so use explicit markdown links built from manifest config for created tickets.
1. Move the milestone planning ticket to the configured review/status value when available after implementation/spec tickets are created; do not mark it `Done` until the human/project owner has reviewed the created ticket set.
1. Update project provider ticket log.
1. Update milestone planning status artifact.
1. Update milestone `AGENTS.md` and optional `milestone.md`.
1. Create ticket directories only after provider issue IDs exist using the provider key plus slug, e.g. `eng-0000-short-slug/`; do not add numeric ordering prefixes.
1. Write routing-only ticket `AGENTS.md` files.
1. Write a ticket-root `index.md` that links the provider issue, approved description doc, canonical docs, and next QRSPI command.
1. Comment on each created ticket with a fenced YAML `qrspi_result` block. Set `workspace` to that ticket directory, `artifact` to the approved ticket description doc, and `next.steps` to read QRSPI/question skills, read the ticket artifact, then start `q-question`.
1. Run `just sync-thoughts` when available.

## Response

Use fenced YAML `qrspi_result` blocks for all stage results. Required fields: `project`, `related_projects`, `stage`, `status`, `outcome` for complete results, `workspace`, `workspace_metadata`, `policy`, `summary`, `artifact`, `artifacts`, and structured `next.steps`.

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
    stage_completed: "Milestone tickets complete."
    key_decisions: "Next stage should start immediately: /done."
  artifact: "thoughts/.../created ticket/status artifact path"
  artifacts:
    - role: "primary"
      path: "thoughts/.../created ticket/status artifact path"
  next:
    steps:
      - action: "read_skill"
        param: ".pi/skills/qrspi-planning/SKILL.md"
      - action: "read_skill"
        param: ".pi/skills/done/SKILL.md"
      - action: "read_artifact"
        param: "thoughts/.../created ticket/status artifact path"
      - action: "start_stage"
        param: "done"
```
