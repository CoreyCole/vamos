---
name: q-workspace
description: Prepare the safe implementation workspace.
---

# q-workspace

Use a fresh filesystem copy, never a git worktree. Verify base/stack safety and record plan and implementation paths in plan memory. For implementation-review follow-up work, reuse the reviewed workspace/head. Recommend `implement` only when safe.

## Hermes completion

After durable work and verification, end with a normal final response that names the durable artifact and the smallest decision for Hermes or the lead. In a managed opaque-settlement run, do not call `vamos hermes pi done`, emit semantic `outcome`/`next` YAML, or launch a successor; `pi done` is an explicit operator recovery command only.


## Durable boundaries

Use `thoughts/...`-relative artifact references. The filesystem and plan-owned `.vamos/sessions/{pi,hermes}` artifacts are durable truth; database indexes are rebuildable. Do not expose credentials, gateway URLs, process IDs, or manager diagnostics in plan artifacts.
