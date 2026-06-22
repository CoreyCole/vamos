# Vamos Agents

Vamos is a reusable Go/templ/Datastar runtime for private agentic software factories. It serves a host-owned `thoughts/` knowledge tree, shared Agent Chat/Pi sessions, QRSPI planning workflows, and optional workspace/release operations.

## Quick start

For a local first run, follow [Local quickstart](docs/local-quickstart.md). The quickstart uses Google OAuth with one whitelisted developer email so local setup exercises the same auth boundary as hosted deployments.

```bash
cp config.example.yml config.local.yml
cp .env.example .env
export VAMOS_CONFIG=$PWD/config.local.yml
just build --no-restart
```

## Runtime and host boundary

Vamos is the reusable runtime. A host repository or host deployment owns organization-specific data and policy:

- `thoughts/` storage and backups
- OAuth credentials, allowed domains, whitelisted emails, and webhook secrets
- public base URLs, CORS origins, reverse proxy, TLS, and workspace domains
- linked project checkout paths and clean baseline checkout policy
- deploy service names, restart hooks, and release lane commands

A typical host repo keeps private config beside durable artifacts:

```text
company-agents/
  config/company-agents.yml      # host-owned OAuth, paths, domains, projects
  thoughts/                      # plans, research, ADRs, handoffs
  systemd/ or deploy/            # host-specific service wiring
  README.md                      # team-specific operating guide
```

## Datastar Pro is optional

If a licensed Datastar Pro bundle is available, set `VAMOS_DATASTAR_PRO_ASSET` or place it at `../datastar-pro/datastar-pro-v1.js`. If it is absent, Vamos loads public Datastar from jsDelivr and installs small compatibility polyfills for the Pro contracts Vamos uses.

## Documentation

- [Local quickstart](docs/local-quickstart.md) — first run with Google OAuth.
- [Concepts](docs/concepts.md) — runtime, host, thoughts, Agent Chat, QRSPI, workspaces.
- [Host configuration](docs/host-configuration.md) — map YAML/env fields to host responsibilities.
- [Deployment](docs/deployment.md) — production host checklist.
- [Contributing](docs/contributing.md) — build, tests, generated files, E2E pointers.
- [HTML Applets](docs/html-applets.md) — trusted `.html` Thoughts documents, shared CSS, theme helper, and sandbox boundary.
