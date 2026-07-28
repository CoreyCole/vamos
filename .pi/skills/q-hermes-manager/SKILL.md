---
name: q-hermes-manager
description: Guide Hermes-managed isolated Pi workers for QRSPI artifacts.
---

# Hermes QRSPI manager

Hermes owns conversation state, background task handles, process-write steering, and the decision to launch a successor. Vamos provides durable plan artifacts and the thin worker CLI. This skill is guidance for Hermes, not a second workflow runtime.

1. Run `vamos hermes setup --gateway-url <url>` once as an administrator; gateway configuration and credentials remain host-local.
1. Start a worker with `vamos hermes pi start --plan <absolute-plan-dir> [--previous-session <id>] "<bounded task>"` inside a Hermes background task. To adopt a managerless/manual continuation on the same host, accept the easy-to-copy `vamos hermes pi continue <short-id>` command printed by `pi done` and run it in the Hermes background task; the host-local resume index resolves its plan and prior session.
1. Hermes may steer that live Pi process with its native process-write API.
1. Treat process exit as liveness only. Read `vamos hermes pi result --plan <plan> --session <id> --format json` for semantic completion.
1. Inspect the durable artifact and concise summary, then let Hermes or the lead engineer choose whether to launch another worker.

Do not use tmux panes, manager state, graph transitions, process polling/recovery, wake protocols, or copied prior worker documents. Stop for explicit human gates, lost-work safety, missing artifacts, blocked/error outcomes, or decisions Hermes cannot safely infer. For an implementation handoff, launch a fresh worker with `--previous-session` and a task to resume from the handoff.
