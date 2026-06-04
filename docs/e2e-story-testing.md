# E2E Story Testing

## Model

- Authored Go Story API tests in `pkg/e2e/tests` are canonical.
- DatastarUI owns the flat Story builder, runtime, launcher, artifacts, review, and goldens.
- App-owned helpers in `pkg/e2e/vamos` compose auth, fixtures, pages, selectors, readiness, and expectations.
- The old markdown story parser/generator path has been removed; there is no `docs/features` source or `pkg/e2e/generated` package.
- Add new coverage as reviewed Go Story tests plus typed Vamos helpers only.

## Commands

### Auth for public workspace E2E

Browser E2E authenticates through the app endpoint with a short-lived manager-minted token:

```text
GET /internal/agent-auth/browser-login?purpose=e2e_playwright&token=<minted>&redirect=<path>
```

Configure a manager-issued machine credential once, then mint an E2E token for each run window:

```bash
vamos auth login-machine --manager-url <manager-url> --key-id <id> --secret <secret>
eval "$(vamos auth playwright-env --slug <slug>)"
```

Vamos Go Story auth helpers read `VAMOS_E2E_AUTH_TOKEN` and visit `/internal/agent-auth/browser-login` before scenario steps. Public workspace URLs require a minted token. Run stories with the local `just e2e` recipe; it delegates to `../datastarui/scripts/datastarui.sh` with this checkout's `datastarui-e2e.yml`. The script rebuilds the stable launcher `../datastarui/bin/datastarui` only when launcher sources change. The launcher builds `../datastarui/bin/datastarui-runtime-<hash>` when DatastarUI CLI/E2E sources change, then execs that runtime.

List authored Go Story tests:

```bash
go test ./pkg/e2e/tests -list Test
```

Run authored Go Story browser tests from a registered non-main workspace. If the host serves a thoughts root outside the workspace checkout, set `VAMOS_E2E_THOUGHTS_ROOT` so fixtures create served documents in the same tree the browser reads.

```bash
VAMOS_E2E_THOUGHTS_ROOT=/path/to/host/thoughts \
  just e2e --story durable-session-chat
```

Run a focused scenario:

```bash
just e2e \
  --story durable-session-chat \
  --scenario freeform-chat-started-from-thoughts-root-survives-refresh-and-resume
```

The live QRSPI continuation browser check is intentionally singular and expensive: `agentchat qrspi question completion auto starts research` starts a real Pi/Temporal `question` workflow-node run and asserts the runtime starts `research`. Keep card rendering, reload, and sidebar coverage in fixture stories such as `agentchat qrspi continuation`.

```bash
VAMOS_E2E_QRSPI_PROMPT_OVERRIDE=1 \
  just e2e --base-url <feature-url> --story agentchat-qrspi-question-completion-auto-starts-research
```

Review a completed run against semantic goldens:

```bash
../datastarui/scripts/datastarui.sh e2e review \
  --run .e2e-runs/<run-id> \
  --plan-dir thoughts/<owner>/plans/<plan-dir>
```

Accept semantic goldens only after explicit human approval:

```bash
../datastarui/scripts/datastarui.sh e2e goldens accept \
  --run .e2e-runs/<run-id> \
  --human-approved
```

## QRSPI `/q-verify` gate

When this guide is the project verification guide, `/q-verify` must list and run the relevant authored Go Story browser scenarios for the touched surface before asking for human testing. Browser E2E must target the same public feature URL the human will test by passing that URL with `--base-url`; a different local server is not equivalent unless the human will test that same server. For Agent Chat, Thoughts chat, URL-state, route, transcript, or QRSPI-next changes, run the focused `durable-session-chat` scenarios that cover freeform replay, freeform refresh/resume, document navigation preserving embedded chat, workspace chat restore/switching, and QRSPI metadata/artifact behavior; add `thoughts-workbench` scenarios when document workbench URL/navigation behavior changed. If managed restart is unavailable, record browser E2E as blocked instead of treating list commands as sufficient.

After automated browser checks pass against the same URL, `/q-verify` must stop and ask the user to manually test the running workspace before it marks verification complete. The prompt must include the workspace URL printed by `vamos ctl workspace restart` / `vamos ctl workspace restart` (for example `https://<workspace-slug>.<workspace-domain>/`) and a concise list of flows to inspect. Do not proceed to a complete `verify.md` until the user confirms manual testing passed, or record `needs_human` / `blocked` with the user's findings.

## Safety

- Browser runs with fixtures must use a registered non-main workspace.
- Runtime metadata uses `.vamos/run/workspace.env` and `VAMOS_*` environment variables.
- The runtime refuses canonical main database paths when fixture setup could mutate durable state.
- DatastarUI `e2e review` may emit `needs-human-review`; that is a review handoff, not a deterministic test failure.
- Vamos repair/fix policy is bounded to typed app helpers and authored Go Story tests unless a human explicitly approves wider changes.

## Run artifacts

`just e2e` / `datastarui e2e run` writes browser artifacts under `--artifacts-dir` or the config `artifacts_dir`, grouped by run timestamp, feature, scenario, and viewport.

Keep screenshots, HTML snapshots, traces when present, visual review markdown, and the exact command with implementation handoffs or verification artifacts.
