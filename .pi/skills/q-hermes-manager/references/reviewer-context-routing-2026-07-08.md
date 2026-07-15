# Reviewer context routing QRSPI run, 2026-07-08

Use this as a compact example when q-hermes-manager is orchestrating a delegated QRSPI workflow with repeated implementation handoffs, cross-repo work, and mid-run lead-engineer corrections.

## Durable orchestration lessons

- Treat user messages attached to background-process completion notifications as active steering before launching the next stage.
- Read the full process log with the process tool before constructing the next prompt; the notification tail may omit the opening fence or important YAML fields.
- Preserve the full prior `qrspi_result` YAML verbatim in every next prompt, including implementation handoff results.
- For q-question/q-research/q-design/q-outline/q-plan/review/workspace stages, launch the next graph-safe stage immediately after parsing a valid result.
- For implementation `status: handoff`, launch `/q-resume` immediately in a fresh background Pi process. Do not pause merely because a handoff exists.
- Make the next prompt name the concrete target from the prior handoff, but still require the child to read the handoff and plan rather than trusting the orchestration prose.
- In cross-repo implementation checkpoints, instruct the child to perform checkout safety checks and preserve unrelated work before touching the related repo.

## Domain lesson from this run

Reviewer assignment design should distinguish ticket existence from reviewer choice:

- A post-merge ticket is skipped only when every PR in the commit block was approved.
- Mixed approved/unapproved blocks still create a post-merge ticket.
- In mixed blocks, the approver/commenter/context-rich reviewer should be preferred over balanced-load fallback.
- Comment/request activity needs a freshness window; this run used 30 days. Approvals and explicit reviewer markers are durable context.
- Pre-merge stack continuity should use PR base/head prior-PR marker lookup, not head-ref-as-stack identity.

## Prompting pattern

When carrying a human correction into the next stage, add it above the preserved YAML as settled alignment:

```text
Incorporate this fresh lead-engineer clarification as settled alignment:
- [correction]

IMPORTANT previous stage result:

```yaml
[full previous qrspi_result]
```
```

For implementation resumes, also name the immediate next handoff target:

```text
Next target from prior handoff: [exact Done/Next target]
Implement only one unchecked checkpoint, then emit required qrspi_result.
If this emits status: handoff, route next.steps to q-resume with the exact handoff path.
```
