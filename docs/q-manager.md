# q-manager manifest

## Manager mission

Hermes supervises isolated QRSPI Pi workers while keeping durable plan artifacts authoritative. Hermes owns conversation state, process-write steering, and every decision to stop, continue, or launch another worker. The direct manager-wake path only returns a child's exact final response to its owning manager; it is not a second workflow runtime.

## Authority boundaries

The project-local `.pi/extensions/q-manager-child` validates managed-child identity, publishes immutable deterministic YAML evidence, and posts that published record directly to the Hermes platform plugin. The plugin forwards the exact `raw_response` to the configured manager thread through `handle_message`.

Neither the extension nor plugin:

- parses child YAML as lifecycle instructions;
- selects, starts, or steers a successor;
- invokes model-issued `pi done`;
- creates a callback registration, schedule, or worker;
- creates a durable receipt or claims exactly-once delivery.

A manager or human reads the artifact and response and chooses the next action. Vamos remains authoritative for QRSPI artifacts and workflow policy outside this narrow delivery adapter.

## Managed launcher

Start a bounded interactive worker with:

```bash
vamos hermes pi start \
  --plan <absolute-plan-dir> \
  --thread-id <originating-hermes-thread> \
  [--previous-session <id>] \
  "<bounded task>"
```

Run it inside a Hermes background task with a PTY. Do not add `pi -p`. Use Hermes's existing process-write capability to steer the still-live worker. Require a durable artifact and a normal final response, not `pi done`.

Managed launch transiently supplies exactly:

- `VAMOS_MANAGER_WAKE_MANAGER_THREAD_ID`
- `VAMOS_MANAGER_WAKE_PI_SESSION_ID`
- `VAMOS_MANAGER_WAKE_GATEWAY_URL`
- `VAMOS_MANAGER_WAKE_INGRESS_TOKEN`

Unmanaged launch supplies none of them. The ingress credential is distinct from the Vamos callback credential; the callback credential is never injected into Pi. Neither token nor the gateway URL belongs in prompts, custom Pi entries, evidence, attempt records, diagnostics, or logs.

## Direct manager-wake contract

At each persisted final-assistant `agent_settled` boundary, the extension writes one immutable deterministic YAML record before attempting delivery:

```text
.vamos/sessions/pi/<session>/settlements/<message_id>.yaml
```

The `message_id` and filename are stable for the assistant entry. Empty final text is valid. Every bounded live retry reloads `raw_response` from that same published record; it does not reconstruct the message from later hook state.

`gateway_url` and `VAMOS_MANAGER_WAKE_GATEWAY_URL` contain the normalized Hermes adapter base URL, not an endpoint URL. Setup and managed-start preflight verify `GET <base>/health`; the extension posts `version`, `manager_thread_id`, `pi_session_id`, `message_id`, and the preserved message to `<base>/vamos/manager-wake`. The adapter awaits the owning manager's `handle_message` call before returning.

Delivery is best-effort at-least-once. Duplicate manager messages are possible. A 2xx means only that the adapter returned after `handle_message`; it is not a durable manager receipt. There is no outbox, automatic post-exit redrive, or exactly-once guarantee.

## Recovery

Manual recovery starts from one explicitly selected deterministic YAML settlement file. Read its preserved identity and `raw_response`, then resubmit those fields through the authenticated plugin endpoint. Do not scan for settlements, automatically replay or migrate historical JSON records, or infer an action from fenced YAML.

If delivery is uncertain, preserve the record and report at-least-once uncertainty. Hermes may steer the original live worker or, after reviewing its artifact, explicitly start another worker with `--previous-session`. The child itself does not choose that action.

## Retired schedule cleanup runbook

After deploying code with opaque-settlement discovery registration removed, an operator runs once:

```bash
vamos-runtime hermes cleanup-opaque-settlement-schedules
```

The command uses `TEMPORAL_ADDRESS` (default `localhost:7233`) and optional `TEMPORAL_NAMESPACE`. It fully paginates all schedules, deletes only IDs with the literal byte prefix `opaque-settlement-discovery:`, then performs a fresh fully paginated list.

Success reports zero remaining matches. List, delete, re-list, or remaining-match failures exit nonzero; repair Temporal access or the reported schedule and safely rerun. Record only the empty verification result—never credentials, URLs, or settlement contents.

This cleanup is an explicit idempotent operator action, not a startup hook. It never creates, triggers, replaces, or registers a schedule or worker. Historical JSON settlements remain historical and are not scanned, replayed, or migrated automatically.

## Verification habits

Use fake Temporal schedule clients for cleanup tests and fake HTTP endpoints for manager-wake tests. Automated verification must not clean a real namespace, contact a live Hermes gateway, perform live dogfood, invoke `pi done`, register callbacks, or launch successors. Finish runtime changes through normal QRSPI review and `/vamos-merge` after the bounded implementation slice ends.
