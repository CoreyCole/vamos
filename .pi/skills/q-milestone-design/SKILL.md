---
name: q-milestone-design
description: Design a milestone and proposed ticket set.
---

# q-milestone-design

Write milestone ownership, outcomes, gap map, dependencies, and ticket proposals. Stop for human alignment before ticket creation.

## Hermes completion

After durable work and verification, record the conclusion with `vamos hermes pi done --session "$PI_SESSION_ID" --outcome <outcome> --next <action> [--artifact thoughts/...] --summary $'1. ...\n2. ...\n3. ...'`. Use only the outcome and action vocabularies in `qrspi-planning`. Hermes owns process lifecycle and continuation; do not use tmux, a manager state file, retry policy, or hand-authored machine-routing output. Keep gateway settings and credentials host-local.


## Durable boundaries

Use `thoughts/...`-relative artifact references. The filesystem and plan-owned `.vamos/sessions/{pi,hermes}` artifacts are durable truth; database indexes are rebuildable. Do not expose credentials, gateway URLs, process IDs, or manager diagnostics in plan artifacts.
