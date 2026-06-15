# Fast Datastar CQRS Pages

Vamos pages use backend-source-of-truth Datastar CQRS: initial HTML is useful by itself, one SSE read stream fat-morphs stable components, and short POSTs perform writes.

## Rules

- Keep page render and SSE first patch bounded to DB/template reads.
- Do not run shell commands, git checks, network fanout, or long workspace scans inside render/model build.
- Serialize SQLite writers; WAL and `busy_timeout` are not writer concurrency control.
- Keep stream handlers simple: re-query backend state, render full components, and patch stable IDs.
- Initial stream failure must not make server-rendered HTML useless.
- After the initial stream patch, log notifier rebuild errors and keep listening when safe.
- Signals are UI-only toggles/loading indicators; application state stays backend-owned.
- Dropdown/menu IDs inside morph targets must be unique per rendered instance.
- Do not put dropdown content inside clipping scroll wrappers unless it portals outside.
- Destructive actions must use a confirmation dialog; never submit delete/close cleanup directly from a dropdown item.
- Add latency/error logs and click tests for interactive controls.

## Workspaces example

`/workspaces` follows this pattern: cheap model build, full-component patches for header/release/list, release git preflight only on explicit enqueue, scoped Actions dropdown signals, and confirmation dialogs for destructive workspace cleanup.
