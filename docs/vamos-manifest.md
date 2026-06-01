# Vamos dogfood project manifest

This is local dogfood context for agents working in the Chestnut/Vamos development setup. It is not a reusable Vamos runtime contract. Keep host-specific paths, domains, Caddy routes, and private operational notes here instead of in general OSS docs such as DatastarUI docs or reusable Vamos feature docs.

## Where this note belongs

Use this manifest for information that is true for our current Vamos host setup but not generally true for all Vamos users:

- local sibling checkout roles
- configured workspace lanes and Caddy hostnames
- which checkout is editable vs clean baseline
- cross-project dependency notes between Vamos, DatastarUI, cn-agents, and Chestnut
- manual-review URLs exposed by the private workspace network

Do not put this information in DatastarUI OSS docs, because those docs should describe reusable library behavior, not our private Caddy/workspace routing.

## Workspace network

The private `cn-agents` host owns the workspace network configuration. It renders Caddy/CoreDNS config for `*.workspaces.creative-mode.ai` from host YAML. The configured checkout hostnames are private dogfood conveniences, not reusable Vamos defaults.

Current important hosts:

| Host | Purpose | Backing checkout |
| --- | --- | --- |
| `https://main.workspaces.creative-mode.ai` | durable main Vamos command center | `../cn-agents-main` host + `../vamos-main` runtime |
| `https://stage.workspaces.creative-mode.ai` | editable Vamos staging lane | `../vamos` |
| `https://datastarui.workspaces.creative-mode.ai` | DatastarUI staging/manual review lane | `../datastarui` |
| `https://<feature-slug>.workspaces.creative-mode.ai` | copied QRSPI feature workspace | copied checkout under the configured workspace parent |

If someone asks whether the DatastarUI staging server is available, check `datastarui.workspaces.creative-mode.ai` first, then verify which local checkout/commit backs it before claiming it is latest.

## Project roles

| Project | Checkout(s) | Role | Edit rules |
| --- | --- | --- | --- |
| Vamos runtime | `../vamos`, `../vamos-main`, copied `vamos-*` workspaces | reusable Go/templ/Datastar runtime for thoughts, Agent Chat, Pi sessions, workspaces, QRSPI | edit `../vamos` or copied feature workspaces; keep `../vamos-main` clean |
| cn-agents host | `../cn-agents`, `../cn-agents-main` | private host/wrapper, config, systemd, thoughts store, workspace network | host/config changes in `../cn-agents`; systemd/browser-visible host runs from clean `../cn-agents-main` |
| DatastarUI | `../datastarui` | reusable Go/templ component library and Story E2E runner used by Vamos | reusable primitive/E2E fixes go here; manual review via `datastarui.workspaces.creative-mode.ai` when configured/running |
| Chestnut monorepo | `../monorepo`, `../monorepo-main` when present | product application checkout(s) used by Chestnut work | follow monorepo-specific AGENTS/testing docs; baseline checkout must stay clean |
| shared agent config | `../.agents`, symlinked from repos | shared skills/hooks/rules/commands | commit symlinks in public/reusable repos; do not copy private `.agents` contents into reusable repos |
| thoughts | `../cn-agents/thoughts`, symlinked as `thoughts/` | canonical planning/research/handoff artifacts | write plan artifacts through the current repo's `thoughts/` symlink |

## DatastarUI staging notes

- `datastarui.workspaces.creative-mode.ai` is a private Caddy workspace route for the sibling `../datastarui` checkout.
- Use it for manual component review when the host config has the `datastarui` configured checkout enabled and the workspace network is running.
- It is the right place to document that DatastarUI is exposed in our Vamos setup. It is not appropriate for DatastarUI's general OSS docs.
- Before manual review, verify the server is running and backed by the expected branch/commit. Locally, also check `http://localhost:4242` when the Docker dev container is the active DatastarUI server.

## Verification pointers

- Vamos runtime/workspaces: `docs/verify.md`, `docs/workspaces-verification.md`
- Vamos development workflow: `docs/vamos-development-workflow.md`
- DatastarUI consumer/upstream workflow from Vamos: `docs/datastarui-development.md`
- DatastarUI story E2E guide in DatastarUI checkout: `../datastarui/docs/e2e-story-testing.md`
