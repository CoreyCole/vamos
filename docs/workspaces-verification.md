# Workspace Verification

## Purpose

`agentsctl verify workspaces` proves multi-checkout workspace lifecycle from both sides:

- server-owned lifecycle, process, metadata, proxy, and log truth
- client-visible DNS, TLS, Caddy/public-host routing, manager auth, browser switch handoff, and unavailable-after-stop behavior

Use it before claiming a workspace checkout can be switched to, restarted, inspected, and reached at its public host.

## Required environment

Manager `.env` must provide:

```dotenv
VAMOS_WORKSPACE_MODE=manager
VAMOS_WORKSPACE_DOMAIN=vamos.test
VAMOS_PUBLIC_BASE_URL=https://main.vamos.test
VAMOS_WORKSPACE_RESTART_TOKEN=...
VAMOS_PLAYWRIGHT_AUTH_ENABLED=true
VAMOS_PLAYWRIGHT_AUTH_TOKEN=...
```

External setup must also be in place:

- Caddy terminates wildcard HTTPS for `*.vamos.test` and proxies to the manager HTTP server.
- Caddy preserves the original `Host` header so manager host-dispatch can route `main` and child workspace hosts.
- Tailnet DNS resolves `main.vamos.test` and `<slug>.vamos.test` to the manager host from the verifier machine.
- Verifier machine trusts the Caddy internal CA if Caddy uses `tls internal`.
- A sibling checkout exists for the target slug, for example `vamos-2026-05-10_05-14-35_multi-checkout-dev-workspaces` -> slug `multi-checkout-dev-workspaces`.

## Main vs feature workspace rule

`main.<domain>` is reserved for the canonical manager checkout. Feature branches and QRSPI implementation copies must be started through the manager and tested at their derived slug host (`https://<slug>.<domain>/`). Do not manually run a feature checkout on the manager port to "take over" `main`; use `/workspaces`, `agentsctl workspace restart`, or `just build` from a managed child checkout so lifecycle stays owned by `server/services/workspaces/`.

## Commands

From the Vamos repo root:

```bash
just build --no-restart
just verify-workspaces slug=multi-checkout-dev-workspaces start=true restart=true stop=true browser=true
```

To keep artifacts with a plan or review:

```bash
just verify-workspaces slug=multi-checkout-dev-workspaces start=true restart=true stop=true browser=true \
  report=thoughts/<owner>/plans/<plan-dir>/reviews/<review-dir>/artifacts/$(date +%Y-%m-%d_%H-%M-%S)_multi-checkout-dev-workspaces
```

Equivalent nested command:

```bash
cd vamos
go run ./cmd/agentsctl verify workspaces \
  --env .env \
  --base-url https://main.vamos.test \
  --domain vamos.test \
  --slug multi-checkout-dev-workspaces \
  --start=true --restart=true --stop=true --browser=true
```

## Verification layers

| Layer | What it proves |
| --- | --- |
| `config` | Required env/flags are present and internally consistent. |
| `dns` | Manager and child public hostnames resolve from the verifier machine. |
| `tls` | Public HTTPS reaches a TLS terminator, not plain Go HTTP. |
| `caddy` | Public HTTPS routing/proxy path reaches the manager and child hosts. |
| `auth` | Restart-token APIs and Playwright verifier auth are accepted. |
| `lifecycle` | Manager can start, restart, and stop the child process. |
| `metadata` | Workspace metadata is written/read and matches the target slug. |
| `logs` | Manager/child log tails can be captured for diagnostics. |
| `proxy` | Manager host-dispatch proxies the child host while running. |
| `handoff` | Manager switch redirects through signed handoff to the child workspace. |
| `browser` | Playwright reaches the child public host and later sees unavailable state after stop. |

## Artifacts

Reports are written under the requested `--report` directory, or `tmp/workspace-verification/<timestamp>_<slug>` by default. Expected files include:

- `summary.md` and `summary.json`
- `server-runs.json`
- `dns-main.txt` and `dns-child.txt`
- public HTTPS/curl probe output files
- `manager-log-tail.txt` and `child-log-tail.txt` when available
- `playwright-output.txt`
- `screenshots/manager-workspaces.png`, `screenshots/child-app.png`, `screenshots/unavailable-after-stop.png`
- `playwright-trace.zip` when the browser verifier reaches tracing setup

Include the report path and first failed layer in handoffs/reviews. Do not claim full acceptance without a passing browser-enabled run.

## Common failures

- **DNS**: `lookup main.vamos.test: no such host` means split DNS/wildcard DNS is not configured for the verifier machine. Configure Tailnet DNS so `*.vamos.test` resolves to the manager host.
- **TLS**: `server gave HTTP response to HTTPS client`, certificate errors, or protocol errors mean HTTPS is not terminating correctly or the verifier does not trust Caddy's CA.
- **Caddy/proxy**: TLS succeeds but public host returns the wrong app, 404, or unavailable while the child is running. Check wildcard site config, upstream address, and Host preservation.
- **Auth**: 401/403 from internal endpoints usually means `VAMOS_WORKSPACE_RESTART_TOKEN` or `VAMOS_PLAYWRIGHT_AUTH_TOKEN` differs between CLI and manager environment.
- **Lifecycle**: start/restart/stop failures or stale PID/port checks point to child process startup, state directory, env override, or port allocation problems.
- **Metadata/logs**: missing metadata/log tails indicate the child did not boot far enough or state/log paths are not isolated per workspace.
- **Handoff/browser**: manager auth works but switch does not land on `https://<slug>.vamos.test/`. Inspect manager switch logs, child `/internal/dev-auth/handoff`, cookie domain/security, and browser screenshots.

## Acceptance rule

A complete acceptance run must pass all configured layers with the manager serving `main.<domain>` and the target branch served from its own slug host:

```bash
just verify-workspaces slug=<slug> start=true restart=true stop=true browser=true
```

If DNS/TLS/Caddy are not configured on the current host, record the layer-specific failure and report path, but leave final E2E acceptance open.
