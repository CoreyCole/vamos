# Feature: Thoughts workbench

## User story

As an internal user, I want Thoughts to open as a document workbench with Chat available.

## Business rules

- Thoughts is document-centered, not Agent Chat landing page.
- Chat is available by default on `/`.
- Retired session-history/right-rail UI must not appear.

## Scenario: Root opens document workbench with Chat

### Given

- I am authenticated as "tester@example.com".
- Fixture "thoughts-workbench.basic" is loaded.

### When

- I visit "/".
- I wait for feature "thoughts.workbench" to be ready.

### Then

- Region "thoughts.workbench.sidebar" is visible.
- Tab "thoughts.rightRail.chat" is selected.
- Text "Session history" is absent.

## Properties

### Workbench regions remain usable across viewport classes

For each viewport:

- mobile
- desktop-half
- desktop-full

For each route:

- "/"
- "/thoughts/example.md?context=chat"

Then:

- Region "thoughts.workbench.sidebar" is reachable.
- Region "thoughts.workbench.center" is reachable.
- Region "thoughts.workbench.right" is reachable.
- Text "Session history" is absent.
