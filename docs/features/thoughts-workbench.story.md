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
- Console has no errors or warnings.

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

## Scenario: Workbench reload preserves DB layout state

### Given

- I am authenticated as "tester@example.com".
- Fixture "thoughts-workbench.basic" is loaded.
- I visit "/thoughts/example.md?context=chat".
- I wait for feature "thoughts.workbench" to be ready.

### When

- I switch tab "thoughts.sidebar.workspaces".
- I switch tab "thoughts.rightRail.chat".
- I toggle region "thoughts.workbench.sidebar".
- I follow first sidebar document link.

### Then

- Tab "thoughts.sidebar.workspaces" is selected.
- Tab "thoughts.rightRail.chat" is selected.
- Region "thoughts.workbench.center" is reachable.
- Inactive tab panels are hidden before interaction.

## Scenario: QRSPI sidebar hides terminal plan workspaces by default

### Given

- I am authenticated as "tester@example.com".
- Fixture "thoughts-workbench.qrspi-lifecycle" is loaded.
- I visit "/".
- I wait for feature "thoughts.workbench" to be ready.

### When

- I switch tab "thoughts.sidebar.workspaces".

### Then

- Workspace "question-plan" is visible.
- Workspace "merged-plan" is absent.
- Workspace "closed-plan" is absent.
- I enable Show historical workspaces.
- Workspace "merged-plan" is visible.
- Workspace "closed-plan" is visible.

## Scenario: Workspaces page pins release lanes and hides stale implementation rows

### Given

- I am authenticated as "tester@example.com".
- Fixture "workspaces.release-lanes" is loaded.

### When

- I visit "/workspaces".
- I wait for feature "workspaces.list" to be ready.

### Then

- Workspace "main" appears before workspace "stage".
- Workspace "stage" appears before workspace "active-feature".
- Workspace "missing-feature" is absent.
- Workspace "merged-feature" is absent.
- Workspace "cleaned-feature" is absent.
- I enable Show historical workspaces.
- Workspace "missing-feature" is visible.
- Workspace "merged-feature" is visible.
- Workspace "cleaned-feature" is visible.

## Scenario: Duplicate cleanup is harmless

### Given

- I am authenticated as "tester@example.com".
- Fixture "workspaces.cleaned" is loaded.
- I visit "/workspaces".
- I wait for feature "workspaces.list" to be ready.

### When

- I clean up workspace "cleaned-feature".

### Then

- Workspace cleanup succeeds.
- Workspace "cleaned-feature" is absent.

## Properties

### Workbench regions remain usable across viewport classes

For each viewport:

- mobile
- desktop-half
- desktop-full

For each route:

- "/"
- "/thoughts"
- "/thoughts/example.md?context=chat"

Then:

- Region "thoughts.workbench.sidebar" is reachable.
- Region "thoughts.workbench.center" is reachable.
- Region "thoughts.workbench.right" is reachable.
- Text "Session history" is absent.
- Console has no errors or warnings.

### Saved mobile active state does not pin desktop refresh

For each viewport:

- mobile
- desktop-full

For each route:

- "/thoughts/example.md?context=chat"

Then:

- Region "thoughts.workbench.sidebar" is reachable.
- Region "thoughts.workbench.center" is reachable.
- Region "thoughts.workbench.right" is reachable.
- Console has no errors or warnings.
