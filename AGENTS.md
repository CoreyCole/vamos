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
- Runtime metadata uses `.vamos/` and `VAMOS_*` names.
- Datastar Pro JS assets are licensed and gitignored. Build tooling may copy from `VAMOS_DATASTAR_PRO_ASSET` or `../datastar-pro/datastar-pro-v1.js`; do not download them silently.

## Development checkout model

- `../vamos` is the working checkout for human/agent edits. Run Pi sessions and normal feature development here.
- `../vamos-main` is the clean/latest baseline checkout. Keep it clean, on `main`, and do not edit it directly.
- Feature branches use Graphite stacks from the working checkout. Preserve stack commit shape; do not squash or patch-apply branch contents into `main`.
- For substantial planned work, use the QRSPI skills (`/q-question`, `/q-research`, `/q-design`, `/q-outline`, `/q-plan`, `/q-review`, `/q-workspace`, `/q-implement`, `/q-review-implementation`, `/q-verify`) rather than ad hoc planning or implementation.
- `.agents` is committed as a symlink to `../.agents` when this repo is hosted beside a shared agent-config directory. Put broadly useful cross-repository skills there; commit the symlink only, never the target files.
- `.pi/` is a real project-local directory for Vamos-specific Pi resources: skills, prompts, and extensions that should travel with this repository.
- Use `/vamos-merge` when a workspace branch is ready. It fast-forwards the working checkout `main`, fast-forwards `../vamos-main`, then runs the configured host rebuild/restart verification.

## Linked project baseline model

When a host config defines both working and baseline checkouts, treat them as separate roles:

- working checkout: human/agent editable and may be dirty
- baseline checkout: clean/latest on configured branch, used for git history reads and copied workspace seeds

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

## Story E2E guidance

Use `vamos e2e check` for story/parser/generator validation. Use `vamos e2e run` only in a registered non-main workspace when fixtures are enabled; it refuses canonical main DBs. Generated files under `pkg/e2e/generated` come from `docs/features/*.story.md` and should be regenerated, not hand-edited. Visual review belongs to `vamos e2e review` and `e2e-image-review`, not deterministic test runs. `vamos e2e fix` is bounded to selectors, steps, runtime, and generated tests unless a human explicitly approves story/product changes.
