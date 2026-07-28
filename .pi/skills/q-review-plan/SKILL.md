---
name: q-review-plan
description: Review planning artifacts before implementation.
---

# q-review-plan

Check design, optional product design, outline, plan, and code assumptions. Fix obvious document defects. If facts are missing, create focused research questions; otherwise recommend workspace preparation, or implementation for an existing review-follow-up workspace.

## Hermes completion

After durable work and verification, record the conclusion with `vamos hermes pi done --session "$VAMOS_PI_SESSION_ID" --outcome <outcome> --next <action> [--artifact thoughts/...] --summary $'1. ...\n2. ...\n3. ...'`. Use only the outcome and action vocabularies in `qrspi-planning`. Hermes owns process lifecycle and continuation; do not use tmux, a manager state file, retry policy, or hand-authored machine-routing output. Keep gateway settings and credentials host-local.


## Durable boundaries

Use `thoughts/...`-relative artifact references. The filesystem and plan-owned `.vamos/sessions/{pi,hermes}` artifacts are durable truth; database indexes are rebuildable. Do not expose credentials, gateway URLs, process IDs, or manager diagnostics in plan artifacts.
