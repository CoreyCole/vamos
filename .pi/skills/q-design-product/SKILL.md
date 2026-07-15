---
name: q-design-product
description: Manually audits an approved technical design against product sources and writes `design-product.md`. Standalone opt-in helper; never part of automatic QRSPI routing.
disable-model-invocation: true
---

# Standalone Product Design Review

This helper runs only when the human explicitly invokes it. It is not a QRSPI stage, gate, prerequisite, or automatic recommendation. It may be invoked at any planning point; it must not choose or alter the caller's next QRSPI stage.

Be extremely concise everywhere: alignment interview, summaries, and `design-product.md`. Sacrifice grammar for concision. Optimize for scan speed, low reading overhead, cheap output.

Answer: **is the approved technical design aligned with what product wants?** Run a concise alignment interview, then write `design-product.md`. Do not write another technical design.

## When Invoked

0. **Load all relevant context before interviewing:**
   - Read `.pi/skills/qrspi-planning/SKILL.md`
   - Read `[plan_dir]/AGENTS.md`
   - Read `[plan_dir]/design.md`
   - Read all files in `[plan_dir]/questions/`
   - Read all files in `[plan_dir]/research/`
   - Read all files in `[plan_dir]/context/question/` especially question-stage product captures like `context/question/linear/issue.json`
   - Read all files in `[plan_dir]/context/research/`
   - Read all files in `[plan_dir]/context/design/`
   - Read all files in `[plan_dir]/context/design-product/` if any
   - Read all files in `[plan_dir]/adrs/`
   - Read all files in `[plan_dir]/prds/`
   - Read relevant captured external sources in `[plan_dir]/context/{linear,notion,external}/` if present
   - Read ticket/PRD/product sources referenced by those docs when accessible
1. **If a plan directory path, `design.md` path, or `design-product.md` path was provided**, resolve the plan directory from it, load the artifacts above, then begin. If the path is under `[parent_plan_dir]/reviews/*/`, that timestamped review directory is the plan directory and all product-design artifacts must be written there.
1. **If no parameters**, respond:

```text
I'll create a product design audit from the approved technical design.

Please provide the plan directory path or design doc path:
e.g. `/q-design-product thoughts/[git_username]/plans/2026-03-29_12-26-32_feature-name`
or `/q-design-product thoughts/[git_username]/plans/2026-03-29_12-26-32_feature-name/design.md`
```

Then wait for input.

## Gate Tree

```text
Missing product source OR approved technical design?
  YES -> Blocked; ask for missing required input; do not write design-product.md
  NO  -> build coverage matrix comparing product source to design.md

Any explicit PRD requirement uncovered by design.md or ambiguous enough to change implementation?
  YES -> Blocked unless engineer explicitly accepts non-goal or override
  NO  -> continue

Any hidden complexity / E2E edge / demo implication unresolved?
  YES -> Blocked unless accepted non-goal or explicit override
  NO  -> Pass

All gaps accepted as non-goals or explicitly overridden?
  YES -> Pass with accepted non-goals
```

Allowed row verdicts: `Covered`, `Gap`, `Ambiguous`, `Accepted non-goal`.
Document verdicts: `Pass`, `Blocked`, `Pass with accepted non-goals`.

## Source Capture

If Linear/Notion/Slack/web/Figma/product sources are provided or referenced and no captured copy exists, capture them before interviewing. Question-stage captures under `context/question/{linear,notion,external}/` count as captured copies; use them instead of blocking or duplicating.

| Source | Store |
|---|---|
| PRD / inline product text | `prds/[source-slug].md` |
| Linear ticket/comment/link | `context/linear/[ticket-id].md` |
| Notion doc/link | `context/notion/[page-title-or-id].md` |
| Slack thread, web URL, Figma summary, other external source | `context/external/[source-slug].md` |

Do not create empty context dirs. Every captured file needs frontmatter: `source_type`, `source_url` or `source_id`, `fetched_at`, `captured_from`.

## Alignment Interview

Before writing `design-product.md`, read all relevant captured/referenced context, then grill engineer until shared understanding is explicit.

Rules: be extremely concise; sacrifice grammar for concision; ask one direct question at a time; include recommendation and why; investigate codebase-answerable questions yourself; resolve product intent before edge cases; track decisions, assumptions, deferred research, risks in terse bullets; do not write until goals, scope, constraints, source coverage, non-goals, risks, next steps are clear.

Use this format:

```text
Decision branch: [short branch]
What I found: [only if investigated]
Recommendation: [recommended answer and why]
Question: [one direct confirm/reject/adjust question]
```

## Process

1. **Verify required inputs**: product source and approved `design.md`. If either missing, stop and ask.
1. **Capture external product sources** using Source Capture unless already present.
1. **Read all relevant context** in `prds/`, `context/question/`, `context/*/`, `design.md`, ADRs, questions/research, and referenced docs/files before interviewing.
1. **Extract product requirements and design claims**: product intent, proposed behavior, affected users, demos, rollout assumptions, non-goals.
1. **Run alignment interview**. One question at a time until aligned.
1. **Compare design against product requirements**. Mark coverage mechanically in matrix.
1. **Inspect code only to verify current behavior, constraints, or feasibility.** Write notes under `context/design-product/` only when writing a file.
1. **Surface gaps**: user end-state, demo assumptions, E2E paths, roles/tenants, data visibility/migrations, empty/loading/error states, rollout/rollback/support.
1. **Surface product/engineering tradeoffs**: product behavior adding complexity, engineering simplicity constraining product behavior, and whether the tradeoff is accepted.
1. **Draft `design-product.md` only after alignment resolves.**
1. If `Blocked`, make each blocker actionable: product decision, design change, accepted non-goal, or override.
1. Present summary; if user gives feedback, update doc and re-emit response shape.
1. If approved findings are durable, update `[plan_dir]/AGENTS.md` with only short invariants/pointers.
1. Immediately before writing/updating `design-product.md`, run `~/dotfiles/spec_metadata.sh` and use it for frontmatter.

## Output Template

Write to `[plan_dir]/design-product.md`.

Output artifact style: 1-2 pages, product/stakeholder readable, terse, declarative end-state language. Sacrifice grammar for concision. This is a product decision memo, not a second PRD or implementation plan.

```markdown
---
date: [ISO datetime with timezone]
researcher: [git_username]
last_updated_by: [git_username]
git_commit: [current commit hash]
branch: [current branch]
repository: [repository name]
stage: design-product
ticket: "[ticket reference if any]"
plan_dir: "thoughts/[git_username]/plans/[timestamp]_[plan-name]"
verdict: [Pass|Blocked|Pass with accepted non-goals]
---

# Product Design: [Feature Name]

## Product Outcomes
- [Outcome users/stakeholders get]
- [Operational outcome]
- [Approval/gate outcome if relevant]

## User-Facing Behavior
- [End-state behavior users see]
- [Changed workflows, permissions, data visibility, or support/admin behavior]
- Edge behavior: [important empty/loading/error/permission/data-state behavior]

## Product Alignment

| Product expectation | End-state behavior | Alignment |
|---|---|---|
| [plain-language product expectation] | [what users/system will do] | [Aligned/Accepted non-goal/Needs decision + note] |

## Product / Engineering Tradeoffs
- [Product behavior chosen]: [engineering simplicity/cost tradeoff, or why added complexity is justified]

## Decisions / Non-Goals
- Decision: [product behavior decision]
- Non-goal: [accepted non-goal]
- Assumption: [assumption]

## Demo / Rollout Notes
- Demo path: [happy path]
- Setup/data: [required data or setup]
- Risk: [demo/rollout/support risk]

## Blocking Gaps
- [None, or specific product decision/design change/override required]
```

## Response

Return the artifact path, verdict, critical findings, and blockers. Do not emit a `qrspi_result`, select a next stage, or modify QRSPI routing. The human decides what happens next.

## Rules

- Grill before writing. Do not create `design-product.md` until agent and engineer are aligned or engineer explicitly asks for a draft.
- Always require both product source and approved `design.md`.
- Do not treat `design.md` as the PRD.
- Treat question-stage captures such as `context/question/linear/issue.json` as valid product source inputs.
- Missing PRD/ticket/product source or missing design blocks document creation.
- `Gap` or `Ambiguous` rows block unless explicitly accepted as non-goals or overridden.
- Do not bury blockers in prose; surface them in Source Coverage, Verdict, and Blocking Gaps.
- Do not write technical signatures, package structures, or implementation slices. That belongs in `/q-outline`.
- Do not duplicate the PRD; translate Notion/product intent into clear end-state behavior.
- Do not write before/after migration narration in the artifact; state the approved product end state.
- Do not cite internal capture paths as a standalone Source column; if source traceability is useful, mention Notion/Linear/Figma by human name inside Alignment notes.
- Do record engineering simplicity as a product tradeoff when it changes user-facing behavior, rollout, support, or future flexibility.
- Every captured source file needs frontmatter pointing back to the original source document/link/ID.
- Every requirement row needs a captured source path when available; otherwise cite original source and explain why not captured.
- Every blocker needs a next decision or design change.
- Present to user before finalizing.
- Completion responses must be the fenced YAML `qrspi_result` block required by the runtime contract, followed by the mandatory concise human summary.
