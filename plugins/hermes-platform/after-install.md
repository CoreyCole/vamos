# Vamos Hermes platform adapter

Install the standalone source explicitly; do not copy it into the Hermes source tree:

```bash
hermes plugins install --enable file:///absolute/path/to/vamos#plugins/hermes-platform/vamos_platform
hermes plugins list --enabled
```

The exact-session ingress routes require Hermes core commit `db66ff265697d87c64ddaaf96569b733c79c2bba` or a release containing it. The running core must advertise protocol `1` and capability `exact-session-next-turn-v1`. An older or incompatible core leaves only exact-session manager notification unsupported; the plugin returns `surface_unsupported` and does not enqueue work.

Configure the Hermes gateway's `platforms.vamos` entry with an ingress credential and the Vamos server URL. The adapter binds to `127.0.0.1` by default. For a remote Vamos server, use an explicit TLS-terminating reverse proxy and configure a non-loopback bind only after that route and network policy are in place.

```yaml
platforms:
  vamos:
    enabled: true
    extra:
      host: 127.0.0.1
      port: 8765
      token: "dedicated ingress credential"
      callback_token: "Vamos callback-only credential"
      vamos_url: "https://vamos.example"
```

Restart the gateway after configuration. The dedicated ingress token authenticates both `POST /vamos/session-ingress/v1/capabilities` and `POST /vamos/session-ingress/v1/enqueue`; the callback token is callback-only. Capability success advertises only protocol `1` and `exact-session-next-turn-v1`. Enqueue success (`202`) means the turn was admitted to the exact current Hermes session generation; it does not mean the manager processed or answered it.

`vamos hermes setup` checks `/health` and then makes a separate authenticated request to the capability route. A healthy adapter without exact protocol-v1 capability fails closed. Capability is not admission, and enqueue admission is not proof that the manager model ran or that a response reached the original child.

For an uncertain or rejected notification, an operator may retry only one explicitly selected immutable record:

```bash
vamos hermes pi notify --plan <absolute-plan-dir> --pi-session <id> --message-id <id>
```

The command uses the same capability and notifier path as managed launch, preserves the exact settlement response as opaque content, and reports at-least-once uncertainty without claiming manager completion. It does not discover or scan settlements.

The existing `/vamos/manager-wake` endpoint remains an isolated compatibility route for old launchers. It retains its previous synthetic `manager_thread_id` request and behavior and never translates to the v1 routes or exact-session runner.

Delivery is at-least-once, so duplicates are possible. The adapter wraps immutable settlement text as non-authoritative manager input. It does not parse child YAML, register a callback, create a schedule, select or launch a successor, or invoke `pi done`. Promotion requires the separate source-tree TUI round-trip proof; health, capability, publication, and admission are insufficient.
