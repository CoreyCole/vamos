---
name: q-verify
description: Generic QRSPI post-implementation verification stage. Use after implementation review and before final human approval to run project-defined verification, inspect UI/artifacts, update tests/docs, fix clear issues, and emit verify.md plus QRSPI YAML.
---

# QRSPI Verify

> Pipeline overview: `.pi/skills/qrspi-planning/SKILL.md`

`q-verify` is a generic agent stage after implementation review and before final human implementation approval. It validates reviewed code in the real project environment using a project-owned verification contract.

## Runtime YAML contract

Every completed verify stage must emit fenced YAML first, then one concise summary line.

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

Post-YAML summary format: `Verified: ... Fixed: ... Evidence: ... URL: https://<feature-slug>.<workspace-domain>/` If no fixes: `Verified: ... Fixed: none. Evidence: ... URL: ...`

For managed feature-branch workspaces, every verify result must include the exact feature workspace URL in the fenced YAML artifacts list as `role: "feature_url"` and in the post-YAML summary. The same URL must appear in `verify.md`. If UI/browser verification is required and the feature URL cannot be determined or shown to reach the child app, block instead of emitting an incomplete complete-result.

If verification finds a recoverable follow-up or needs to hand off context before another verify pass, use `status: "handoff"`, omit `outcome`, keep `workspace_metadata` with both workspace paths, write `verify.md` or a handoff artifact, and set `next.steps` steps to read `qrspi-planning`, read `q-resume`, read `verify.md` or the handoff, then start `/q-resume`. The runtime keeps the workflow on `verify` and can launch a fresh verify/resume child.

If blocked by failing verification that cannot be safely fixed or handed off for another verify pass, use `status: "blocked"`, omit `outcome`, keep `workspace_metadata` with both workspace paths, write `verify.md`, and set `next.steps` steps to read `qrspi-planning`, read `q-resume`, read `verify.md` or handoff, then start `/q-resume`. On success, `next.steps` should use ordered `step` children that present final implementation evidence for human review; runtime transition remains graph-authoritative.

## Inputs

Load only what is needed:

1. `.pi/skills/qrspi-planning/SKILL.md`.
1. Plan dir `AGENTS.md`, `design.md`, optional `design-product.md`, `outline.md`, `plan.md`, ADRs.
1. Final implementation handoff and implementation review artifact.
1. Project verification guide path supplied by workflow/user. If absent, first look for `docs/verify.md` and read it as the standard project verification entrypoint. If `docs/verify.md` is absent, look for exactly one obvious guide in repo docs such as `docs/qrspi-verify.md` or package docs. If none, block and ask for the guide path.
1. Files referenced by the guide and failing evidence.

The project guide is authoritative for project-specific commands, E2E tools, screenshot policy, canonical UI baseline, fixture setup, and artifact locations. This skill must not hardcode project names, commands, or paths.

## Process

1. Confirm current directory is the implementation workspace recorded by `/q-workspace` or handoff.
1. Read `docs/verify.md` when present, then read any detailed project guides it links for the touched surface.
1. Read the project verification guide and extract:
   - required commands
   - required E2E/user stories
   - required artifacts/screenshots/visual-review outputs
   - allowed fix scope
   - pass/fail criteria
1. Run required verification commands exactly as the guide specifies.
1. Inspect outputs, logs, screenshots, visual review artifacts, generated test diffs, and docs findings.
1. Fix clear problems directly when ALL are true:
   - root cause is proven by evidence
   - fix is local and low risk
   - fix is inside project-guide allowed scope or obviously required test/doc/verifier repair
   - fix does not weaken intended behavior, delete assertions, hide failures, or bypass real verification
1. For unclear product/UX changes, broad architecture changes, flaky infra, credential issues, or risky production behavior changes: do not guess; record blocker in `verify.md` and return blocked/needs_human as appropriate.
1. Re-run the smallest verification that proves each fix, then required final verification if practical.
1. Commit every fix applied during verify before requesting human manual testing. Use the repository's normal commit/stack workflow, include regenerated outputs, and keep the workspace clean except explicitly unrelated pre-existing changes. Do not ask the human to test uncommitted verification fixes.
1. For managed feature-branch workspaces, derive and validate the exact feature URL from project CLI/server output or `.vamos/run/workspace.env` plus the workspace domain. Record that URL in `verify.md`, include it in the fenced YAML `artifacts` list with `role: "feature_url"`, and repeat it in the post-YAML summary.
1. Before marking verification complete, prompt the user to manually test any running UI/workspace described by the project guide. Include the exact URL from the project CLI/server output and concise flows to inspect. Do not proceed to a complete `verify.md` until the user confirms manual testing passed; if the user cannot test or reports a problem, record `needs_human` or `blocked` with their findings.
1. Write `[plan_dir]/verify.md`.
1. Update `[plan_dir]/AGENTS.md` only for durable gotchas future sessions must load before handoffs.

## Allowed fixes

Default allowed:

- tests, test fixtures, test helpers, generated test outputs
- docs additions/updates/simplifications
- verification scripts/config owned by the project guide
- screenshot/visual-review artifacts and indexes
- small production fixes only when the issue is obvious, localized, and directly blocking verification

Never silently:

- weaken stories/assertions to pass
- mark a visual regression as accepted without guide-approved/human approval path
- edit unrelated code
- hide failures with sleeps, retries, broad ignores, or fallback behavior
- bypass real E2E when the guide requires it

## `verify.md` template

```markdown
---
date: [ISO]
researcher: [git_username]
git_commit: [current commit]
branch: [current branch]
repository: [repo]
stage: verify
plan_dir: thoughts/...
status: complete|blocked|needs_human
verification_guide: [path]
---

# Verify: [plan name]

## Summary
[Concise result and confidence.]

## Project Verification Contract
- Guide: `[path]`
- Required checks: [list]

## Commands Run
- `[command]` — pass|fail|skipped, evidence path/log summary

## Feature Workspace URL
- [exact feature URL, or `N/A` for non-UI/non-managed verification]

## E2E / UI Evidence
- [story/check] — [result, screenshot/artifact paths]

## Fixes Applied During Verify
- `[path]` — [what and why]

## Tests / Docs Updated
- `[path]` — [what changed]

## Remaining Risks / Human Decisions
- [None or exact blocker/question]

## Recommended Human Review Focus
- [what human should inspect]
```

## Rules

- `q-verify` runs after implementation review, before final human implementation approval.
- Verification evidence must be durable: prefer files under the plan dir or project-guide artifact locations.
- If real browser/agent/worker services are required, use the guide's auth and service-management instructions. Do not invent credentials or hand-configure auth.
- Commit verify-stage fixes before human testing so the running/manual-tested workspace corresponds to committed code.
- For managed feature branches, always include the exact feature workspace URL in `verify.md`, `qrspi_result.artifacts` (`role: "feature_url"`), and the final user summary.
- Keep final user summary short and evidence-focused.
- Do not create a final implementation-review `done.md`; successful verify routes to `human-review-implementation`, and terminal completion/done artifacts belong only after final human approval.
