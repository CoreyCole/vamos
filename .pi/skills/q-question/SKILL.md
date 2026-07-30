---
name: q-question
description: Decompose a task into factual research questions.
---

# q-question

Interview the lead engineer about outcomes, scope, principles, and tradeoffs. In a managed Pi run, ask concise questions in your ordinary response, then wait for Hermes to submit the manager's answer into the same live session. Do not invent alignment or treat waiting for that answer as a failure. Write the questions artifact and preserve the human alignment gate before Hermes decides whether to start research.

## Managed completion

After durable work and verification, end with a normal final response that names the durable artifact and the smallest human decision Hermes should inspect. Do not call `vamos hermes pi done`, write semantic `outcome`/`next` YAML, or launch a successor in a normal managed run: the stop-bound extension captures the actual final response as opaque settlement evidence. `pi done` is an explicit operator recovery command only.


## Durable boundaries

Use `thoughts/...`-relative artifact references. The filesystem and plan-owned `.vamos/sessions/{pi,hermes}` artifacts are durable truth; database indexes are rebuildable. Do not expose credentials, gateway URLs, process IDs, or manager diagnostics in plan artifacts.
