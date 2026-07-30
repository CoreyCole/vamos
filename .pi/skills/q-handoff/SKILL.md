---
name: q-handoff
description: Create durable QRSPI continuity handoffs.
---

# q-handoff

Write concise `Done:` and `Next:` recovery information, slice self-verification evidence, relevant artifact paths, and workspace/branch identity when applicable. Do not include ephemeral process state. Hermes decides whether another child resumes an unfinished implementation plan or a later stage begins.

## Hermes completion

After durable work and verification, end with a normal final response that names the durable handoff artifact and the smallest decision for Hermes or the lead. In a managed opaque-settlement run, do not call `vamos hermes pi done`, emit semantic `outcome`/`next` YAML, or launch a successor; `pi done` is an explicit operator recovery command only.

## Durable boundaries

Use `thoughts/...`-relative artifact references. The filesystem and plan-owned `.vamos/sessions/{pi,hermes}` artifacts are durable truth; database indexes are rebuildable. Do not expose credentials, gateway URLs, process IDs, or manager diagnostics in plan artifacts.
