---
name: q-resume
description: Resume a QRSPI handoff in its recorded workspace.
---

# q-resume

Read the handoff, plan memory, current plan status, and only needed artifacts. Confirm the workspace is safe, then self-verify the handoff's completed slice with its targeted tests and relevant diff/check evidence before performing one bounded pending activity. Record that evidence in the replacement handoff or final completion. If implementation slices remain unchecked, continue implementation rather than routing a slice to review or verify.

## Hermes completion

After durable work and verification, record the conclusion with `vamos hermes pi done --session "$PI_SESSION_ID" --outcome <outcome> --next <action> [--artifact thoughts/...] --summary $'1. ...\n2. ...\n3. ...'`. For an unfinished implementation plan, use `--outcome handoff --next implement`; only the final implementation completion uses `--outcome complete --next review`. Use only the outcome and action vocabularies in `qrspi-planning`. Hermes owns process lifecycle and continuation; do not use tmux, a manager state file, retry policy, or hand-authored machine-routing output. Keep gateway settings and credentials host-local.

## Durable boundaries

Use `thoughts/...`-relative artifact references. The filesystem and plan-owned `.vamos/sessions/{pi,hermes}` artifacts are durable truth; database indexes are rebuildable. Do not expose credentials, gateway URLs, process IDs, or manager diagnostics in plan artifacts.
