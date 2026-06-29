# Datastar Starter Applet

A neutral Vamos example applet template using Go, templ, Datastar, Tailwind, sqlc, SQLite, pnpm, Air, and just.

## Run

```bash
just build
just status
```

Open <http://127.0.0.1:8080/> locally, or use the machine's Tailnet/LAN hostname on port 8080.

Logs live in `.run/proxy.log` and `.run/air.log`. Durable applet files live under `VAMOS_APP_FILES_ROOT`, defaulting to `./files`.

`just build` starts a stable dev proxy on `0.0.0.0:${PORT:-8080}` and an Air-managed backend on `127.0.0.1:${BACKEND_PORT:-18080}`. The browser keeps one Datastar `/events` stream to the proxy. When Air restarts the backend, the proxy waits for `/healthz`, sends a reload script over the still-open stream, and then bridges to the new backend.
