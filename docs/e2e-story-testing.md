# E2E Story Testing

## Model

- Story specs in `docs/features/*.story.md` are canonical.
- Generated Go tests in `pkg/e2e/generated` are derived from those stories and checked in.
- Generated tests call `pkg/e2e/runtime` and `pkg/e2e/steps`; do not hand-edit generated tests or add raw Playwright selectors there.
- The step catalog in `pkg/e2e/steps` and selector catalog in `pkg/e2e/selectors` are the controlled extension points.

## Commands

Validate stories and catalogs:

```bash
go run ./cmd/vamos-runtime e2e check
```

Regenerate checked-in tests, or verify they are fresh:

```bash
go run ./cmd/vamos-runtime e2e generate
go run ./cmd/vamos-runtime e2e generate --check
```

Run generated browser tests from a registered non-main workspace. If the host serves a thoughts root outside the workspace checkout, set `VAMOS_E2E_THOUGHTS_ROOT` so fixtures create served documents in the same tree the browser reads.

```bash
VAMOS_E2E_THOUGHTS_ROOT=/path/to/host/thoughts \
go run ./cmd/vamos-runtime e2e run \
  --story durable-session-chat \
  --plan-dir thoughts/<owner>/plans/<plan-dir>
```

Run a focused scenario:

```bash
go run ./cmd/vamos-runtime e2e run \
  --story durable-session-chat \
  --scenario freeform-chat-started-from-thoughts-root-survives-refresh-and-resume \
  --plan-dir thoughts/<owner>/plans/<plan-dir>
```

Review a completed run against semantic goldens:

```bash
go run ./cmd/vamos-runtime e2e review \
  --run .e2e-runs/<run-id> \
  --plan-dir thoughts/<owner>/plans/<plan-dir>
```

Accept semantic goldens only after explicit human approval:

```bash
go run ./cmd/vamos-runtime e2e goldens accept \
  --run .e2e-runs/<run-id> \
  --human-approved
```

## Safety

- Browser runs with fixtures must use a registered non-main workspace.
- Runtime metadata uses `.vamos/run/workspace.env` and `VAMOS_*` environment variables.
- The runtime refuses canonical main database paths when fixture setup could mutate durable state.
- `e2e review` may emit `needs-human-review` when no Pi visual adapter is available; that is a review handoff, not a deterministic test failure.
- `e2e fix` is bounded to selectors, steps, runtime, and generated tests unless a human explicitly approves wider changes.

## Run artifacts

`vamos e2e run` writes artifacts under `--artifacts-dir` or `.e2e-runs/<run-id>`. When `--plan-dir thoughts/...` is provided, it also writes a QRSPI run index under the plan context directory and links to heavy artifacts instead of copying trace binaries into `thoughts/`.

Keep run manifests, failure summaries, screenshots, HTML snapshots, traces, visual review markdown, and the exact command with implementation handoffs or verification artifacts.
