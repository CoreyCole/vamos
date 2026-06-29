# Examples Development Guide

Examples are small, long-running Go applets that demonstrate Vamos app patterns. Keep them portable, easy to run locally, and safe for non-technical users to modify through chat.

## UI and interaction pattern

- Use the Datastar CQRS pattern:
  - one long-lived SSE stream for reads, usually opened with `data-init="@get('/events')"`;
  - short POST/PUT/PATCH/DELETE handlers for writes;
  - backend state is the source of truth;
  - send full server-rendered components and let Datastar morph by stable IDs;
  - do not put application state or scoring/business logic in frontend signals.
- Use `templ` for HTML views/components. Commit both `.templ` source files and generated `*_templ.go` files for standalone example applets.
- Use Tailwind utility classes for styling in new examples. Avoid bespoke inline CSS unless the example is intentionally demonstrating a no-build standalone constraint and documents that exception.
- Use real HTML forms with `name` attributes for write actions. Prefer `@post('/path', {contentType: 'form'})` from a button inside the form.

## Signals and ownership

- The database/backend owns durable and authoritative state. Frontend signals are not the source of truth.
- Use frontend signals for ephemeral browser interaction state only, such as:
  - whether a dropdown, drawer, or details panel is open;
  - the current value of a transient input before submission;
  - client-only loading/disabled indicators;
  - small view preferences that do not need to survive refresh unless the backend persists them.
- Do not use frontend signals for business rules, game scoring, authorization, workflow state, or any data that must be correct after refresh or across clients.
- During initial page render, initialize page signals from backend/database state with `data-signals` so the browser starts from the server's known state.
- When backend state changes and the frontend needs signal updates, patch signals from Datastar SSE using `PatchSignals`/`datastar-patch-signals` instead of computing those values in the browser.
- Prefer patching full templ components for UI state. Patch signals only when a signal is genuinely the right representation for local UI behavior or Datastar bindings.

## Persistence

- Keep state in the backend process when ephemeral state is enough.
- If an example needs durable relational storage, use SQLite.
- If an example uses SQLite, use `sqlc` for typed queries rather than hand-written row scanning spread through handlers.
- Store generated database files only inside the example's configured files root, never in source directories unless explicitly checked in as a fixture.

## Applet boundaries

- Read and write user-visible files only inside `VAMOS_APP_FILES_ROOT` or the example's documented files root.
- Keep applets as long-running Go HTTP servers with `/healthz` and `/`.
- Keep hidden implementation iterations under `files/apps/iterations/`; normal app code must not write there.
- User-facing language should describe the domain, not builds, branches, logs, workspaces, or implementation machinery.
