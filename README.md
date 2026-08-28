# Vamos

Vamos is a deployable, configurable server for a custom software factory. A host-owned thoughts repo configures the deployment and provides its shared artifacts.

Vamos provides one secured server that shares context across a trusted environment. Teams can browse artifacts, discuss them, and run small web applications beside them.

> **Pre-release:** Vamos is under active development and is not ready for general production use. Configuration, APIs, and storage formats can change without compatibility guarantees.

Vamos is for experimental early adopters who can configure, operate, and change their own deployment.

## Artifacts and applets

Vamos serves a shared artifact tree with built-in views for:

- Markdown documents and plan directories
- CSV files
- trusted HTML documents and static applets
- server-backed HTTP applets, including Datastar, Streamlit, Go, and Python applications

Artifact pages support comments. This keeps discussion beside the durable source material.

Agent Chat is a work in progress. It lets users discuss artifacts and plan directories with an agent in the same shared context.

## Thoughts repo and server boundary

Vamos supplies the reusable server runtime. Each deployment uses a host-owned thoughts repo for its content and configuration.

The thoughts repo owns:

- the artifact tree and its backups
- Google OAuth 2.0 credentials and access policy
- public URLs, reverse-proxy rules, and TLS
- applet definitions and trusted source paths
- service management and deployment commands

This boundary keeps organization-specific policy and data out of the reusable Vamos repository.

A thoughts repo can use this structure:

```text
software-factory/
  config/software-factory.yml   # server, access, paths, and applet configuration
  thoughts/                     # Markdown, HTML, CSV, plans, and other artifacts
  deploy/                       # host-specific service wiring
  README.md                     # team operating guide
```

## Experimental quick start

Create a local configuration:

```bash
cp config.example.yml config.local.yml
cp .env.example .env
```

Set the thoughts paths and Google OAuth 2.0 credentials in `config.local.yml`. Then build and start the server:

```bash
export VAMOS_CONFIG=$PWD/config.local.yml
just build --no-restart
go run ./cmd/server
```

Open `http://localhost:4200` and sign in with an allowed Google account. Read the [local quickstart](docs/local-quickstart.md) for the complete procedure.

## Datastar Pro is optional

Set `VAMOS_DATASTAR_PRO_ASSET` when a licensed Datastar Pro bundle is available. You can also place the bundle at `../datastar-pro/datastar-pro-v1.js`.

If the bundle is absent, Vamos loads public Datastar from jsDelivr. It also installs compatibility polyfills for the Pro contracts that Vamos uses.

## Documentation

- [Local quickstart](docs/local-quickstart.md) — configure and run an experimental local server.
- [Host configuration](docs/host-configuration.md) — configure artifacts, access, projects, applets, and deployment settings.
- [HTML applets](docs/html-applets.md) — serve trusted static HTML with optional shared styles and theme support.
- [HTTP applets](docs/http-applets.md) — run Datastar, Streamlit, Go, Python, and other local web servers.
- [Deployment](docs/deployment.md) — configure a persistent shared server.
- [Vamos development workflow](docs/vamos-development-workflow.md) — develop Vamos and its feature branches.
- [Contributing](docs/contributing.md) — build, generate code, and run tests.
