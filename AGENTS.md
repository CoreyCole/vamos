# Vamos Agents Development Guide

Vamos Agents is a reusable Go/templ/Datastar server for private agentic software factories. It serves a configured `thoughts/` artifact directory, manages shared Agent Chat/Pi sessions, and hosts workflow runtimes such as QRSPI.

## Repository shape

```text
vamos/
├── go.mod                         # module github.com/CoreyCole/vamos
├── justfile                       # build wrapper: go run ./cmd/build-agents
├── cmd/
│   ├── server/                    # server entrypoint
│   ├── agentsctl/                 # management/verification CLI
│   └── build-agents/              # smart build tool
├── server/                        # HTTP services, layouts, templates
├── pkg/                           # reusable runtime, db, components, git, proto
├── static/                        # served assets; generated CSS/JS ignored
└── docs/                          # reusable Vamos docs
```

## Portability rules

- Keep company-specific paths, domains, OAuth policy, users, repo names, and service names out of reusable code.
- Host applications provide YAML/env config for branding, auth, thoughts roots, linked projects, deploy names, and workspace conventions.
- The `thoughts/` directory is host-owned data. Vamos reads/writes the configured thoughts root; it should not assume a colocated `thoughts/` directory.
- Runtime metadata uses `.vamos/` and `VAMOS_*` names. Do not reintroduce legacy `CN_AGENTS_*`, `REPO_PATH`, or `MARKDOWN_BASE_PATH` behavior.
- Datastar Pro JS assets are licensed and gitignored. Build tooling may copy from `VAMOS_DATASTAR_PRO_ASSET` or `../datastar-pro/datastar-pro-v1.js`; do not download them silently.

## Linked project baseline model

When a host config defines both working and baseline checkouts, treat them as separate roles:

- working checkout: human/agent editable; may be dirty; e.g. `vamos`, `monorepo`
- baseline checkout: clean/latest on configured branch; used for git history reads and copied workspace seeds; e.g. `vamos-main`, `monorepo-main`

Before copying from a baseline checkout, verify it is clean, on the configured branch, and latest with upstream when strict baseline validation is requested.

## Common commands

```bash
just build --no-restart

go test ./server/config ./server/services/workspaces ./server/services/agentchat ./cmd/build-agents/internal/build
```

Use plain `just build` only when intentionally restarting a configured running service/workspace.

## UI/server rules

- Build MPAs with Datastar CQRS: backend source of truth, SSE streams for reads, short POSTs for writes.
- Use real HTML forms with `name` attributes and stable IDs for SSE-patched elements.
- Avoid inline styles; use Tailwind utilities.
- Shared UI primitives live under `pkg/components/`.
