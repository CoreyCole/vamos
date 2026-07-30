---
name: q-implement
description: Execute exactly one unchecked implementation item.
---

# q-implement

Work only in the prepared implementation workspace. Read affected code, implement and self-verify one item, update plan checkboxes, commit according to repository policy, and write a durable handoff using the `q-handoff` pattern. Hermes decides whether another child resumes the work or a later stage begins.

## Hermes completion

After durable work and verification, end with a normal final response that names the durable artifact and the smallest decision for Hermes or the lead. In a managed opaque-settlement run, do not call `vamos hermes pi done`, emit semantic `outcome`/`next` YAML, or launch a successor; `pi done` is an explicit operator recovery command only.

## Durable boundaries

Use `thoughts/...`-relative artifact references. The filesystem and plan-owned `.vamos/sessions/{pi,hermes}` artifacts are durable truth; database indexes are rebuildable. Do not expose credentials, gateway URLs, process IDs, or manager diagnostics in plan artifacts.
