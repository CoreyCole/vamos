---
date: 2026-05-23T17:07:30-07:00
researcher: creative-mode-agent
stage: implement
repository: vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos
branch: port-durable-chat-e2e-to-vamos_slice-5
git_commit: c25fb6f
plan_dir: thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos
artifact: thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/plan.md
---

# Implementation Handoff: Story Generator + Generated Tests

Done: added deterministic story-to-Go generator, `vamos e2e generate`, checked-in generated tests, runtime compile shim, and runtime-artifact gitignore rules (5/10).

Next: port Playwright runtime, auth/config loading, run manifests/artifacts, and `vamos e2e run` command.

Workspace: /home/ruby/cn/chestnut-flake/vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos; Branch: port-durable-chat-e2e-to-vamos_slice-5@c25fb6f (amended after handoff creation).

## Verification

- `go test ./pkg/e2e/generate ./pkg/e2e/generated ./cmd/vamos-runtime/internal/e2ecmd`
- `go test ./pkg/e2e/...`
- `go run ./cmd/vamos-runtime e2e check`
- `go run ./cmd/vamos-runtime e2e generate --check`
- `! rg 'Locator\(|playwright\.Page|page\.' pkg/e2e/generated`
- `git diff --check`

## Notes for resume

- `pkg/e2e/runtime/scenario.go` is a compile shim for generated tests only; the real Playwright runtime replaces/extends it next.
- Generated tests are now source-controlled and contain curated `steps.*` calls only, with no raw Playwright selectors.
- `vamos e2e check` remains story/catalog/fixture validation; generated freshness is enforced by `vamos e2e generate --check`.
