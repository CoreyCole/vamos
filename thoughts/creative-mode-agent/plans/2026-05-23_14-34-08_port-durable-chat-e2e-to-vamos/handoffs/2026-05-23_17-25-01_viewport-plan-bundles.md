---
date: 2026-05-23T17:25:01-07:00
researcher: creative-mode-agent
stage: implement
repository: vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos
branch: port-durable-chat-e2e-to-vamos_slice-8
git_commit: 622d40f (amended after handoff creation)
plan_dir: thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos
artifact: thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/plan.md
---

# Implementation Handoff: Viewport Plan Bundles

Done: added viewport resolver tests, hardened QRSPI plan bundle export with manifest/failure/html/trace links, exact run command capture, safe plan-dir validation, and checked off the plan bundle work (8/10).

Next: add semantic goldens, visual review, bounded repair commands, then remove remaining deterministic `not implemented` E2E command paths.

Workspace: /home/ruby/cn/chestnut-flake/vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos; Branch: port-durable-chat-e2e-to-vamos_slice-8@622d40f (amended after handoff creation).

## Verification

- `go test ./pkg/e2e/... ./cmd/vamos-runtime/internal/e2ecmd`
- `go run ./cmd/vamos-runtime e2e check`
- `go run ./cmd/vamos-runtime e2e generate --check`
- `go run ./cmd/vamos-runtime e2e run --help | rg -- '--plan-dir|--viewport|--artifacts-dir'`
- `git diff --check`

## Notes for resume

- `ExportPlanBundle` now requires relative plan dirs to start with `thoughts/` and rejects `..`; absolute plan dirs are cleaned for test/runtime use.
- `RunE2E` writes an initial run manifest before exporting the plan bundle, then rewrites it with `PlanBundlePath` after the bundle index is known.
- The next work should implement `review`, `goldens`, and `fix` command paths instead of relying on the current `notImplemented` placeholders.
