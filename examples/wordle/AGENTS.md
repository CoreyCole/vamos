---
vamos_artifact: applet
applet:
  id: wordle
  title: Daily Wordle
  kind: datastar
  files_root: files
  app_dir: .
  route: /examples/wordle
  app_route: /examples/wordle/app/
  start_command: [just, build]
  health_path: /healthz
  port: 8081
  backend_port: 18081
  root_aliases:
    - pattern: /events
      methods: [GET]
    - pattern: /guesses
      methods: [POST]
---

# Daily Wordle Applet

This directory is a Vamos Datastar Wordle applet.

Rules for agents:

- Run `just build` after source changes; it generates code/assets, tests, compiles, restarts the background dev server, healthchecks, and exits.
- Check `just status` and `.run/air.log` before claiming the app is working.
- Use `VAMOS_APP_FILES_ROOT` for durable applet files. Default: `./files`.
- Do not write durable app data outside `VAMOS_APP_FILES_ROOT`.
- Do not edit generated `*_templ.go`, `internal/store/dbgen/*`, or `static/app.css` directly.
- Keep Datastar application state on the backend. Use one SSE stream for reads and short form POSTs for writes.
- The daily word is deterministic from the user's local calendar date and the vendored `internal/words/words.txt` ordering.
- Username-only auth is not security; it only demonstrates cross-device resume by reusing the same username.
- `just build` starts a stable dev proxy on `0.0.0.0:${PORT:-8081}` and an Air-managed backend on `127.0.0.1:${BACKEND_PORT:-18081}`.
