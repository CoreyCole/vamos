# Vamos Hermes platform adapter

Install the standalone source explicitly; do not copy it into the Hermes source tree:

```bash
hermes plugins install --enable file:///absolute/path/to/vamos#plugins/hermes-platform/vamos_platform
hermes plugins list --enabled
```

Configure the Hermes gateway's `platforms.vamos` entry with an ingress credential and the Vamos server URL. The adapter binds to `127.0.0.1` by default. For a remote Vamos server, use an explicit TLS-terminating reverse proxy and configure a non-loopback bind only after that route and network policy are in place.

```yaml
platforms:
  vamos:
    enabled: true
    extra:
      host: 127.0.0.1
      port: 8765
      token: "manager-wake ingress credential"
      callback_token: "Vamos callback-only credential"
      vamos_url: "https://vamos.example"
```

Restart the gateway after configuration. `extra.token` is the ingress credential for `/vamos/manager-wake`; `extra.callback_token` is callback-only. Managed children receive exactly `VAMOS_MANAGER_WAKE_MANAGER_THREAD_ID`, `VAMOS_MANAGER_WAKE_PI_SESSION_ID`, `VAMOS_MANAGER_WAKE_GATEWAY_URL`, and `VAMOS_MANAGER_WAKE_INGRESS_TOKEN` as transient process environment. Unmanaged children receive none of them. Neither credential nor the manager-wake URL may enter prompts, custom Pi entries, evidence, attempt records, diagnostics, or logs; the callback token is never injected into Pi.

The adapter forwards the exact message from immutable deterministic YAML evidence to the owning manager. Delivery is best-effort at-least-once: duplicates are possible, and a 2xx is neither a durable receipt nor an exactly-once guarantee. The adapter does not parse child YAML, register a callback, select or launch a successor, or invoke `pi done`.
