---
name: q-review
description: Router for QRSPI LLM reviews. Use for reviewing design, product design, outline, plan, or completed implementation artifacts; loads q-review-plan before code exists and q-review-implementation after code has been written.
---

# QRSPI Review Router

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

`status` is lifecycle. `outcome` selects the graph branch. `next.steps` is an ordered instruction block for the next agent: read `qrspi-planning`, read the next stage skill, read the appropriate artifact, then start the next stage immediately unless a named human/safety gate blocks. Runtime transitions remain graph-authoritative and may validate/rewrite the steps. Complete results must include `outcome`. Review stages must use explicit node IDs (`review-outline`, `review-plan`, or `review-implementation`), never `review`.

Every `/q-review` session starts by reading `.pi/skills/qrspi-planning/SKILL.md`, then this router, then the selected focused review skill. After route selection, immediately run that focused review. Do not answer “ready to proceed.”

`/q-review` is the stable entry point for QRSPI LLM review. It does not contain the review workflow itself. It resolves whether code has been written, then loads exactly one focused review skill:

- `.pi/skills/q-review-plan/SKILL.md` for pre-implementation planning review after `outline.md` and `plan.md`; it reviews and may edit `design.md` and optional `design-product.md` as supporting context.
- `.pi/skills/q-review-implementation/SKILL.md` for post-implementation code review after `/q-implement` completes.

## When Invoked

1. Read `.pi/skills/qrspi-planning/SKILL.md`.
1. Resolve the input:
   - `outline.md` or `plan.md` path → planning review.
   - Implement-complete handoff path under `[plan_dir]/handoffs/` → implementation review.
   - Plan directory path → inspect artifacts to choose mode.
   - Canonical review artifact path under `[plan_dir]/reviews/*/review.md` → use the `review_mode` frontmatter if present.
1. If no input was provided, respond:

```text
I'll run a QRSPI review and route it based on whether implementation code exists.

Please provide one of:
- an outline path, e.g. `/q-review thoughts/[git_username]/plans/.../outline.md`
- a plan path, e.g. `/q-review thoughts/[git_username]/plans/.../plan.md`
- an implement-complete handoff path, e.g. `/q-review thoughts/[git_username]/plans/.../handoffs/YYYY-MM-DD_HH-MM-SS_implement-handoff.md`
- or a plan directory path, e.g. `/q-review thoughts/[git_username]/plans/YYYY-MM-DD_HH-MM-SS_plan-name`
```

Then wait for input.

## Mode Resolution

### Planning review

Choose planning review when:

- the input is `outline.md` or `plan.md`
- the user asks to review the outline, plan, or planning docs
- the input is a plan directory that has no implement-complete handoff
- the input is a plan directory with `plan.md` but implementation is not complete

After resolving planning review, read and follow:

```text
.pi/skills/q-review-plan/SKILL.md
```

### Implementation review

Choose implementation review when:

- the input is an implement-complete handoff
- the input is a plan directory with a complete implement handoff in `[plan_dir]/handoffs/`
- the user explicitly asks to review completed code or implementation

A complete implement handoff is the final `/q-implement` handoff that points to `/q-review`, has `next_stage: review`, or otherwise states implementation is complete.

If both modes could apply for a plan directory, prefer implementation review only when a complete implement handoff is unambiguous. Otherwise run planning review.

After resolving implementation review, read and follow:

```text
.pi/skills/q-review-implementation/SKILL.md
```

## Rules

- Do not run both review modes in one invocation.
- Do not keep using this router after mode selection; load the focused skill and follow it.
- `/q-review` does not read or delegate same-workspace routing to `/q-workspace`; `q-review-plan` owns the decision. If the reviewed `plan.md` is under another plan's `reviews/` tree or the human says to implement in the current workspace, `/q-review` must skip `/q-workspace` and emit `ready-for-implement` directly.
- Planning review edits planning documents directly when findings are clear, including `design-product.md` when present. After a successful normal parent-plan `plan.md` review, the next stage is `/q-workspace [plan.md]`, not `/q-implement`; the YAML and post-YAML summary must say to start `/q-workspace` immediately, not “ready to proceed.”
- Review-plan-dir exception: for any reviewed `plan.md` whose plan directory is itself under another plan's `reviews/` tree (`.../reviews/*/plan.md`), do not route to `/q-workspace` unless the human explicitly asks for a fresh copy. This includes `*_implementation-review` follow-ups and review-fix/follow-up plan dirs with other names. The current repo root is the existing implementation workspace. The completion YAML must route directly to `/q-implement [plan.md]`, omit top-level `workspace`, set `plan_workspace` to the review-dir plan workspace, set `implementation_workspace` to the current/original implementation workspace, and say implementation must continue in this workspace. Do not create a fresh copy, do not reset to trunk/main, and do not imply implementation should happen anywhere except the reviewed workspace.
- Same-workspace override: if the human says implementation will happen in the current workspace, skip `/q-workspace` even for a plan path that does not match `.../reviews/*/plan.md`. Treat the current repo root as `implementation_workspace`, set outcome `ready-for-implement`, and route directly to `/q-implement [plan.md]`. `/q-workspace` is only for parent plans that need a new implementation copy; it must not create another workspace around an existing implementation workspace.
- Implementation review reviews code, applies only straightforward code fixes directly, and creates a review-directory QRSPI plan for deeper follow-up work.
- The focused review must summarize the current design/implementation and its alignment with PRDs, tickets, brainstormed requirements, approved QRSPI constraints, and relevant project guidance in `review.md`.
- The focused review must load repository guidance for the actual paths under review before judging or editing: enumerate the planning/implementation file paths it intends to change or has changed, read those files (or the nearest concrete files in each touched directory) so path-scoped `AGENTS.md` context is loaded, and explicitly check the outline/plan/implementation against that guidance.
- Before delegating, call `subagent({ action: "list" })` and use only executable, non-disabled agents returned by discovery. Lane Markdown files are embedded prompts, not independently registered agents. If `scout` or `reviewer` is unavailable, record the lane system as unavailable and stop for setup rather than pretending lanes ran.
- Lane selection is semantic, not regex-based: first delegate `q-review-lane-selector.md` to a fresh `scout`, persist `[review_dir]/context/lane-selection.md`, and have the main reviewer validate its evidence, known lane IDs, mandatory lanes, and lane budget before launching reviewers.
- The focused review must delegate the selected lanes through the `subagent` tool. Each child writes one report under `[review_dir]/context/lanes/`; the main reviewer reads all reports, verifies their evidence, and synthesizes them. Never claim a lane ran without a persisted report.
- Planning review always includes intent-fit, simplicity, and project-guidance lanes. Implementation review always includes correctness, simplicity, and project-guidance lanes. The project-guidance reviewer owns discovery and enforcement of applicable root/path-scoped `AGENTS.md`, `.agents/rules/`, `.agents/skills/`, and nearby package docs.
- There is no fixed lane maximum. Add every materially relevant specialist with a distinct review question. The selector must assign exclusive ownership and remove overlapping lanes rather than suppressing independent high-risk reviews.
- The simplicity lane seeks eliminations, collapses, and narrower solutions while preserving complete coverage of declared PRD, ADR, design, and repository requirements.
- The focused review must run/use the project-guidance lane for relevant `AGENTS.md`, `.agents/rules/`, `.agents/skills/`, and nearby package docs based on the reviewed/changed code paths.
- The focused review must run/use the docs-health lane to decide whether relevant docs can be corrected, simplified, or made more concise.
- The main review artifact must disposition every lane finding as fixed, ignored, research/follow-up, or human judgment and give a concise evidence-based rationale. Never silently drop a child finding.
- The post-YAML user summary for review normally uses `Found: ... Fixed: ...`. For successful `review-outline`, append `Next: start /q-plan now.` For successful normal parent-plan `review-plan`, append `Next: start /q-workspace now.` For successful review-plan-dir or same-workspace `review-plan`, append `Next: start /q-implement now.` If clean, use the matching normal/review-dir next stage. Caveman clear. Few words. Most important words only.
- The focused review must preserve any conflicting relevant guidance from docs, `AGENTS.md`, `.agents/rules/`, `.agents/skills/` as `IMPORTANT: needs human attention` with exact source refs and the human decision needed.
- Always return the canonical review artifact path produced by the focused skill.
