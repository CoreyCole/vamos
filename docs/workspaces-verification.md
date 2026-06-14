# Workspace Verification

## Purpose

`vamos ctl verify workspaces` proves multi-checkout workspace lifecycle from both sides:

- server-owned lifecycle, process, metadata, proxy, logs, runtime env snapshot, and worker identity truth
- client-visible DNS, TLS, Caddy/public-host routing, manager auth, browser switch handoff, and unavailable-after-stop behavior
- optional child-local Agent Chat probe proof for callback/snapshot/cwd isolation

Use it before claiming a workspace checkout can be switched to, restarted, inspected, and reached at its public host.

## Required environment

Manager `.env` must provide:

```dotenv
VAMOS_WORKSPACE_MODE=manager
VAMOS_WORKSPACE_DOMAIN=vamos.test
VAMOS_PUBLIC_BASE_URL=https://main.vamos.test
VAMOS_WORKSPACE_RESTART_TOKEN=...
VAMOS_PLAYWRIGHT_AUTH_ENABLED=true
```

For browser verification, create a manager machine credential in the manager SQLite DB, then store it on the verifier/client machine:

```bash
# Run on the manager host, where the manager SQLite DB is reachable.
vamos auth create-machine-key \
  --database-path <manager-agents.db> \
  --manager-url https://main.vamos.test \
  --name laptop \
  --email agent@example.test \
  --slug <slug> \
  --purpose e2e_playwright \
  --purpose hermes_chat \
  --purpose verify

# Run on the verifier/client machine, using the printed key id and one-time secret.
vamos auth login-machine --manager-url https://main.vamos.test --key-id <id> --secret <secret>
vamos auth status --slug <slug>
eval "$(vamos auth playwright-env --slug <slug>)"
```

When restricting purposes, include `--purpose verify` for `vamos auth status`.

External setup must also be in place:

- Caddy terminates wildcard HTTPS for `*.vamos.test` and proxies to the manager HTTP server.
- Caddy preserves the original `Host` header so manager host-dispatch can route `main` and child workspace hosts.
- Tailnet DNS resolves `main.vamos.test` and `<slug>.vamos.test` to the manager host from the verifier machine.
- Verifier machine trusts the Caddy internal CA if Caddy uses `tls internal`.
- A sibling checkout exists for the target slug.

## Main vs feature workspace rule

`main.<domain>` is reserved for the canonical manager checkout. Feature branches and QRSPI implementation copies must be started through the manager and tested at their derived slug host (`https://<slug>.<domain>/`). Do not manually run a feature checkout on the manager port to "take over" `main`; use `/workspaces`, `vamos ctl workspace restart`, or `just build` from a managed child checkout so lifecycle stays owned by `server/services/workspaces/`.

Before sending a feature workspace to a human for manual testing, make the child runtime current and reachable. A build with `--no-restart` proves compilation only; it can leave the public feature host serving the previous process or the manager recovery page. Run a managed restart (`just build` from the feature checkout, or the manager restart action), then verify the public feature URL reaches the child app before handing it off.

## Status source model

Agents must keep workspace status sources separate:

- **Manager DB lifecycle / implementation workspace record**: authoritative for active, merged, cleaned-up, merge proof, and cleanup proof.
- **Scheduled sync diagnostics**: latest filesystem-to-DB indexing result, counts, errors, and warnings. This is separate from `just sync-thoughts`, which syncs documentation/artifacts and does not rebuild manager DB lifecycle state.
- **Local runtime diagnostics**: checkout-local `.vamos/run/status.json`, `desired.json`, `runtime-env.json`, and logs. Useful for current child process/build debugging and human-test readiness evidence; not authoritative for merge or cleanup lifecycle.

`just build` from a managed checkout and the manager Workspaces page show source-labeled lifecycle, scheduled sync, and local runtime diagnostics when the manager is reachable. Local-only CLI commands can read `.vamos/run/*`, but they cannot prove manager DB lifecycle.

## Feature Workspaces page read-only mode

Feature child hosts are still not real workspace managers. They mount only the read-only Workspaces page and Datastar stream (`/workspaces`, `/workspaces/stream`) backed by the workspace-local database selected by `.vamos/run/workspace.env`. Real lifecycle/provision/release/cleanup actions remain manager-owned and unavailable on child Workspaces routes.

## Session ownership during verification

Browser and chat sessions created while verifying `work` or feature workspace hosts belong to that workspace's disposable `.vamos` DB. Pi CLI sessions created from those directories may remain in `~/.pi/agent/sessions` after cleanup. Main may index or import them intentionally, but verification noise is not durable main chat history by default. Summarize important evidence into `verify.md`, review notes, screenshots, logs, or release records. See `docs/vamos-development-workflow.md`.

## Commands

Browser-enabled workspace verification uses the DatastarUI Go Story E2E runner and authenticates through a short-lived manager-minted browser token:

```text
GET /internal/agent-auth/browser-login?purpose=e2e_playwright&token=<minted>&redirect=<path>
```

Normal shell flow:

```bash
# Manager host: create the durable machine key in the manager SQLite DB.
vamos auth create-machine-key \
  --database-path <manager-agents.db> \
  --manager-url https://main.<domain> \
  --name laptop \
  --email agent@example.test \
  --slug <slug> \
  --purpose e2e_playwright \
  --purpose verify

# Verifier/client machine: store the printed one-time secret and mint per-run auth.
vamos auth login-machine --manager-url https://main.<domain> --key-id <id> --secret <secret>
vamos auth status --slug <slug>
eval "$(vamos auth playwright-env --slug <slug>)"
just verify-workspaces slug=<slug> start=true restart=true stop=true browser=true
```

Add `--purpose hermes_chat` when the same credential will also back `vamos chat`. `vamos ctl verify workspaces --browser=true` also attempts to mint an `e2e_playwright` token from the stored machine profile when no env token is present. If minting cannot run, it fails with guidance to create a key on the manager, run `vamos auth login-machine`, and run `vamos auth playwright-env`; do not hunt for host secrets.

From the Vamos repo root:

```bash
just build --no-restart
just verify-workspaces slug=multi-checkout-dev-workspaces start=true restart=true stop=true browser=true
just verify-workspaces slug=multi-checkout-dev-workspaces start=true restart=true stop=true browser=true agent_chat_probe=true
```

## Fast workspace DB invariant gate

For fast local/QRSPI phase checks, run the DB-only verifier against the current checkout database:

```bash
just verify-workspace-db db=.vamos/run/agents.db
# or
scripts/workspace-db-verify/verify.sh --database-path .vamos/run/agents.db --format text
```

This gate checks current SQLite projection invariants only:

- `PRAGMA foreign_keys` is enabled on the verifier connection
- `PRAGMA foreign_key_check` has no rows
- no duplicate `impl_workspaces.checkout_path`
- no `impl_workspaces.plan_dir_rel` without a `plan_workspaces` target
- no active binding rows with missing plan target
- no active binding rows with missing impl workspace target
- no more than one active primary project row per plan
- protected `main`/`stage` rows are not `merged` or `cleaned_up`

It does not start Temporal, trigger scheduled sync, restart a workspace, or run browser checks. Scheduled-sync success and `SQLITE_BUSY` recovery classification belong to E2E/deploy verification such as `/vamos-merge`.

## Deploy-time workspace sync health

Deploy verification must prove both HTTP route health and workspace projection health. `/login` returning 200 is not enough.

After restarting a stage/main manager, verify:

```bash
scripts/workspace-db-verify/verify.sh --database-path <agents.db> --format text
rg -n "workspace_sync_refresh_complete|workspace sync.*complete|SyncWorkspaces" <fresh-log-window>
rg -n "FOREIGN KEY constraint failed|UNIQUE constraint failed|SQLITE_BUSY|database is locked" <fresh-log-window>
```

Rules:

- FK/UNIQUE/projection verifier failures block merge/approval.
- Fresh `FOREIGN KEY constraint failed` or `UNIQUE constraint failed` logs block merge/approval.
- `SQLITE_BUSY` / `database is locked` is tolerated only when the same fresh window shows later successful workspace sync evidence.
- If no fresh sync success evidence exists, treat deploy verification as blocked even when HTTP smoke checks pass.

For a feature branch handoff to a human tester, use the feature checkout root:

```bash
# 1. Compile without disturbing the running child while agent verification is still in progress.
just build --no-restart

# 2. When ready for human testing, restart the managed child runtime for this checkout.
just build

# 3. Confirm public routing reaches the child app, not workspace recovery.
domain=<domain> # for example: vamos.test
slug=$(grep '^VAMOS_WORKSPACE_SLUG=' .vamos/run/workspace.env | cut -d= -f2- | tr -d "'\"")
curl -k -sS -D - --max-time 15 "https://${slug}.${domain}/" -o /tmp/vamos-feature-home.html

# Expected unauthenticated result is usually HTTP 307 to /login?redirect=%2F.
# A 503 Workspace recovery page means the feature runtime is not ready for human testing.
```

To keep artifacts with a plan or review:

```bash
just verify-workspaces slug=multi-checkout-dev-workspaces start=true restart=true stop=true browser=true \
  report=thoughts/<owner>/plans/<plan-dir>/reviews/<review-dir>/artifacts/$(date +%Y-%m-%d_%H-%M-%S)_multi-checkout-dev-workspaces
```

Equivalent nested command:

```bash
cd vamos
go run ./cmd/vamos-runtime ctl verify workspaces \
  --env .env \
  --base-url https://main.vamos.test \
  --domain vamos.test \
  --slug multi-checkout-dev-workspaces \
  --start=true --restart=true --stop=true --browser=true --agent-chat-probe=true
```

## Verification layers

| Layer | What it proves |
| --- | --- |
| `workspace-db` | Current SQLite workspace projection has valid FKs, checkout identity, bindings, primary-project rows, and protected lane status. |
| `config` | Required env/flags are present and internally consistent. |
| `dns` | Manager and child public hostnames resolve from the verifier machine. |
| `tls` | Public HTTPS reaches a TLS terminator, not plain Go HTTP. |
| `caddy` | Public HTTPS routing/proxy path reaches the manager and child hosts. |
| `auth` | Restart-token APIs and minted browser verifier auth are accepted. |
| `lifecycle` | Manager can start, restart, and stop the child process. |
| `metadata` | Workspace metadata, runtime env snapshot, and TS worker identity marker are written/read and match the target slug/checkout/ports. |
| `logs` | Manager/child log tails can be captured for diagnostics. |
| `proxy` | Manager host-dispatch proxies the child host while running. |
| `handoff` | Manager switch redirects through signed handoff to the child workspace. |
| `agentchat` | Optional child-local Agent Chat probe proves workflow input callback/snapshot endpoints and cwd use the child web server/checkout. |
| `browser` | Playwright reaches the child public host and later sees unavailable state after stop. |

## Artifacts

Reports are written under the requested `--report` directory, or `tmp/workspace-verification/<timestamp>_<slug>` by default. Expected files include:

- `summary.md` and `summary.json`
- `server-runs.json`; when `agent_chat_probe=true`, each run may include `agent_chat_probe` with run ID, workflow ID, callback endpoint, snapshot endpoint, cwd, Temporal address, TS worker PID, and reached snapshot/callback booleans
- `dns-main.txt` and `dns-child.txt`
- public HTTPS/curl probe output files
- `manager-log-tail.txt` and `child-log-tail.txt` when available
- `datastarui-e2e-output.txt`
- `datastarui-e2e-runs/<run-id>/summary.json`, `index.html`, screenshots/traces, and per-job artifacts from the `workspace-public-switch` / `workspace-public-unavailable` Go stories
- child `.vamos/run/runtime-env.json` and TS worker ready marker are surfaced through server diagnostics in `server-runs.json`

Include the report path and first failed layer in handoffs/reviews. Do not claim full acceptance without a passing browser-enabled run.

## Common failures

- **DNS**: `lookup main.vamos.test: no such host` means split DNS/wildcard DNS is not configured for the verifier machine. Configure Tailnet DNS so `*.vamos.test` resolves to the manager host.
- **TLS**: `server gave HTTP response to HTTPS client`, certificate errors, or protocol errors mean HTTPS is not terminating correctly or the verifier does not trust Caddy's CA.
- **Caddy/proxy**: TLS succeeds but public host returns the wrong app, 404, or unavailable while the child is running. Check wildcard site config, upstream address, and Host preservation.
- **Auth**: 401/403 from internal endpoints usually means the restart token is wrong or the machine credential/profile cannot mint the scoped browser token.
- **Lifecycle**: start/restart/stop failures or stale PID/port checks point to child process startup, state directory, env override, or port allocation problems.
- **Metadata/logs**: missing metadata/log tails, runtime env snapshot, or TS worker identity marker indicate the child did not boot far enough or state/log paths are not isolated per workspace.
- **Agent Chat probe**: `localhost:4200` callback/snapshot URLs, wrong cwd, missing internal token, or failed snapshot/callback proof mean the child web process did not build or accept child-local workflow endpoints. Check `VAMOS_INTERNAL_TOKEN`, `.vamos/run/runtime-env.json`, Agent Chat callback base, and child web logs.
- **Handoff/browser**: manager auth works but switch does not land on `https://<slug>.vamos.test/`. Inspect manager switch logs, child `/internal/dev-auth/handoff`, cookie domain/security, and browser screenshots.

## Feature workspace human-testing readiness

Before telling a human to test a feature URL, record these checks in `verify.md` or the handoff:

- `just build` (without `--no-restart`) completed from the feature checkout after the final committed changes.
- The feature URL `https://<slug>.<domain>/` returns the child app. For auth-protected deployments, an unauthenticated `307` to `/login?redirect=%2F` is healthy.
- The feature URL does **not** return the manager Workspace recovery page or HTTP `503`.
- Local runtime diagnostics in `.vamos/run/runtime-env.json` have `workspace_slug`, `checkout_path`, `database_path`, and `default_cwd` for the feature checkout, not `stage`, `main`, or another checkout.
- Local runtime diagnostics in `.vamos/run/status.json` have `status: running` and log paths under the feature checkout.
- Recent `web.log` includes the current child `server_startup` line with the feature slug and public base URL.

If the public URL shows Workspace recovery while local runtime diagnostics say running, suspect stale runtime metadata or a child process from another checkout. Restart via `just build` from the feature checkout or the manager restart action, then re-check the URL and runtime env. Do not ask for human testing until the public host reaches the child app.

The workspace error queue may include old Temporal shutdown/restart warnings such as `context canceled`, `connect: connection refused`, `graceful_stop`, or `max_age`. Treat them as diagnostic context, not current blockers, when they predate the latest successful child startup and the public URL is healthy. Current `web.log`, `.vamos/run/runtime-env.json`, and the public feature URL are required human-test readiness evidence.

## Acceptance rule

A complete acceptance run must pass all configured layers with the manager serving `main.<domain>` and the target branch served from its own slug host:

```bash
just verify-workspaces slug=<slug> start=true restart=true stop=true browser=true
```

Isolation-hardening acceptance for callback/snapshot/cwd proof additionally requires:

```bash
just verify-workspaces slug=<slug> start=true restart=true stop=true browser=true agent_chat_probe=true
```

If DNS/TLS/Caddy are not configured on the current host, record the layer-specific failure and report path, but leave final E2E acceptance open.
