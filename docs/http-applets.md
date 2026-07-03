# HTTP Applets

Vamos HTTP applets embed a local HTTP server inside the Thoughts Workbench. They are for Datastar, Streamlit, Go/templ, Python, or other trusted local apps that need a running process instead of a static `.html` document.

For static authored HTML, use [HTML Applets](html-applets.md). HTML applets serve trusted bytes directly. HTTP applets start a managed process, put its UI in the Workbench center iframe, and keep the normal Files/Workspaces sidebar plus Chat/Comments rail.

## Workbench model

Opening an HTTP applet should feel like opening any Thoughts document:

- Left: Files and Workspaces sidebar.
- Center: iframe backed by the applet HTTP process.
- Right: Chat and Comments for the applet manifest identity.

The applet manifest path is the durable identity for comments and chat. Runtime details such as process IDs, ports, and absolute checkout paths are not durable artifact identity.

## Manifest frontmatter

HTTP applets are declared with markdown frontmatter:

```yaml
---
vamos_artifact: applet
applet:
  id: wordle
  kind: datastar
  title: Wordle
  source_dir: examples/wordle
  start_command: ["just", "build"]
  health_path: /healthz
  root_aliases:
    - pattern: /events
      methods: [GET]
    - pattern: /guesses
      methods: [POST]
---
```

Fields:

- `id`: lowercase applet identifier used for manager/runtime routing.
- `kind`: `datastar`, `streamlit`, or `http`.
- `title`: Workbench title.
- `source_dir`: app source directory resolved under the configured files root.
- `start_command`: explicit command that starts the HTTP app; Vamos supplies a local `PORT`.
- `health_path`: app health check path. Defaults to `/healthz`, except Streamlit defaults to `/_stcore/health`.
- `root_aliases`: explicit root-relative app paths that Vamos may forward to the app process.

## Standalone app rule

App code should stay standalone. It may keep root-relative routes such as `/events`, `/guesses`, or `/_stcore/stream`. The manifest declares which root paths belong to the applet so Vamos can forward those requests while the app is embedded.

Vamos does not install a global root catch-all. Aliases are explicit and conflict checked.

## Datastar example

Datastar apps often use one SSE read route and short POST write routes:

```yaml
applet:
  id: wordle
  kind: datastar
  title: Wordle
  source_dir: examples/wordle
  start_command: ["just", "build"]
  root_aliases:
    - pattern: /events
      methods: [GET]
    - pattern: /guesses
      methods: [POST]
```

The app can keep Datastar attributes like `data-init="@get('/events')"`. Vamos forwards `/events` to the running applet only because the manifest declared it.

## Streamlit example

Streamlit apps use `/_stcore/*` for health and streaming/WebSocket endpoints. Vamos defaults Streamlit applets to `health_path: /_stcore/health` and root aliases for `/_stcore/*` and `/vendor/*`.

```yaml
applet:
  id: sales_dashboard
  kind: streamlit
  title: Sales Dashboard
  source_dir: dashboards/sales
  start_command:
    - streamlit
    - run
    - app.py
    - --server.address
    - 127.0.0.1
```

Do not rely on default `/static/*` forwarding. Vamos owns manager `/static` assets. If an app truly needs root `/static/*`, declare it explicitly only when the alias registry can prove it will not shadow manager assets. Prefer scoped applet asset paths.

## Proxy behavior

The applet proxy is designed for Datastar SSE and Streamlit streaming endpoints:

- scoped iframe paths strip the applet prefix before forwarding;
- declared root aliases forward without path stripping;
- query strings and raw paths are preserved;
- `X-Forwarded-Host`, `X-Forwarded-Proto`, `X-Forwarded-Prefix`, and `X-Vamos-Applet-Proxy` are set;
- SSE uses immediate flush;
- WebSocket upgrade headers are preserved;
- root-path cookies can be rewritten to the scoped applet path.

## Lifecycle

HTTP applets are demand-started. When a stopped applet page is opened, the Workbench renders a starting panel and subscribes to an applet status SSE stream. Once the process is healthy, the page refreshes or replaces the frame so the app loads. Idle applets can be stopped by the sweeper, but active SSE/WebSocket connections keep the process alive.

## Security limits

HTTP applets are trusted local apps, not an arbitrary internet sandbox. Vamos still applies safety boundaries:

- no global catch-all proxy;
- route aliases must be explicit and conflict checked;
- applet source paths are resolved under configured roots;
- cookies should be scoped to the applet proxy when possible;
- applet identity for comments/chat is a durable manifest artifact path, not a machine-local runtime value.
