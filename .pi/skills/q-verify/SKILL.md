---
name: q-verify
description: Perform post-implementation verification.
---

# q-verify

Run project-defined checks, inspect artifacts/UI as appropriate, fix clear bounded defects, write `verify.md`, and stop for final human approval when evidence is ready.

## Hermes completion

After durable work and verification, end with a normal final response that names the durable artifact and the smallest decision for Hermes or the lead. In a managed opaque-settlement run, do not call `vamos hermes pi done`, emit semantic `outcome`/`next` YAML, or launch a successor; `pi done` is an explicit operator recovery command only.


## Durable boundaries

Use `thoughts/...`-relative artifact references. The filesystem and plan-owned `.vamos/sessions/{pi,hermes}` artifacts are durable truth; database indexes are rebuildable. Do not expose credentials, gateway URLs, process IDs, or manager diagnostics in plan artifacts.
