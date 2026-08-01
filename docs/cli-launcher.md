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

## Hermes exact-session setup and recovery

Configure the adapter without echoing secret values:

```bash
vamos hermes setup \
  --gateway-url <hermes-adapter-base-url> \
  --vamos-url <vamos-url> \
  --ingress-token <exact-session-ingress-credential> \
  --callback-token <legacy-callback-credential>
```

`--gateway-url` is a base URL, not an endpoint URL. Setup first verifies `/health`, then separately performs an authenticated protocol-v1 capability request and requires protocol `1` plus `exact-session-next-turn-v1`. Health alone is not readiness. Capability proves compatibility only; it does not prove enqueue admission or manager processing.

New managed launches inherit the runtime-issued opaque `HERMES_SESSION_ID`. The Go parent owns exact-session notification and keeps endpoints and credentials out of child arguments, environment, evidence, and output. `--vamos-url` and `--callback-token` remain setup compatibility fields for old callback-based host behavior; the new parent notifier does not expose them to the child.

Retry exactly one published settlement with no discovery:

```bash
vamos hermes pi notify \
  --plan <absolute-plan-dir> \
  --pi-session <pi-session-id> \
  --message-id <message-id> \
  [--format text|json]
```

The result distinguishes immutable publication, admission, retryability, and timeout-after-write uncertainty. Admission is not manager execution, and manager execution is not reverse delivery to the still-live child. Recovery does not scan for latest or all settlements, follow a successor, create a schedule or callback, or invoke `pi done`.

The legacy `/vamos/manager-wake` and `manager_thread_id` route remains isolated compatibility behavior only. Do not use its response as exact-session proof. Promotion still requires the separately approved source-tree Hermes TUI round-trip proof; CLI health, capability, publication, and admission are insufficient.
