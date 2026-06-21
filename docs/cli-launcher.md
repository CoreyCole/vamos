# Vamos CLI launcher

The global `vamos` command should be a stable launcher binary. It resolves a configured runtime source checkout, fingerprints runtime-relevant source, builds a managed `vamos-runtime` binary in the launcher cache when needed, then execs that runtime.

## Install or repair

Build the launcher, not `cmd/vamos-runtime`, into your PATH:

```bash
go build -o ~/.local/bin/vamos ./cmd/vamos-launcher
vamos launcher configure --runtime-source-root /absolute/path/to/vamos-baseline
vamos launcher doctor
(cd /tmp && vamos qrspi --help)
```

For dogfood-style installs, point `--runtime-source-root` at your clean baseline checkout. For feature checkout development and tests, set `VAMOS_PACKAGE_ROOT=/absolute/path/to/checkout` to override persisted launcher state temporarily.

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
