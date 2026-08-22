---
name: q-outline
description: Turn approved design into an implementation outline.
---

# q-outline

Before writing, identify the plan directory. If the user has not named an existing plan directory, create a new timestamped plan directory under `thoughts/<owner>/plans/` and use it for the outline. Summarize slices, invariants, and exclusions and obtain approval unless Hermes has explicit authority to proceed. Then write `<plan-dir>/outline.md` and recommend planning review.

## Hermes completion

After durable work and verification, end with a normal final response that names the durable artifact and the smallest decision for Hermes or the lead. In a managed opaque-settlement run, do not call `vamos hermes pi done`, emit semantic `outcome`/`next` YAML, or launch a successor; `pi done` is an explicit operator recovery command only.


## Durable boundaries

Use `thoughts/...`-relative artifact references. The filesystem and plan-owned `.vamos/sessions/{pi,hermes}` artifacts are durable truth; database indexes are rebuildable. Do not expose credentials, gateway URLs, process IDs, or manager diagnostics in plan artifacts.
