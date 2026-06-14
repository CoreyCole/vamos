# Contributing

## Build loop

Use the repository build wrapper for generated assets and compile checks without restarting a configured running host:

```bash
just build --no-restart
```

Use plain `just build` only when you intentionally want the configured host restart behavior.

## Generated files

When modifying `.templ` files, run:

```bash
templ generate
```

Then run the relevant Go tests and build command. Do not hand-edit generated templ output.

## Useful tests

For layout, workspace discovery, and build-asset changes, start with:

```bash
go test ./server/layouts ./server/services/workspaces ./cmd/build-agents/internal/build
```

For config-related changes, include:

```bash
go test ./server/config
```

Use `go test ./...` when a change crosses package boundaries or affects shared runtime behavior.

## Datastar Pro asset

Datastar Pro is optional for OSS development. Licensed assets remain gitignored. If a licensed bundle is available, set `VAMOS_DATASTAR_PRO_ASSET`; otherwise the browser falls back to public Datastar plus Vamos compatibility polyfills.

## DatastarUI

Shared UI primitives come from `github.com/coreycole/datastarui`. Prefer those primitives before bespoke app UI. Reusable primitive fixes belong upstream in DatastarUI; Vamos owns app-specific composition.

## Story E2E

Story E2E uses authored Go Story API tests under `pkg/e2e/tests`.

```bash
just e2e --base-url <feature-url> --story <story>
```

Use E2E from a registered non-main workspace when fixtures are enabled. Keep story definitions in Go; do not add markdown story generators.

## Local maintainer dogfood docs

Local dogfood topology is documented separately in `docs/vamos-manifest.md` and is not OSS setup. Maintainers working in the repository owner's dogfood host can read that manifest and `docs/vamos-development-workflow.md`; new OSS users do not need those docs for local setup.
