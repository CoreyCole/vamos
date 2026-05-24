# Feature: Thoughts workbench

## User story

As a workspace user, I want Thoughts to open as a document workbench with Chat available.

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

## Scenario: Document sidebar navigation uses normal document links

### Given

- I am authenticated as "tester@example.com".
- Fixture "thoughts-workbench.basic" is loaded.
- I visit "/thoughts/example.md?context=chat&chat_workspace=ws_1&thread=th_1&run=run_1".
- I wait for feature "thoughts.workbench" to be ready.

### When

- I follow first sidebar document link.

### Then

- Browser URL contains "context=chat".
- Browser URL contains "thread=th_1".
- Region "thoughts.workbench.center" is reachable.

## Scenario: Breadcrumb parent navigation works

### Given

- I am authenticated as "tester@example.com".
- Fixture "thoughts-workbench.basic" is loaded.
- I visit "/thoughts/owner/plans/demo/outline.md?context=chat&thread=th_1".
- I wait for feature "thoughts.workbench" to be ready.

### When

- I follow first breadcrumb link.

### Then

- Browser URL contains "/thoughts/".
- Region "thoughts.workbench.center" is reachable.

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
