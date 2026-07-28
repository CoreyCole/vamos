# Vamos Hermes platform adapter

Install the standalone source explicitly; do not copy it into the Hermes source tree:

```bash
hermes plugins install --enable file:///absolute/path/to/vamos#plugins/hermes-platform/vamos_platform
hermes plugins list --enabled
```

Configure the Hermes gateway's `platforms.vamos` entry with a shared credential and the Vamos server URL. The adapter binds to `127.0.0.1` by default. For a remote Vamos server, use an explicit TLS-terminating reverse proxy and configure a non-loopback bind only after that route and network policy are in place.

```yaml
platforms:
  vamos:
    enabled: true
    extra:
      host: 127.0.0.1
      port: 8765
      token: "shared ingress credential"
      callback_token: "Vamos callback credential"
      vamos_url: "https://vamos.example"
```

Restart the gateway after configuration. Credentials are host configuration and must not appear in plan artifacts or Pi environments.
