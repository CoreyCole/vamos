---
name: e2e-image-review
description: Compare Vamos story E2E actual screenshots against main semantic goldens using story rules, assertions, HTML snapshots, and QRSPI plan intent. Use when running or reviewing `vamos e2e review` artifacts.
---

# E2E Image Review

Inputs:

- Story `.story.md` and business rules.
- Generated test assertions.
- Main golden screenshot for story/scenario/viewport.
- Workspace actual screenshot.
- HTML snapshot or accessibility text when available.
- QRSPI plan/design/outline when `--plan-dir` is provided.

Classify each difference as one of:

- `pass`
- `intentional improvement`
- `selector drift`
- `visual regression`
- `story outdated`
- `needs human decision`

Rules:

- Do not use pixel matching as the sole criterion.
- Do not approve missing required regions, hidden primary actions, auth failures, or error pages.
- Treat story/business rules as source of truth.
- If product intent and screenshot conflict, choose `needs human decision`.
