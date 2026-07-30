---
name: q-hermes-manager
description: Guide Hermes-managed isolated Pi workers for QRSPI artifacts.
---

# Hermes QRSPI manager

Hermes owns conversation state, background task handles, process-write steering, and the decision to launch a successor. Vamos provides durable plan artifacts and the thin worker CLI. This skill is guidance for Hermes, not a second workflow runtime. For local dogfood, the canonical shared skill and CLI source is `~/cn/chestnut-flake/vamos`; make skill or CLI changes there, not in a dotfiles checkout.

1. Run `vamos hermes setup --gateway-url <url>` once as an administrator; gateway configuration and credentials remain host-local.
1. Start a worker with `vamos hermes pi start --plan <absolute-plan-dir> --thread-id <originating-hermes-thread> [--previous-session <id>] "<bounded task>"` inside a Hermes background task **with a PTY**. To adopt a manual continuation, add `--thread-id <originating-hermes-thread>` only when the new Pi session is registered before launch; otherwise report it as a local-result fallback.
1. The worker is intentionally interactive: do **not** add `pi -p` or treat its initial prompt wait as a stalled job. Steer it through Hermes process-write/submit with scoped follow-up instructions. Its initial bounded task must require a durable artifact and a normal final response, not model-issued `pi done`.
1. Hermes may steer that live Pi process with its native process-write API. Do not kill an interactive worker merely because it is awaiting or processing a steerable turn; stop it only when superseding it or at the human's direction.
1. Treat process exit as liveness only. Read `vamos hermes pi result --plan <plan> --session <id> --format json` for semantic completion.

### macOS persistent Vamos/Hermes host configuration

For a browser-visible Vamos host on macOS, inspect and operate the service with terminal `launchctl` commands only; do not use desktop/computer-control tooling for launchd state. Check `launchctl print gui/$(id -u)/dev.vamos`, the listening port, and the active process command before changing anything.

Keep the integration attached to the persistent host service, not a temporary verifier port:

1. Verify the clean runtime baseline and the clean host-wrapper checkout are both current and clean. Rebuild from the host wrapper so it produces the current runtime binary and wrapper; rebuild alone does not replace an already-running process.
1. Configure the Hermes Vamos platform adapter on loopback only (normally its `/health` endpoint is on `127.0.0.1:8765`) with an ingress credential and the Vamos base URL. Install/enable the adapter explicitly, start/restart the Hermes gateway, and prove adapter health before saving host callback settings.
1. Run `vamos hermes setup` with the adapter health URL, the persistent Vamos URL (for example `http://127.0.0.1:4200`), and the callback credential. This writes the host-local `hermes.yaml` with restrictive permissions.
1. Persist the corresponding `VAMOS_HERMES_GATEWAY_URL`, gateway ingress credential, and `VAMOS_HERMES_CALLBACK_TOKEN` in the actual `dev.vamos` LaunchAgent configuration, then restart that agent. Do not rely on a temporary shell export or `launchctl setenv`: those do not establish a reboot-safe service configuration.
1. After restart, verify the agent is running from the expected host wrapper/runtime, port 4200 returns its expected health/auth response, the Hermes adapter `/health` responds, and the host-local config points at the persistent Vamos URL. Never print credentials while doing these checks.

### Stop-bound delivery verification

Normal managed completion is extension-owned at the actual persisted final-assistant boundary. It writes immutable opaque settlement evidence before delivery; a model-issued `pi done` is only manual recovery and never authorizes manager advancement.

Before claiming end-to-end Hermes↔child delivery, verify all of these without printing credentials:

1. The host config exists and its callback URL/token are configured.
1. Vamos has the same `VAMOS_HERMES_CALLBACK_TOKEN` value.
1. The Pi session is registered to exactly one Hermes thread for the plan.
1. The Vamos Hermes gateway is configured and can deliver the settlement evidence.

Missing host configuration, or a `404` for an unowned/unknown Pi session, is an intentional local-result/manual-continuation fallback—not proof that the child failed. Do not read or expose existing credentials merely to test this path; ask the user to provide or explicitly authorize secure local provisioning.

### CLI dogfood

The stable launcher should point at `~/cn/chestnut-flake/vamos`. Runtime and skill edits there become active after the launcher refreshes its managed runtime cache or Pi reloads resources; rebuild `~/.local/bin/vamos` only when `cmd/vamos-launcher` itself changes. Use `VAMOS_PACKAGE_ROOT=/absolute/path/to/feature-checkout` for isolated pre-merge CLI verification without changing persisted launcher state.

1. Inspect the durable artifact and concise summary, then let Hermes or the lead engineer choose whether to launch another worker.

Do not use tmux panes, manager state, graph transitions, process polling/recovery, wake protocols, or copied prior worker documents. Stop for explicit human gates, lost-work safety, missing artifacts, blocked/error outcomes, or decisions Hermes cannot safely infer. For an implementation handoff, launch a fresh worker with `--previous-session` and a task to resume from the handoff.
