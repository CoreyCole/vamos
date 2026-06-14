# Vamos dogfood development workflow

This doc describes the local maintainer workflow for developing Vamos inside a host-managed dogfood environment. It is not a reusable OSS setup guide. For generic setup, read [Local quickstart](local-quickstart.md), [Host configuration](host-configuration.md), and [Deployment](deployment.md).

## Generic model

- A durable manager host serves long-lived app history and workspace/release controls.
- A reusable Vamos runtime checkout is edited in a working checkout or copied implementation workspace.
- Clean baseline checkouts stay clean/latest and seed workspace copies or release lanes.
- Feature implementation happens in copied filesystem workspaces created by `/q-workspace`; do not use git worktrees.
- Host-owned config decides domains, service names, release lanes, and restart commands.

## Local dogfood details

Private checkout names, private domains, and sibling repository roles for this maintainer machine live in [Vamos dogfood project manifest](vamos-manifest.md). Do not copy those names into public docs or example configs.

## Testing rule

Use the durable manager to manage workspace/release actions. Use a staging lane or feature workspace to test runtime/chat/Temporal UX safely. Promote only when QRSPI review/verify artifacts and test evidence exist.
