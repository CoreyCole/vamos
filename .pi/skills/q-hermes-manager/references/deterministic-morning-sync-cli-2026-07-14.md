# Deterministic morning sync CLI QRSPI run, 2026-07-14

Session-specific lessons from a long Hermes-managed QRSPI run that went outline review → plan → plan review → workspace → seven implementation handoffs → implementation review.

## What worked

- Auto-advance implementation handoffs immediately. Each `status: handoff` result was valid continuation state, not a human gate.
- Poll/read the full process log after each completion. User-delivered snippets were repeatedly truncated mid-YAML, while the full log had coherent top-level `qrspi_result:` blocks.
- Preserve the full prior YAML verbatim in each next prompt, especially `workspace_metadata.implementation_workspace`, Graphite branch metadata, and the exact newest handoff path.
- Name the immediate next target from the handoff in the next prompt. Examples: `gate sync on verification and normalize artifacts`, `Exact 10:30 Pacific landed-work collection`, `Slack rendering/lint`, `selective document publication`, `compose non-posting morning cron packet`.
- Keep `cwd` on the implementation workspace after `/q-workspace`; only planning/review-before-workspace stages ran from the source checkout.

## Pitfalls observed

- Completion snippets often omit the opening YAML and first fields. Do not route from the snippet alone; use the process log tool and validate `stage`, `status`, `artifact`, and `next.steps`.
- Direct-outline plans may intentionally omit `design.md`. Every implementation/review/resume prompt should explicitly say not to block on absent optional design artifacts when the plan memory, outline, plan, and handoffs are sufficient.
- For repeated `/q-resume` loops, reusing a stable prompt path is fine only after overwriting it with the newest full YAML and newest handoff target. The durable identifier is the process ID plus handoff artifact, not the prompt filename.
- Full-suite failures can be known unrelated fixture failures. Preserve them precisely in prompts/reviews, but do not convert them into a blocker if focused tests/lint/vet/build evidence is sufficient and the child result marks the stage graph-safe.

## Good manager status shape

Use terse manager state after each launch:

```text
Completed: q-resume -> handoff
Artifact: thoughts/.../handoffs/YYYY-MM-DD_HH-MM-SS_implement-handoff.md
Done: [behavior] (N/7).

Started next: q-resume
Next target: [exact target from prior handoff]
Process: proc_...
Cwd: /absolute/implementation/workspace
Prompt: /tmp/q-hermes-manager-q-resume-prompt.md
```

For final implementation completion, route to `review-implementation` with the exact final handoff path rather than another resume.
