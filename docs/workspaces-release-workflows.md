# Workspace release workflows

Vamos can expose a release queue on `/workspaces` when the host config provides release lane checkouts and Temporal is enabled. Release flows are deterministic workflow runtime definitions; HTTP handlers only validate/enqueue, and Temporal activities own git mutations, host commands, event logs, and cleanup.

## Model

- `pkg/agents/workflows/runtime` owns workflow IDs, versions, service nodes, graph ordering, validation, and registries.
- `pkg/release` adapts that runtime to release domain metadata: lanes, source selectors, target lanes, push policy, and preconditions.
- `server/services/workspaces` projects lane/action state into the Workspaces page, persists queue rows/events, starts Temporal workflows, and executes service nodes.

Do not create a parallel release builder or ad hoc state machine for a new release-like feature. Add service node metadata to the shared runtime, then keep domain-specific safety checks in a thin adapter package.

## Default lanes

If a manager host config registers configured checkouts with slugs `stage` and `main`, Vamos builds a default release registry:

- feature workspace → `stage`: `release.preflight`, `release.merge`, no push
- `stage` → `main`: `release.preflight`, `release.merge`, `release.push`

`main` is protected and cannot be used as a release source. Hosts that need different lane names or host-specific verification commands should construct a custom `release.Registry` and workflow registry in host wiring rather than hardcoding private paths in Vamos.

## Development topology

For Vamos self-development, `main.<domain>` is the durable manager. `work.<domain>` and feature workspace hosts are sandbox runtimes. Release actions are triggered from main, not from sandbox-local DB state. Sandbox sessions may be imported or linked as evidence, but release readiness should come from explicit QRSPI/verification artifacts. See `docs/vamos-development-workflow.md`.

## Queue behavior

1. `/workspaces/release/enqueue` validates the selected action and expected source/target commits.
1. The handler creates a pending `release_queue_items` row and signals `release-queue/default`.
1. `ReleaseQueueWorkflow` wakes on signals but always lets the DB claim the oldest pending item. Signal item IDs are observability hints only.
1. `ReleaseActivities.ProcessNextReleaseQueueItem` resolves the captured release/workflow version and executes reachable service nodes in graph order.
1. Business/preflight failures mark the item `failed` and unblock later queue items. Infrastructure errors return to Temporal retry/backoff.

Queue events are durable and streamed back into the `release-queue-panel` by SSE. Failed items remain in history and can be re-enqueued after the underlying checkout problem is fixed.

## Service node types

Built-in service executors:

- `release.preflight`: source/target exist, expected commits still match, target checkout is clean, and source can be fetched by the target checkout.
- `release.merge`: fetches source into target and fast-forwards/merges via safe git commands.
- `release.command`: runs configured argv with stdout/stderr lines appended to queue events.
- `release.push`: pushes the target lane only when the flow uses `PushAfterVerify`.

Unknown service types are delegated to host executors when registered. Do not embed host service names, private systemd units, domains, or repository paths in reusable Vamos code.

## Cleanup

Workspace cleanup is manual. Merged workspaces render `Clean up`; unmerged workspaces render `Close` and require confirmation. Cleanup starts `CleanupWorkspaceWorkflow`, which delegates deletion to `ManagerService.CleanupWorkspace`, rejects configured/main checkouts, removes checkout-owned runtime files by deleting the checkout directory, marks implementation workspace rows cleaned up, and preserves configured thoughts roots and plan directories.

## Verification

For code changes around release workflows, run:

```bash
templ generate
go test ./pkg/agents/workflows/runtime ./pkg/release ./server/services/workspaces ./pkg/db ./cmd/build-agents/internal/build
just build --no-restart
```

Manual acceptance requires a manager with Temporal enabled and at least `stage`/`main` configured checkouts:

1. Open `/workspaces`.
1. Confirm the release panel shows lane state and row-scoped feature promote actions gated by QRSPI human-review readiness.
1. Enqueue a feature → stage action; verify pending/running/log/succeeded or failed history updates.
1. Confirm a failed item does not block a later item.
1. Confirm merged workspaces show `Clean up`; unmerged workspaces show `Close` warning.
1. Confirm cleanup removes checkout/runtime metadata but leaves `thoughts/...` plan files.
