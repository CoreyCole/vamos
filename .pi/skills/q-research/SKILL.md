---
name: q-research
description: Answer plan research questions with codebase facts.
---

# q-research

Read the plan guidance and question artifacts. Write factual findings only; recommend another research pass when material code-answerable questions remain, otherwise recommend `design`.

## Hermes completion

After durable work and verification, record the conclusion with `vamos hermes pi done --session "$VAMOS_PI_SESSION_ID" --outcome <outcome> --next <action> [--artifact thoughts/...] --summary $'1. ...\n2. ...\n3. ...'`. Use only the outcome and action vocabularies in `qrspi-planning`. Hermes owns process lifecycle and continuation; do not use tmux, a manager state file, retry policy, or hand-authored machine-routing output. Keep gateway settings and credentials host-local.


## Durable boundaries

Use `thoughts/...`-relative artifact references. The filesystem and plan-owned `.vamos/sessions/{pi,hermes}` artifacts are durable truth; database indexes are rebuildable. Do not expose credentials, gateway URLs, process IDs, or manager diagnostics in plan artifacts.
