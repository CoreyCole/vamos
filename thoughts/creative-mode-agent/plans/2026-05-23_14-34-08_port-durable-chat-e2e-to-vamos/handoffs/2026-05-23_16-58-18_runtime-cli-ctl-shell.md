---
date: 2026-05-23T16:58:18-07:00
researcher: creative-mode-agent
stage: implement
repository: vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos
branch: port-durable-chat-e2e-to-vamos_slice-3
git_commit: 619a0306796c7a0a84bbfce7e82ec4253445e16d
plan_dir: thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos
artifact: thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/plan.md
---

# Implementation Handoff: Runtime CLI + ctl Shell

Done: added `cmd/vamos-runtime`, `cmd/vamos-launcher`, reusable `pkg/ctl`, and kept `cmd/agentsctl` as a compatibility shim over `pkg/ctl`; adapted runtime metadata to `.vamos`, `VAMOS_*`, and `X-Vamos-Workspace-Restart-Token` (3/10).

Next: port story files, parser, selector catalog, step catalog, fixture registry stub, and `vamos e2e check` command; replace the temporary `e2e check` shell implementation with real validation.

Workspace: /home/ruby/cn/chestnut-flake/vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos; Branch: port-durable-chat-e2e-to-vamos_slice-3@619a030 (amended after handoff creation).

## Verification

- `go test ./cmd/vamos-runtime/... ./cmd/vamos-launcher/... ./pkg/ctl/...`
- `go run ./cmd/vamos-runtime --help`
- `go run ./cmd/vamos-runtime ctl workspace --help`
- `go run ./cmd/vamos-runtime e2e --help`
- `! rg 'CN_AGENTS|\.cn-agents|REPO_PATH|MARKDOWN_BASE_PATH' cmd/vamos-runtime cmd/vamos-launcher pkg/ctl cmd/agentsctl`
- `go test ./cmd/agentsctl`
- `git diff --check`

## Notes for resume

- The current `cmd/vamos-runtime/internal/e2ecmd/root.go` intentionally exposes command shape only; subcommands return `not implemented yet` until their planned slices replace them.
- `pkg/ctl` was moved from the old `cmd/agentsctl/internal` implementation and adapted to the current Vamos workspace-manager API header (`X-Vamos-Workspace-Restart-Token`).
- `cmd/agentsctl` now delegates to `pkg/ctl` so there is one reusable ctl implementation.
