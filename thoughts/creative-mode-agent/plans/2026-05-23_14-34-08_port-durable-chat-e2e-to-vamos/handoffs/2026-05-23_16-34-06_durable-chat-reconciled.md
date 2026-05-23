---
date: 2026-05-23T16:34:06-07:00
researcher: creative-mode-agent
stage: implement
repository: vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos
branch: port-durable-chat-e2e-to-vamos_slice-2
git_commit: ad5c74038ecaab51e9b0439cb55ac540d82e689f
plan_dir: thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos
artifact: thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/context/implement/durable-chat-reconciliation.md
---

# Implementation Handoff: Durable Chat Reconciled

Done: compared durable-chat/schema candidates, kept current `impl_workspaces` schema because target still has live workspace sync/UI dependencies, and ported persisted freeform selection rendering so refresh/resume uses the freeform right rail (2/10).

Next: add the Vamos runtime CLI and ctl shell from the old stack, adapting paths/env from nested `pkg/agents` and legacy names to Vamos root, `.vamos`, and `VAMOS_*`.

Workspace: /home/ruby/cn/chestnut-flake/vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos; Branch: port-durable-chat-e2e-to-vamos_slice-2@ad5c740 (amended after handoff creation).

## Verification

- `rg 'impl_workspaces|ImplWorkspace' pkg/db server pkg cmd || true` (582 current target references; confirms table cannot be safely deleted in this boundary)
- `go test ./pkg/db/... ./pkg/agents/chatsession/... ./server/services/agentchat`
- `git diff --check`

## Notes for resume

- Reconciliation artifact: `context/implement/durable-chat-reconciliation.md`.
- Diff artifacts: `context/implement/diffs/**`.
- The only functional old unstaged delta in the listed agentchat files was `embedded_chat.go`; the other compared agentchat diffs were gofmt-only.
- `impl_workspaces` remains a follow-up semantic conflict for later workspace projection work if/when the old plan-workspace projection surface is ported; do not delete it until all current workspaces package/sqlc callers are replaced.
