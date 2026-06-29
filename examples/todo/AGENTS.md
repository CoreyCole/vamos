---
vamos_artifact: applet
applet:
  id: todo
  title: Todo
  files_root: files
  app_dir: .
  route: /examples/todo
  app_route: /examples/todo/app/
  start_command: [just, build]
  health_path: /healthz
  port: 8080
  backend_port: 18080
---

# Todo Applet

This directory is a Vamos Datastar todo applet.

Rules for agents:

- Run `just build` after source changes; it generates code/assets, tests, compiles, restarts the background dev server, healthchecks, and exits.
- Check `just status` and `.run/air.log` before claiming the app is working.
- Use `VAMOS_APP_FILES_ROOT` for all durable applet files. Default: `./files`.
- Do not write durable app data outside `VAMOS_APP_FILES_ROOT`.
- Do not edit generated `*_templ.go`, `internal/store/dbgen/*`, or `static/app.css` directly.
- Keep Datastar application state on the backend. Use one SSE stream for reads and short form POSTs for writes.
- `just build` starts a stable dev proxy on `0.0.0.0:${PORT:-8080}` and an Air-managed backend on `127.0.0.1:${BACKEND_PORT:-18080}`.
- Browser hot reload is owned by the stable dev proxy: the browser keeps one Datastar `/events` stream to the proxy, Air restarts the backend, then the proxy reloads the page after the backend is healthy.
