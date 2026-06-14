# Vamos Agents Development Guide

Vamos Agents is a reusable Go/templ/Datastar server for private agentic software factories. It serves a configured `thoughts/` artifact directory, manages shared Agent Chat/Pi sessions, and hosts workflow runtimes such as QRSPI.

Agent Chat is fully multiplayer/shared. There is no per-user/private-workspace visibility model for plan-owned sessions. `user_email` and `workspace_id` should be populated when known for provenance, attribution, routing, and attachment metadata, but they must not gate sidebar/history visibility or hydration of plan-owned `.sessions/<agent>` artifacts.

### Workspace vocabulary

Use human-facing vocabulary before internal table names:

- **Main workspace / manager host**: authoritative service that tracks workspace inventory and lifecycle.
- **Staging workspace / `../vamos`**: durable staging checkout used after merging feature checkouts to test and fix before pushing `origin/main` and rebuilding/syncing the clean `../vamos-main` baseline.
- **Main baseline / `../vamos-main`**: clean/latest baseline checkout rebuilt after staging is verified and pushed; do not edit directly.
- **Manager DB lifecycle / implementation workspace record**: authoritative DB projection for feature/implementation checkout state (active, merged, cleaned, unknown). The implementation uses `impl_workspaces` internally, but docs/plans should not lead with that table name.
- **Feature checkout / implementation checkout**: copied source checkout where agents implement a plan before merging into staging.
- **Local runtime diagnostics**: checkout-local `.vamos/run/*` files such as `status.json`, `desired.json`, `runtime-env.json`, and `workspace.env`; useful for process/build/debug state, not authoritative lifecycle truth.

Vamos is pre-release OSS: optimize for clean long-term design, not legacy DB compatibility. The filesystem `thoughts/` tree and plan-owned `.sessions/<agent>` JSONL files are durable source of truth; the DB is rebuildable index/projection/cache. If the scheduled sync workflow can rebuild correct state from filesystem artifacts, it is acceptable to drop old DB rows, replace columns, or migrate destructively rather than preserving legacy code paths or tech debt.

### Long-term product architecture philosophy

Vamos is a distributed organizational memory and coordination system for AI-assisted software work. It should support local development, shared servers, durable artifacts, and multiplayer planning across an organization. Design for the future where AI sessions, plans, reviews, decisions, code context, and handoffs are shared organizational knowledge — not isolated state on one developer's laptop.

Implications for future planning:

- Prefer durable, portable, organization-shareable artifacts over machine-local state.
- Prefer `thoughts/...`-relative artifact identity over absolute filesystem paths. Vamos runs on many engineers' machines and shared servers from different checkout roots; absolute host paths are safety/IO details only, not durable identity.
- Prefer shared/multiplayer session and plan models over private per-user chat history when artifacts are plan-owned or project-owned.
- Treat `thoughts/`, QRSPI artifacts, `.sessions/<agent>`, ADRs, reviews, and handoffs as the recursive self-improvement substrate for the organization.
- Schema should express the durable domain clearly: artifact identity, provenance, lineage, workflow run, plan ownership, session/thread identity, sharing/visibility, and projection state.
- Do not preserve legacy columns, legacy adapters, or confusing compatibility paths when they obscure that domain model.
- If an old schema encoded laptop-local assumptions, user-private visibility, absolute paths, or transient workspace ownership, replace it with the long-term organizational model.
- Prefer explicit identity/visibility enums and normalized relationships over inferring semantics from nullable columns or path shapes.
- Keep provenance separate from authorization/visibility. Who created/indexed/attached something is not necessarily who may see it.
- The correct design should make multiplayer planning and shared AI session discovery obvious to future agents reading the schema and queries.

### Thoughts-backed DB/schema doctrine

For all future Vamos planning and implementation involving `thoughts/` artifacts, QRSPI plans, and plan-owned `.sessions/<agent>` JSONL:

- Disk is source of truth.
- DB is disposable projection/index/cache and should be safe to wipe at any time.
- A new engineer joining with the shared `thoughts/` directory should be able to run the scheduled filesystem-to-DB sync and rebuild the DB projection from disk artifacts.
- Prefer the best long-term schema over compatibility with old DB rows.
- Big schema changes are OK when they simplify the durable domain model and eliminate complexity from legacy decisions.
- Destructive migrations, dropped columns, renamed columns, table replacement, and full row rebuilds are acceptable when the scheduled filesystem-to-DB sync can rebuild from `thoughts/`.
- `just sync-thoughts` is the docs/artifact sync: it formats and pushes/syncs `thoughts/...` files to durable storage. It does not rebuild manager DB lifecycle state.
- "Scheduled sync" means the scheduled Temporal filesystem-to-DB indexing/projection workflow that reads `thoughts/...` artifacts plus configured project/workspace files and writes DB rows. It does not mean `just sync-thoughts` or any thoughts durable-cloud-storage persistence step.
- The scheduled filesystem-to-DB sync must rebuild correct DB state from `thoughts/...` artifacts and config files instead of preserving stale projection state.
- Do not add compatibility shims for legacy absolute paths or owner/workspace visibility when a clean re-index from disk is possible.
- For plan-owned artifacts, `user_email` and `workspace_id` are provenance/routing/attachment metadata only; never authorization or visibility gates.

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
- Store durable artifact references as `thoughts/...`-relative paths whenever possible. Resolve absolute paths only at IO boundaries for validation and file access.
- Runtime metadata uses `.vamos/` and `VAMOS_*` names.
- Datastar Pro JS assets are licensed and gitignored. Build tooling may copy from `VAMOS_DATASTAR_PRO_ASSET` or `../datastar-pro/datastar-pro-v1.js`; do not download them silently.

## Workflow-shaped feature guidance

When a feature is deterministic, agentic, multi-step, stateful, or needs human/agent/service transitions, model it as a workflow on top of `pkg/agents/workflows/runtime` instead of inventing a parallel builder/state machine. Extend the shared runtime with new node kinds or metadata when needed, then keep domain packages as adapters (for example `pkg/release` owns lanes/git safety while workflow runtime owns definition IDs, versions, nodes, transitions, validation, and registries). Workspace provisioning is workflow-shaped too: CLI should invoke a server workflow to create checkouts and initialize `.vamos/`, not rely on agents to manually follow setup instructions.

## Development checkout model

- For the local dogfood project map, configured workspace hosts, Caddy/Coredns notes, and cross-project checkout roles, read `docs/vamos-manifest.md`. Put setup-specific notes there instead of general OSS docs.
- `../vamos` is the staging/working checkout for human/agent edits. Feature checkout stacks merge here first; test and fix here before pushing to `origin/main` and rebuilding/syncing `../vamos-main`.
- In the current Chestnut dogfood host setup, the durable `stage` lane points at `../vamos`. Use that stage host for fast runtime iteration, quick fixes after feature merges, and pre-main verification.
- `../vamos-main` is the clean/latest baseline checkout. In the current host setup, durable `main` points at `../vamos-main`. Keep it clean, on `main`, and do not edit it directly; update it only after staging is verified and main is pushed/synced.
- Feature branches use Graphite stacks from the working checkout. Preserve stack commit shape; do not squash or patch-apply branch contents into `main`.
- For substantial planned work, use the QRSPI skills (`/q-question`, `/q-research`, `/q-design`, `/q-outline`, `/q-plan`, `/q-review`, `/q-workspace`, `/q-implement`, `/q-review-implementation`, `/q-verify`) rather than ad hoc planning or implementation.
- `.agents` is committed as a symlink to `../.agents` when this repo is hosted beside a shared agent-config directory. Put broadly useful cross-repository skills there; commit the symlink only, never the target files.
- In the local dogfood setup, `thoughts/` is a symlink to separate durable artifact storage and is not tracked by Vamos git. Use `just sync-thoughts` to run formatting and push/sync those thoughts artifacts to durable storage; the scheduled Temporal sync separately rebuilds manager DB state from `thoughts/...` artifacts and config files.
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
