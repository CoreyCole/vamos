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

Create a manager-issued machine credential once in the manager SQLite DB, store it on the runner, then mint an E2E token for each run window:

```bash
# Manager host.
vamos auth create-machine-key \
  --database-path <manager-agents.db> \
  --manager-url <manager-url> \
  --name e2e-runner \
  --email agent@example.test \
  --slug <slug> \
  --purpose e2e_playwright \
  --purpose verify

# Runner/client machine, using the printed key id and one-time secret.
vamos auth login-machine --manager-url <manager-url> --key-id <id> --secret <secret>
vamos auth status --slug <slug>
eval "$(vamos auth playwright-env --slug <slug>)"
```

Add `--purpose hermes_chat` when the same credential will also be used for `vamos chat`.

Vamos Go Story auth helpers read `VAMOS_E2E_AUTH_TOKEN` and visit `/internal/agent-auth/browser-login` before scenario steps. Public workspace URLs require a minted token. `Authenticate()` uses Playwright `WaitForURL` until navigation leaves `/internal/agent-auth/browser-login` (and `/login`), so feature hosts that return HTTP 200 + meta-refresh (sets cookie) instead of an HTTP redirect do not false-fail at DomContentLoaded. For public HTTPS hosts, set `VAMOS_E2E_MACHINE_PROFILE=todo52-host` and `VAMOS_WORKSPACE_MANAGER_URL=https://main.workspaces.creative-mode.ai` (or re-`eval` `vamos auth playwright-env --profile todo52-host …` per run) so one-shot tokens are not replayed. Run stories with the local `just e2e` recipe; it delegates to `../datastarui/scripts/datastarui.sh` with this checkout's `datastarui-e2e.yml`. The script rebuilds the stable launcher `../datastarui/bin/datastarui` only when launcher sources change. The launcher builds `../datastarui/bin/datastarui-runtime-<hash>` when DatastarUI CLI/E2E sources change, then execs that runtime.

List authored Go Story tests:

```bash
go test ./pkg/e2e/tests -list Test
go test ./pkg/e2e/workbenchv2tests -list Test
```

Run only the Workbench v2 suite:

```bash
../datastarui/scripts/datastarui.sh e2e run --config datastarui-e2e-workbench-v2.yml
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

### Ruby VA profile → slug map (Playwright / human browser)

Machine credentials on the manager enforce `AllowedSlugs`. Minting with the wrong `--profile` for a slug returns **403 `slug not allowed`**. On host `ruby`, use `/home/ruby/.local/bin/vamos-va-mint` so UX/FE agents do not need Lead to hand-mint.

Verified from manager DB (`~/.local/state/vamos/agents.db`) + local profiles (`~/.vamos/credentials.json`):

| Local profile (`--profile`) | Allowed slug(s) | Default email | Host checkout (when present) | Notes |
| --- | --- | --- | --- | --- |
| `todo52-host` | `2026-09-08-10-10-54-agent-memory-observable-context` | `agent@example.test` | `~/cn/chestnut-flake/vamos-2026-09-08_10-10-54_agent-memory-observable-context` | Active feature tip (e.g. `aacb9ca`). Manager key name: `todo-5-2-feature-host`. |
| `ai470-converge-va` | `ai470-leftover-converge`, `ai470-agent-home-sketch` | `vamosagentsai@gmail.com` | `~/cn/chestnut-flake/vamos-ai470-leftover-converge` (converge) / `~/cn/chestnut-flake/vamos-ai470-agent-home-sketch` (sketch) | Converge **and** sketch. Prefer this over sketch-only when either host is fine. |
| `ai470-sketch-va` | `ai470-agent-home-sketch` | `vamosagentsai@gmail.com` | `~/cn/chestnut-flake/vamos-ai470-agent-home-sketch` | Sketch-only key. |

Manager URL: `https://main.workspaces.creative-mode.ai`.

#### One-command mint (ruby)

```bash
# Current active feature host (todo52) — default when no slug/profile given
vamos-va-mint cookie
vamos-va-mint chrome

# Converge
vamos-va-mint cookie ai470-leftover-converge
# or
vamos-va-mint cookie --profile ai470-converge-va

# Sketch (sketch-only profile, or converge VA which also allows sketch)
vamos-va-mint cookie ai470-agent-home-sketch
vamos-va-mint cookie --profile ai470-sketch-va

# Print the map (no secrets)
vamos-va-mint list
```

Env overrides: `VAMOS_VA_PROFILE`, `VAMOS_VA_CHECKOUT`, `VAMOS_VA_EMAIL`, `VAMOS_VA_MANAGER`, `VAMOS_VA_CLI_CHECKOUT`.

Artifacts (mode `600`) under `/tmp`:

- cookie: `/tmp/va-<tag>-thoughts-session.txt` (`tag` = `todo52` \| `converge` \| `sketch` \| …)
- chrome one-shot URL: `/tmp/va-<tag>-chrome-only.url`

**Never paste tokens, cookies, or browser-login URLs that contain tokens into chat.** Read the artifact on the host. Stderr prints DevTools inject tips (`thoughts_session`, host-scoped domain, `httpOnly`/`secure`/`sameSite=None`). `chrome` mode URLs are one-shot — open in Chrome only; do not `curl` them if you still need the unused URL.

When a feature-host VA expires or the profile is missing, remint via `vamos-va-mint` if the local profile still works; otherwise ask **E2E Lead** to recreate/login the machine key. Do not invent slugs from checkout folder names.


### Agent-memory VA fixture Stories (density / BotDMChip / pairwise)

Authored Workbench v2 Stories seed chat chrome via query params on `/threads/{id}`:

| Story name | Query | Asserts |
| --- | --- | --- |
| `agent-memory chat density fixture` | `?density_fixture=1` (or `?fixture=density`) | `#msg-ai470-density-fixture-reasoning` / `tool-0` / `tool-1` as Class A `<details data-chat-density>` |
| `agent-memory group bubble BotDMChip` | `?group_bubble_fixture=1` (or `?fixture=group`) | `#msg-ai470-group-{peer,quote,user}-fixture`, `#bot-dm-chip-ai470-group-quote-fixture`, pairwise hrefs `/rooms/a2a/infra/lead` + `/rooms/a2a/lead/research` |
| `agent-memory pairwise fixture view-only` | `?pairwise_fixture=1` (or `?fixture=pairwise`) | composer absent; `#msg-ai470-chroma-highlight-fixture` |

Helpers live in `pkg/e2e/vamos/agent_memory_fixtures.go`. Stories: `pkg/e2e/workbenchv2tests/agent_memory_fixtures_story_test.go`. Default thread is WorkbenchV2 `wb2_alpha`; override with `VAMOS_E2E_AGENT_MEMORY_THREAD_ID` for tip-host dogfood (example `65f8ec8e-02c0-43bd-8cd4-fe3638178221`).

```bash
# Managed/local (fixture DB + wb2_alpha)
just e2e --config datastarui-e2e-workbench-v2.yml \
  --story agent-memory-chat-density-fixture

just e2e --config datastarui-e2e-workbench-v2.yml \
  --story agent-memory-group-bubble-botdmchip

just e2e --config datastarui-e2e-workbench-v2.yml \
  --story agent-memory-pairwise-fixture-view-only

# Tip host (density live on 49da083+; auth via todo52-host profile — never paste tokens)
VAMOS_E2E_MACHINE_PROFILE=todo52-host \
VAMOS_E2E_AGENT_MEMORY_THREAD_ID=65f8ec8e-02c0-43bd-8cd4-fe3638178221 \
  just e2e --base-url https://2026-09-08-10-10-54-agent-memory-observable-context.workspaces.creative-mode.ai \
  --no-restart --story agent-memory-chat-density-fixture
```

If a tip host is still density-only, set `VAMOS_E2E_AGENT_MEMORY_ALLOW_PENDING_GATES=1` to skip group/pairwise Stories until the BE query gates for `group_bubble_fixture` / `pairwise_fixture` are tipped.

### Portable applet smoke stories

Mint and export fresh browser auth as described above immediately before each command, then run these reusable stories against the exact local server under test. Point `VAMOS_E2E_THOUGHTS_ROOT` at that server's thoughts root; checkout-local verification servers normally use `.vamos/state/thoughts`. `--no-restart` keeps the runner on the explicitly supplied external server instead of invoking the configured managed-server command.

```bash
VAMOS_E2E_THOUGHTS_ROOT="$PWD/.vamos/state/thoughts" \
  just e2e --base-url http://127.0.0.1:49231 --no-restart \
  --story static-html-applet-embedded

VAMOS_E2E_THOUGHTS_ROOT="$PWD/.vamos/state/thoughts" \
  just e2e --base-url http://127.0.0.1:49231 --no-restart \
  --story static-html-applet-standalone

just e2e --base-url http://127.0.0.1:49231 --no-restart \
  --story wordle-applet-smoke
```

The embedded story checks initial hidden feedback, wrong-only feedback, correct-only feedback, opaque sandboxing, and console cleanliness. The standalone story opens the same seeded HTML unchanged through `file://` while retaining the explicit base-URL contract for auth and fixture setup. The Wordle story proves the proxied app renders instead of a stale proxy response, logs in, submits a valid guess through the UI, observes durable board state, and leaves a clean console.

The live QRSPI continuation browser check is intentionally singular and expensive: `agentchat qrspi question completion auto starts design` starts a real Pi/Temporal `question` workflow-node run with legacy `assisted`/autopilot, asserts the runtime starts `research`, lets the real `research` run follow a seeded fast-path fixture, and asserts the runtime starts `design`. Once design is current, the story downgrades the workflow policy to `discuss` so later QRSPI nodes do not auto-start, then verifies no `workspace` node run or sibling implementation checkout was created. The generated Q-to-D plan fixture lives under the served thoughts root (`VAMOS_E2E_THOUGHTS_ROOT` when set, otherwise the checkout `thoughts/`) so plan-workspace validation passes; it is removed at test cleanup unless `VAMOS_E2E_QRSPI_PRESERVE_FIXTURE=1` is set for debugging. Keep card rendering, reload, and sidebar coverage in fixture stories such as `agentchat qrspi continuation`.

```bash
VAMOS_E2E_QRSPI_PROMPT_OVERRIDE=1 \
  just e2e --base-url <feature-url> --story agentchat-qrspi-question-completion-auto-starts-design
```

### Workspaces page stories on feature child hosts

Feature child servers serve a read-only Workspaces page so browser stories exercise feature-branch Workspaces page code directly. Full lifecycle authority stays on the manager checkout: child hosts mount only `/workspaces` and `/workspaces/stream` from the feature checkout with workspace-local fixture DB rows selected by `.vamos/run/workspace.env`; lifecycle/provision/release/cleanup mutation routes remain unavailable. Public browser E2E still needs `VAMOS_E2E_AUTH_TOKEN` when the host requires Playwright auth.

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
- 403 `slug not allowed` means the Playwright machine profile's AllowedSlugs does not include the target slug — see the Ruby VA profile → slug map above; remint with the matching profile via `vamos-va-mint`.
- Runtime metadata uses `.vamos/run/workspace.env` and `VAMOS_*` environment variables.
- The runtime refuses canonical main database paths when fixture setup could mutate durable state.
- DatastarUI `e2e review` may emit `needs-human-review`; that is a review handoff, not a deterministic test failure.
- Vamos repair/fix policy is bounded to typed app helpers and authored Go Story tests unless a human explicitly approves wider changes.

## Run artifacts

To inspect old leaked Q-to-D fixtures before manual cleanup:

```bash
find thoughts/creative-mode-agent/plans -maxdepth 1 -type d -name 'e2e-qrspi-q-to-d-*' -print
find .. -maxdepth 1 -type d \( -name 'vamos-*e2e-qrspi-q-to-d*' -o -name 'vamos-e2e-qrspi-q-to-d-*' \) -print
```

Only delete generated `e2e-qrspi-q-to-d-*` fixture directories/checkouts after confirming no human needs them for debugging. Never match/delete the real plan directory `2026-06-17_10-52-01_e2e-qrspi-q-to-d-boundary`.

`just e2e` / `datastarui e2e run` writes browser artifacts under `--artifacts-dir` or the config `artifacts_dir`, grouped by run timestamp, feature, scenario, and viewport.

Keep screenshots, HTML snapshots, traces when present, visual review markdown, and the exact command with implementation handoffs or verification artifacts.

## See also

- Workbench V2 under-tabs chrome regression Story: `workbench-v2-mobile-sibling-doc-keeps-chrome-under-tabs` in `pkg/e2e/workbenchv2tests/workbench_v2_doc_vt_e2e_test.go` — best-practices guide in `docs/workbench-view-transitions.md`.
