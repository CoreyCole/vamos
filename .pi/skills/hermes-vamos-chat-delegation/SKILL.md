---
name: hermes-vamos-chat-delegation
description: Hermes-only automation for Vamos web Agent Chat via `vamos chat start` and `steer`. Use only when testing/operating the Vamos web UI or delegating from Hermes through its authenticated manager API; it requires a manager-issued machine credential. For Hermes-managed local Pi work, use `q-hermes-manager` instead.
---

# Hermes Vamos Chat Delegation

Delegate work from **Hermes** to the authenticated Vamos **web Agent Chat** API without bloating Hermes context.

> **Not for local Pi delegation.** `vamos chat start` is a web-manager API client and requires a manager-issued machine credential. For local plan work, use `.pi/skills/q-hermes-manager/SKILL.md`; Hermes launches isolated Pi workers without a local pane manager.

## Step 1: Start work

Run only from a Hermes background task when the target is Vamos web Agent Chat:

```bash
vamos chat start --project <project_id> "<prompt>"
```

- Choose `project_id` from explicit user/project context; do not guess from local cwd.
- Make the prompt self-contained enough for the selected Vamos workflow to continue without Hermes transcript context.
- Parse the first NDJSON line. It must have `type=started`, `ref.thread_id`, `ref.run_id`, `ref.chat_session_id`, and `ref.web_url`.
- Immediately share `ref.web_url` and `ref.thread_id` with the user so they can watch or steer in Vamos web Agent Chat.
- Keep Hermes context lean: retain prompt, refs, and concise terminal state. Do not dump transcripts by default.

Example user update:

```text
Started in Vamos: <web_url>
Thread: <thread_id>
```

## Step 2: Observe result

- Keep the CLI attached inside the Hermes background task; it streams NDJSON from Vamos chat-session SSE.
- Summarize terminal `result`, `failed`, or `needs_steer` lines for the user.
- Treat compact `event` lines as progress signals; do not relay every event unless the user asks.
- Do not steer a successful active run by default; Hermes decides any follow-up after it observes durable completion.

Terminal summary examples:

```text
Vamos complete: plan written; next auto-started review.
Vamos needs steer: incomplete worker outcome; use thread <thread_id>.
Vamos failed: <short reason>; web: <web_url>.
```

## Step 3: Steer follow-up

Use steer after a run stopped, failed, has no usable durable completion, or the user wants to add direction:

```bash
vamos chat steer --thread <thread_id> "<guidance>"
```

- Send only the existing `thread_id` and guidance prompt; do not invent a run ID parameter.
- If response says `influences_latest=false`, show and use `latest_thread_id` and `latest_web_url` for current work.
- If response says `steer_rejected` with `reason=run_in_progress`, tell the user the current run is active and share current refs.
- Treat steer as follow-up/recovery, not live mid-tool-call interruption.

## Step 4: Preserve boundaries

- Hermes may read docs, code, artifacts, and may edit planning artifacts, docs, and skills when asked.
- Hermes must not edit Vamos runtime code, tests, migrations, or generated files directly; delegate durable implementation to isolated Pi workers.
- Do not hardcode private domains, emails, credentials, tenant names, or host policy.
- Preserve tenant/project boundaries. `start --project` selects a configured server checkout; `steer --thread` follows an authorized existing thread only.
- If refs or authorization look stale/cross-project, stop and ask the user rather than guessing.

## Exit criteria

- User has received the initial web URL and thread ID.
- Hermes retained only prompt, refs, and concise terminal state.
- Follow-up work was left to Hermes and the lead engineer's judgment.
- Recovery/follow-up guidance used `vamos chat steer --thread` with authorized thread refs.
