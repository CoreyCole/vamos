---
name: q-manager-note
description: Records a private operational note for the active q-manager parent without forwarding it to child workers. User-invoked control-plane helper for routing, timing, workspace, model, and safety reminders.
disable-model-invocation: true
---

# q-manager-note

Capture the invocation text as a parent-manager-only operational note.

## Rules

1. Do not forward the note through `steer-child`, parser reprompts, child prompts, previous-result YAML, or pane input.
1. Do not copy the note into QRSPI artifacts, plan memory, Linear/GitHub comments, or child-visible handoffs.
1. Do not interrupt an active child merely because the note arrived.
1. Apply the note only to parent orchestration: when to pause, which canonical stage to launch, workspace isolation, model selection, verification, or escalation.
1. Never use a note to bypass the canonical QRSPI graph or a human/safety gate.
1. If satisfying the note requires child knowledge rather than parent orchestration, ask the human to reissue that portion as normal child-safe feedback; do not silently leak or paraphrase it to a child.

## Persistence

When the active q-manager `state_file` is known, append the note with a timestamp and `active` status to:

```text
<dirname(state_file)>/manager-notes.md
```

This file is local, ephemeral control-plane state. Never add it to the repository or cite it in durable `qrspi_result` YAML. If no active state file is known, retain the note only in the parent conversation and say persistence is unavailable.

Before each manager transition, read active notes and enforce them before running a command that may auto-launch the next child. Mark a note `fulfilled` or `obsolete` only after observing the required condition.

## Response

Acknowledge concisely with:

```text
Manager note recorded privately. It will not be forwarded to child workers.
Applies at: <next relevant manager transition>
```
