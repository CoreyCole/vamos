# Vamos development workflow

This doc defines how humans and agents use Vamos to develop Vamos itself.

## Checkout roles

| Path | Role | Rule |
| --- | --- | --- |
| `../cn-agents-main` | canonical private host checkout used by systemd | stays clean/latest remote `main` |
| `../vamos-main` | canonical reusable Vamos runtime imported by `cn-agents-main` | stays clean/latest remote `main`; never edit directly |
| `../cn-agents` | private host working checkout and `thoughts/` symlink target | use only for private host/config changes |
| `../vamos` | editable staging/local Vamos checkout | prototype/commit local-main work here; exposed as `stage` workspace |
| copied feature checkout | isolated QRSPI implementation workspace | created by `/q-workspace` after final plan review |

## URLs

- `main.workspaces...` is the primary command center and durable app history.
- `stage.workspaces...` previews editable `../vamos` when main manager starts configured `stage`.
- `<feature-slug>.workspaces...` previews a copied implementation checkout.

## Where code is written

- Never write bug fixes or feature code in `../vamos-main`.
- Routine staging/prototype work and quick fixes happen in editable `../vamos`, which is exposed on this host as the durable `stage` lane.
- Prefer to test fast runtime iterations on `stage.workspaces...` from `../vamos` before promoting to `main`.
- QRSPI plan work moves to a copied feature checkout after `/q-review [plan.md]` and `/q-workspace`.
- Feature checkout branches use Graphite slice commits; merge through `/vamos-merge` after review/verify.

## State and session ownership

- Main DB is durable control-plane history: real web chats, QRSPI workspaces, release queue, audit, and long-lived app state.
- Workspace `.vamos/state/agents.db` and `.vamos/state/temporal.db` are sandbox runtime state for `stage` and feature URLs.
- Workspace DB/Temporal/log state can be reset by deleting or replacing that checkout's `.vamos/` directory while stopped.
- Pi CLI JSONL sessions are global user-level artifacts, normally under `~/.pi/agent/sessions`.
- Workspace cleanup deletes the copied checkout and its `.vamos/*`; it does not delete global Pi JSONL files.
- Sandbox Pi/chat sessions are recoverable/importable evidence, but they are not main durable chat history unless explicitly imported, linked, or summarized.

## Testing rule

Use `main` to manage. Use `stage` or feature URLs to test runtime/chat/Temporal UX safely. Promote only from main when explicit evidence exists: QRSPI human-review readiness, verification artifacts, passing tests, and release preflight.
