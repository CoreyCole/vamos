---
name: q-resume
description: Resume a QRSPI handoff in its recorded workspace.
---

# q-resume

Read the handoff, plan memory, current plan status, and only needed artifacts. Confirm the workspace is safe, then self-verify the handoff's completed slice with its targeted tests and relevant diff/check evidence before performing one bounded pending activity. Record that evidence in the replacement handoff or final completion. If implementation slices remain unchecked, continue implementation rather than routing a slice to review or verify.

## Hermes completion

After durable work and verification, end with a normal final response that names the durable artifact and the smallest decision for Hermes or the lead. In a managed opaque-settlement run, do not call `vamos hermes pi done`, emit semantic `outcome`/`next` YAML, or launch a successor; `pi done` is an explicit operator recovery command only. Do not use tmux, a manager state file, retry policy, or hand-authored machine-routing output. Keep gateway settings and credentials host-local.

## Durable boundaries

Use `thoughts/...`-relative artifact references. The filesystem and plan-owned `.vamos/sessions/{pi,hermes}` artifacts are durable truth; database indexes are rebuildable. Do not expose credentials, gateway URLs, process IDs, or manager diagnostics in plan artifacts.
