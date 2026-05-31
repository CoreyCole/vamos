# E2E Story Testing

## Model

- Authored Go Story API tests in `pkg/e2e/tests` are canonical.
- App-owned helpers in `pkg/e2e/vamos` compose auth, fixtures, pages, selectors, and legacy complex steps.
- The old markdown story parser/generator path has been removed; there is no `docs/features` source or `pkg/e2e/generated` package.
- Add new coverage as reviewed Go Story tests and Vamos helpers only.

## Commands

List authored Go Story tests:

```bash
go test ./pkg/e2e/tests -list Test
```

Run authored Go Story browser tests from a registered non-main workspace. If the host serves a thoughts root outside the workspace checkout, set `VAMOS_E2E_THOUGHTS_ROOT` so fixtures create served documents in the same tree the browser reads.

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

## QRSPI `/q-verify` gate

When this guide is the project verification guide, `/q-verify` must list and run the relevant authored Go Story browser scenarios for the touched surface before asking for human testing. Browser E2E must target the same public feature URL the human will test by passing that URL with `--base-url`; a different local server is not equivalent unless the human will test that same server. For Agent Chat, Thoughts chat, URL-state, route, transcript, or QRSPI-next changes, run the focused `durable-session-chat` scenarios that cover freeform replay, freeform refresh/resume, document navigation preserving embedded chat, workspace chat restore/switching, and QRSPI metadata/artifact behavior; add `thoughts-workbench` scenarios when document workbench URL/navigation behavior changed. If managed restart is unavailable, record browser E2E as blocked instead of treating list commands as sufficient.

After automated browser checks pass against the same URL, `/q-verify` must stop and ask the user to manually test the running workspace before it marks verification complete. The prompt must include the workspace URL printed by `vamos ctl workspace restart` / `agentsctl workspace restart` (for example `https://<workspace-slug>.<workspace-domain>/`) and a concise list of flows to inspect. Do not proceed to a complete `verify.md` until the user confirms manual testing passed, or record `needs_human` / `blocked` with the user's findings.

## Safety

- Browser runs with fixtures must use a registered non-main workspace.
- Runtime metadata uses `.vamos/run/workspace.env` and `VAMOS_*` environment variables.
- The runtime refuses canonical main database paths when fixture setup could mutate durable state.
- `e2e review` may emit `needs-human-review` when no Pi visual adapter is available; that is a review handoff, not a deterministic test failure.
- `e2e fix` is bounded to selectors, steps, runtime, authored Go Story tests, and Vamos E2E helpers unless a human explicitly approves wider changes.

## Run artifacts

`vamos e2e run` writes artifacts under `--artifacts-dir` or `.e2e-runs/<run-id>`. When `--plan-dir thoughts/...` is provided, it also writes a QRSPI run index under the plan context directory and links to heavy artifacts instead of copying trace binaries into `thoughts/`.

Keep run manifests, failure summaries, screenshots, HTML snapshots, traces, visual review markdown, and the exact command with implementation handoffs or verification artifacts.
