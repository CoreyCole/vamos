# Feature: Thoughts workbench

## User story

As a workspace user, I want Thoughts to open as a document workbench with Chat available.

## Business rules

- Thoughts is document-centered, not Agent Chat landing page.
- Chat is available by default on `/`.
- Retired session-history/right-rail UI must not appear.

## Scenario: Root opens document workbench with Chat

### Given

- I am authenticated as "playwright@localhost".
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

- I am authenticated as "playwright@localhost".
- Fixture "thoughts-workbench.basic" is loaded.
- I visit "/thoughts/example.md?context=chat&thread=th_1".
- I wait for feature "thoughts.workbench" to be ready.

### When

- I follow first sidebar document link.

### Then

- Browser URL contains "context=chat".
- Browser URL contains "thread=th_1".
- Region "thoughts.workbench.center" is reachable.

## Scenario: Breadcrumb parent navigation works

### Given

- I am authenticated as "playwright@localhost".
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

- I am authenticated as "playwright@localhost".
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
