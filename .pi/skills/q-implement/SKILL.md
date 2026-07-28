---
name: q-implement
description: Execute exactly one unchecked implementation item.
---

# q-implement

Work only in the prepared implementation workspace. Read affected code, implement and self-verify one item, update plan checkboxes, commit according to repository policy, and write a durable handoff. Until every implementation slice is checked, finish with `--outcome handoff --next implement` so the next child continues implementation. Only the final implementation child uses `--outcome complete --next review`.

## Hermes completion

After durable work and verification, record the conclusion with `vamos hermes pi done --session "$PI_SESSION_ID" --outcome <outcome> --next <action> [--artifact thoughts/...] --summary $'1. ...\n2. ...\n3. ...'`. For a non-final slice, use `--outcome handoff --next implement`; do not route a slice to review or verify. Use only the outcome and action vocabularies in `qrspi-planning`. Hermes owns process lifecycle and continuation; do not use tmux, a manager state file, retry policy, or hand-authored machine-routing output. Keep gateway settings and credentials host-local.

## Durable boundaries

Use `thoughts/...`-relative artifact references. The filesystem and plan-owned `.vamos/sessions/{pi,hermes}` artifacts are durable truth; database indexes are rebuildable. Do not expose credentials, gateway URLs, process IDs, or manager diagnostics in plan artifacts.
