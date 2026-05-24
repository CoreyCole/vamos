# Feature: Durable session chat

## User story

As a workspace user, I want Agent Chat transcripts and Pi-produced artifacts to survive reloads and reopened sessions.

## Business rules

- QRSPI workspace chat uses the real browser, server route, Temporal workflow, TS Pi activity, persisted callbacks, and replayed transcript.
- The Pi verification prompt may update only the stable E2E review artifact.
- Freeform durability is verified from seeded fixtures, not a live Pi send.
- Fixture paths that contain `creative-mode-agent` are synthetic E2E fixture data; public prose should use `<owner>` placeholders.

## Scenario: QRSPI plan workspace chat updates verification artifact through Pi and Temporal

### Given

- I am authenticated as "playwright@localhost".
- I open plan workspace "thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go".
- I open workspace chat.
- I remember file hash "thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/context/implement/e2e-pi-plan-docs-review.md".

### When

- I send Pi docs review prompt "VAMOS_E2E_PLAN_DOCS_REVIEW_OK" for "thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/context/implement/e2e-pi-plan-docs-review.md".
- I wait for chat marker "VAMOS_E2E_PLAN_DOCS_REVIEW_OK".

### Then

- Transcript contains "VAMOS_E2E_PLAN_DOCS_REVIEW_OK".
- File "thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/context/implement/e2e-pi-plan-docs-review.md" changed from remembered hash.
- File "thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/context/implement/e2e-pi-plan-docs-review.md" contains required Pi review sections.
- Only file "thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/context/implement/e2e-pi-plan-docs-review.md" changed.
- I reload chat.
- Transcript contains "VAMOS_E2E_PLAN_DOCS_REVIEW_OK".
- I reopen current chat.
- Transcript contains "VAMOS_E2E_PLAN_DOCS_REVIEW_OK".

## Scenario: Freeform chat fixture replays durable transcript

### Given

- I am authenticated as "playwright@localhost".
- Fixture "freeform-chat.durable" is loaded.

### When

- I open freeform chat for fixture "freeform-chat.durable".

### Then

- Transcript contains "VAMOS_E2E_FREEFORM_REPLAY_OK".
- I reload chat.
- Transcript contains "VAMOS_E2E_FREEFORM_REPLAY_OK".
- I reopen current chat.
- Transcript contains "VAMOS_E2E_FREEFORM_REPLAY_OK".

## Scenario: Freeform chat started from Thoughts root survives refresh and resume

### Given

- I am authenticated as "playwright@localhost".
- I open Thoughts root chat.

### When

- I send freeform chat prompt "VAMOS_E2E_FREEFORM_REFRESH_FIRST".
- I wait for latest freeform chat run completion.

### Then

- I reload chat.
- Transcript contains "VAMOS_E2E_FREEFORM_REFRESH_FIRST".
- I send freeform chat prompt "VAMOS_E2E_FREEFORM_REFRESH_SECOND".
- I wait for latest freeform chat run completion.
- I reload chat.
- Transcript contains "VAMOS_E2E_FREEFORM_REFRESH_SECOND".

## Scenario: Anchor document navigation preserves embedded chat

### Given

- I am authenticated as "playwright@localhost".
- Fixture "freeform-chat.durable" is loaded.
- I open freeform chat for fixture "freeform-chat.durable".

### When

- I follow first sidebar document link.

### Then

- Browser URL contains "context=chat".
- Transcript contains "VAMOS_E2E_FREEFORM_REPLAY_OK".

## Scenario: Workspace document without chat params restores workspace chat

### Given

- I am authenticated as "playwright@localhost".
- Latest workspace chats "VAMOS_E2E_WORKSPACE_DOC_RESTORE" and "VAMOS_E2E_WORKSPACE_UNUSED" are seeded.

### When

- I open workspace document without chat params for seeded workspace "A".

### Then

- Transcript contains "VAMOS_E2E_WORKSPACE_DOC_RESTORE".

## Scenario: Root Thoughts restores latest freeform chat

### Given

- I am authenticated as "playwright@localhost".
- Fixture "freeform-chat.durable" is loaded.
- I open freeform chat for fixture "freeform-chat.durable".

### When

- I open Thoughts root with chat context.

### Then

- Transcript contains "VAMOS_E2E_FREEFORM_REPLAY_OK".

## Scenario: Workspace switching restores each workspace latest chat

### Given

- I am authenticated as "playwright@localhost".
- Latest workspace chats "VAMOS_E2E_WORKSPACE_A_LATEST" and "VAMOS_E2E_WORKSPACE_B_LATEST" are seeded.

### When

- I open seeded workspace chat "A".

### Then

- Transcript contains "VAMOS_E2E_WORKSPACE_A_LATEST".
- I open seeded workspace chat "B".
- Transcript contains "VAMOS_E2E_WORKSPACE_B_LATEST".
- I open seeded workspace chat "A".
- Transcript contains "VAMOS_E2E_WORKSPACE_A_LATEST".
- I open seeded workspace chat "B".
- Transcript contains "VAMOS_E2E_WORKSPACE_B_LATEST".
