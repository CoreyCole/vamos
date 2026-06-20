---
name: q-manager-create-manifest
description: Interactively create or update a project's docs/q-manager.md manager manifest.
---

# q-manager-create-manifest

Interview the user one question at a time, then write or update `docs/q-manager.md`.

## Process

1. Read project `AGENTS.md` and existing `docs/q-manager.md` if present.
1. Ask one question at a time about:
   - manager mission
   - authority boundaries
   - human escalation preferences
   - workspace/copy conventions
   - visible child-session expectations
   - verification and merge habits
   - never-decide categories
   - deterministic reload sources
1. Keep host secrets and machine-local private paths out unless this project explicitly owns them.
1. Write concise Markdown to `docs/q-manager.md`.
1. Summarize changed sections.

## Required manifest sections

- Manager mission
- Authority boundaries
- QRSPI policy and graph authority
- Human escalation preferences
- Workspace/copy boundary
- Visible child-session rule
- Session metadata boundary
- Deterministic reload sources
- Verification and merge habits
