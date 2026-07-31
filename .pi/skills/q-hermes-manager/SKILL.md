---
name: q-hermes-manager
description: Guide Hermes-managed isolated Pi workers for QRSPI artifacts.
---

# Hermes QRSPI manager

Hermes owns conversation state, background task handles, process-write steering, and every decision to stop, continue, or launch a successor. Vamos provides durable plan artifacts, the managed launcher, deterministic settlement evidence, and a direct manager-wake adapter. This skill is guidance for Hermes, not a second workflow runtime. For local dogfood, use the canonical shared skill and CLI source at `~/cn/chestnut-flake/vamos`.

## Managed worker operation

1. An administrator runs `vamos hermes setup --gateway-url <url> --vamos-url <url> --ingress-token <value> --callback-token <value>` once. Configuration and credentials remain host-local and must not be printed.
1. Start a worker with `vamos hermes pi start --plan <absolute-plan-dir> --thread-id <originating-hermes-thread> [--previous-session <id>] "<bounded task>"` inside a Hermes background task with a PTY.
1. The worker is interactive. Do not add `pi -p` or treat its prompt wait as a stalled job. Steer the live process through Hermes process-write/submit. Require a durable artifact and a normal final response, not model-issued `pi done`.
1. Treat process exit as liveness only. Each persisted final-assistant `agent_settled` boundary produces an immutable deterministic YAML record and attempts a direct manager wake. Empty final text is valid, and a no-YAML response is still delivered.
1. Inspect the child artifact and exact final response, then let Hermes or the lead choose the next action. Child YAML is opaque evidence and never selects, starts, or steers a successor.

To adopt a manual continuation, pass `--thread-id` only when the new Pi session is registered before launch; otherwise report a local-result fallback. Do not create plugin callback registrations or use copied prior-worker documents.

## Direct manager-wake boundary

Managed launch transiently supplies exactly:

- `VAMOS_MANAGER_WAKE_MANAGER_THREAD_ID`
- `VAMOS_MANAGER_WAKE_PI_SESSION_ID`
- `VAMOS_MANAGER_WAKE_GATEWAY_URL`
- `VAMOS_MANAGER_WAKE_INGRESS_TOKEN`

The extension publishes `.vamos/sessions/pi/<session>/settlements/<message_id>.yaml` before network delivery. Every bounded live retry reloads that record's `raw_response`; it does not reconstruct the message from later hook state. Delivery through `/vamos/manager-wake` is best-effort at-least-once. Duplicate manager messages are possible, a 2xx is not a durable manager receipt, and there is no outbox, automatic post-exit redrive, or exactly-once guarantee.

The manager-wake ingress credential is distinct from the Vamos callback credential. The callback credential is never injected into Pi. Neither token nor the gateway URL may appear in prompts, custom entries, evidence, attempt records, diagnostics, or logs.

A child does not issue `pi done`, register a callback, select or launch a successor, or claim manager receipt. Hermes may steer the original live child through its existing process-write API. Stop it only when superseding it or at the human's direction.

## Manual recovery

Choose one deterministic YAML settlement file explicitly and resubmit its preserved identity and `raw_response` through the authenticated plugin endpoint. Do not search or scan for settlements, automatically replay historical JSON records, migrate them, or interpret fenced YAML as routing policy. Delivery uncertainty remains best-effort at-least-once uncertainty.

After deploying the discovery retirement, an operator may run `vamos-runtime hermes cleanup-opaque-settlement-schedules`. It completely lists Temporal schedules, deletes only literal `opaque-settlement-discovery:` IDs, and freshly verifies zero remain. It is safe to rerun after failure and is never a startup hook. Do not run it during ordinary worker operation or automated verification against a live Temporal service.

## macOS persistent host configuration

Inspect and operate the browser-visible host with terminal `launchctl` commands only. Verify the clean runtime baseline and host-wrapper checkout, rebuild from the wrapper, and restart the actual persistent service; rebuild alone does not replace a running process.

Configure the Hermes Vamos adapter on loopback with separate ingress and callback credentials. Persist matching service configuration in the real `dev.vamos` LaunchAgent, restart it, and verify the expected process, host health response, adapter health, and host-local config without printing secret values. Do not rely on temporary shell exports or `launchctl setenv` for reboot-safe configuration.

Use `VAMOS_PACKAGE_ROOT=/absolute/path/to/feature-checkout` only for isolated pre-merge CLI verification. Rebuild `~/.local/bin/vamos` only when `cmd/vamos-launcher` changes.

Stop for explicit human gates, lost-work safety, missing artifacts, blocked/error outcomes, delivery uncertainty, or decisions Hermes cannot safely infer. For an implementation handoff, the manager may launch a fresh worker with `--previous-session` after reviewing the handoff.
