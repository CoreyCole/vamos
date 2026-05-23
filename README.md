# Vamos Agents

Reusable white-label server for browsing a private `thoughts/` directory, running Agent Chat/Pi sessions, and coordinating agentic workflow runtimes such as QRSPI.

## Build

```bash
just build --no-restart
```

The build tool expects generated/runtime outputs to stay local and gitignored. Datastar Pro assets are licensed; provide them either at `../datastar-pro/datastar-pro-v1.js` or through:

```bash
export VAMOS_DATASTAR_PRO_ASSET=/absolute/path/to/datastar-pro-v1.js
```

## Configure

Start from `config.example.yml` and pass it with:

```bash
export VAMOS_CONFIG=/absolute/path/to/config.yml
```

Host applications own OAuth credentials, thoughts paths, deploy/service names, linked project checkouts, and runtime state locations.
