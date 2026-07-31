# Vamos CLI launcher

The global `vamos` command should be a stable launcher binary. It resolves a configured runtime source checkout, fingerprints runtime-relevant source, builds a managed `vamos-runtime` binary in the launcher cache when needed, then execs that runtime.

## Install or repair

Build the launcher, not `cmd/vamos-runtime`, into your PATH:

```bash
go build -o ~/.local/bin/vamos ./cmd/vamos-launcher
vamos launcher configure --runtime-source-root /absolute/path/to/vamos
vamos launcher doctor
(cd /tmp && vamos hermes pi start --help)
```

For dogfood installs, point `--runtime-source-root` at the editable working checkout and promote tested commits to a clean baseline afterward. Stable installations may point directly at a clean baseline. For isolated feature checkout development and tests, set `VAMOS_PACKAGE_ROOT=/absolute/path/to/checkout` to override persisted launcher state temporarily.

## State and cache overrides

- `VAMOS_LAUNCHER_CONFIG` overrides the launcher state file path.
- Default state path is `$XDG_STATE_HOME/vamos/launcher.json`, or `~/.local/state/vamos/launcher.json`.
- `VAMOS_LAUNCHER_CACHE` overrides the managed runtime cache directory.
- Default cache path is `$XDG_CACHE_HOME/vamos/launcher`, or `~/.cache/vamos/launcher`.

The state file is JSON:

```json
{
  "runtime_source_root": "/absolute/path/to/vamos-baseline"
}
```

## Freshness behavior

The launcher computes a runtime source fingerprint from `cmd/vamos-runtime`, shared runtime packages, module files, generated-code inputs, and embedded runtime assets. Excluded paths include `.vamos/`, `.build-agents/`, `node_modules/`, `dist/`, `thoughts/`, docs, static assets, and test files.

When the fingerprint changes, the launcher builds a new managed runtime under a per-target lock and atomically installs it. When unchanged, it reuses the existing cached runtime.

## Hermes manager-wake setup

Configure the host without echoing secret values:

```bash
vamos hermes setup \
  --gateway-url <hermes-adapter-base-url> \
  --vamos-url <vamos-url> \
  --ingress-token <manager-wake-ingress-credential> \
  --callback-token <vamos-callback-credential>
```

`--gateway-url` is the Hermes adapter base URL, without an endpoint suffix. Setup and every managed-start preflight verify `GET <base>/health`; manager-wake delivery uses `<base>/vamos/manager-wake`. A trailing slash is normalized before the base URL is saved or injected.

The ingress credential authenticates direct `/vamos/manager-wake` delivery. The callback credential is separate, callback-only, and is never injected into Pi. A managed launch transiently supplies `VAMOS_MANAGER_WAKE_MANAGER_THREAD_ID`, `VAMOS_MANAGER_WAKE_PI_SESSION_ID`, `VAMOS_MANAGER_WAKE_GATEWAY_URL`, and `VAMOS_MANAGER_WAKE_INGRESS_TOKEN`; unmanaged launches supply none of them. URLs and credentials must not enter prompts, Pi custom entries, settlement evidence, attempt records, diagnostics, or logs.

After deploying the removal of opaque-settlement discovery, an operator runs `vamos-runtime hermes cleanup-opaque-settlement-schedules` once. It uses `TEMPORAL_ADDRESS` and optional `TEMPORAL_NAMESPACE`, deletes only schedules with the literal `opaque-settlement-discovery:` prefix, then freshly verifies zero remain. This is an explicit, safely repeatable operator action, never a startup cleanup hook. Do not record credentials, URLs, or settlement contents in the runbook result.
