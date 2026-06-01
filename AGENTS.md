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

## Workflow-shaped feature guidance

When a feature is deterministic, agentic, multi-step, stateful, or needs human/agent/service transitions, model it as a workflow on top of `pkg/agents/workflows/runtime` instead of inventing a parallel builder/state machine. Extend the shared runtime with new node kinds or metadata when needed, then keep domain packages as adapters (for example `pkg/release` owns lanes/git safety while workflow runtime owns definition IDs, versions, nodes, transitions, validation, and registries). Workspace provisioning is workflow-shaped too: CLI should invoke a server workflow to create checkouts and initialize `.vamos/`, not rely on agents to manually follow setup instructions.

## Development checkout model

- For the local dogfood project map, configured workspace hosts, Caddy/Coredns notes, and cross-project checkout roles, read `docs/vamos-manifest.md`. Put setup-specific notes there instead of general OSS docs.
- `../vamos` is the working checkout for human/agent edits. Run Pi sessions and normal feature development here.
- In the current Chestnut dogfood host setup, the durable `stage` lane points at `../vamos`. Use that stage host for fast runtime iteration, quick fixes, and pre-merge verification.
- `../vamos-main` is the clean/latest baseline checkout. In the current host setup, durable `main` points at `../vamos-main`. Keep it clean, on `main`, and do not edit it directly.
- Feature branches use Graphite stacks from the working checkout. Preserve stack commit shape; do not squash or patch-apply branch contents into `main`.
- For substantial planned work, use the QRSPI skills (`/q-question`, `/q-research`, `/q-design`, `/q-outline`, `/q-plan`, `/q-review`, `/q-workspace`, `/q-implement`, `/q-review-implementation`, `/q-verify`) rather than ad hoc planning or implementation.
- `.agents` is committed as a symlink to `../.agents` when this repo is hosted beside a shared agent-config directory. Put broadly useful cross-repository skills there; commit the symlink only, never the target files.
- `.pi/` is a real project-local directory for Vamos-specific Pi resources: skills, prompts, and extensions that should travel with this repository.
- Use `/vamos-merge` when a workspace branch is ready. It verifies `stage` from `../vamos`, fast-forwards `../vamos-main`, then runs the configured host rebuild/restart verification.

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
- Shared UI primitives come from the pinned DatastarUI dependency (`github.com/coreycole/datastarui/components/*`). Prefer those primitives before bespoke Tailwind in templ UI.
- Reusable primitive fixes belong upstream in DatastarUI; Vamos owns app-specific composition and must not recreate a local component fork.

## Story E2E guidance

Use authored Go Story API tests under `pkg/e2e/tests` as the canonical E2E specs. Run deterministic browser stories from this checkout with `just e2e --base-url <feature-url> --story <story>`; the recipe delegates to `../datastarui/scripts/datastarui.sh` with this checkout's `datastarui-e2e.yml`. DatastarUI owns the flat Story builder, runtime, launcher, artifacts, review, and goldens. Vamos owns typed app helpers in `pkg/e2e/vamos` for auth, fixtures, pages, selectors, readiness, and expectations. Use only from a registered non-main workspace when fixtures are enabled; Vamos fixture helpers refuse canonical main DBs. The old markdown story parser/generator path has been removed; do not add `docs/features/*.story.md` or `pkg/e2e/generated`. Visual review belongs to DatastarUI `e2e review`; Vamos repair/fix policy is bounded to app helpers/tests unless a human explicitly approves product changes.
