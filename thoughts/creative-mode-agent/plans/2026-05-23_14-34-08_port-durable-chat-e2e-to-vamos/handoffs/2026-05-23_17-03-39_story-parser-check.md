---
date: 2026-05-23T17:03:39-07:00
researcher: creative-mode-agent
stage: implement
repository: vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos
branch: port-durable-chat-e2e-to-vamos_slice-4
git_commit: 43bb5ab5962e244e4a53b53897def3b2e476b4be
plan_dir: thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos
artifact: thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/plan.md
---

# Implementation Handoff: Story Parser + Check Command

Done: ported feature stories, story parser/validation, selector and step catalogs, fixture registry stub, and real `vamos e2e check`; removed private email/path literals from reusable stories/tests (4/10).

Next: add deterministic generator, `vamos e2e generate`, and checked-in generated tests derived from `docs/features/*.story.md`.

Workspace: /home/ruby/cn/chestnut-flake/vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos; Branch: port-durable-chat-e2e-to-vamos_slice-4@43bb5ab (amended after handoff creation).

## Verification

- `go test ./pkg/e2e/story ./pkg/e2e/selectors ./pkg/e2e/steps ./pkg/e2e/fixtures ./cmd/vamos-runtime/internal/e2ecmd`
- `go run ./cmd/vamos-runtime e2e check`
- `git diff --check`

## Notes for resume

- `cmd/vamos-runtime/internal/e2ecmd/check.go` intentionally does not depend on `pkg/e2e/generate` yet; freshness checks belong with the generator work.
- `pkg/e2e/steps/noop_steps.go` is a compile-only helper surface until browser/runtime helpers land.
- `pkg/e2e/fixtures/registry.go` currently validates fixture names with stub builders; database-backed fixture writes arrive with workspace fixture work.
- The remaining `e2e generate`, `run`, `review`, and `fix` commands still return `not implemented yet` until their planned work replaces them.
