# Pre-merge review Slack report run (2026-07-15)

Concrete q-hermes-manager example: direct-outline plan for adding GitHub-only pre-merge review requests to morning sync, followed by outline review, plan, plan review, workspace, five implementation handoffs, and implementation review launch.

## Useful patterns

- Direct-outline workflows may intentionally lack `design.md`; every child prompt should say not to block on absent optional design artifacts when `AGENTS.md`, `outline.md`, `plan.md`, and handoffs are sufficient.
- Preserve lead-engineer corrections as settled constraints in all later prompts. In this run: same-day pre-merge requests must render `(today)`, with one oldest-first list and no requested-today split.
- After every implementation handoff, launch `/q-resume` immediately when the result is `status: handoff`; do not pause because a checkpoint exists.
- Overwriting a stable `/tmp/q-hermes-manager-q-resume-prompt.md` is fine when each overwrite includes the newest full prior YAML, exact handoff path, exact `Next:` target, and corrected artifact path notes.
- Include the handoff's exact `Next:` text in the next prompt. Examples from this run: `Build aged, oldest-first review snapshots`, `Add combined Slack rendering and lint`, `morning cron integration`, `plural diagnostics command`.
- If a previous YAML artifact path is malformed or truncated, preserve the YAML verbatim but add a prompt note with the actual discovered path under the recorded plan/workspace, and instruct the child to emit corrected exact paths.
- For implementation-complete handoffs, route to q-review/q-review-implementation from the implementation workspace and pass the final handoff path, not the plan path alone.

## Pitfalls seen

- Truncated background completion notifications are not enough; read the full process log before extracting YAML.
- Prompt construction can accidentally drop a `reviews/...` path segment. Add an explicit correction note rather than silently rewriting the preserved YAML.
- Long implementation runs are easier to track by process ID + handoff artifact + `Next:` target than by prompt filename.
