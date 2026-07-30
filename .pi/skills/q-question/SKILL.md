---
name: q-question
description: Decompose a task into factual research questions.
---

# q-question

Interview the lead engineer about outcomes, scope, principles, and tradeoffs. In a managed Pi run, ask concise questions in your ordinary response, then stop with one fenced opaque YAML manager-message block that repeats the questions. Wait for Hermes to submit the manager's answer into the same live session. Do not invent alignment or treat waiting for that answer as a failure. Write the questions artifact and preserve the human alignment gate before Hermes decides whether to start research.

```yaml
manager_message:
  kind: question
  questions:
    - What outcome or scope decision is needed?
```

## Managed completion

After durable work and verification, end with a normal final response that names the durable artifact and the smallest human decision Hermes should inspect. For an intentional question stop, include the opaque YAML manager-message block above. Do not call `vamos hermes pi done`, write semantic `outcome`/`next` YAML, or launch a successor in a normal managed run: the stop-bound extension captures the actual final response and any YAML fence as opaque settlement evidence. If a response stops without YAML, Hermes still receives the raw message and must not infer an outcome. `pi done` is an explicit operator recovery command only.


## Durable boundaries

Use `thoughts/...`-relative artifact references. The filesystem and plan-owned `.vamos/sessions/{pi,hermes}` artifacts are durable truth; database indexes are rebuildable. Do not expose credentials, gateway URLs, process IDs, or manager diagnostics in plan artifacts.
