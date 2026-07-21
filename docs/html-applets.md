# HTML Applets

Vamos renders trusted `.html` Thoughts documents as sandboxed applets inside the Thoughts workbench. The applet owns its HTML, CSS, and JavaScript.

Static HTML applets are single files with client-side behavior only. For server-backed Datastar, Streamlit, Go, or Python apps, use [HTTP Applets](http-applets.md). HTTP applets run a managed local server behind the same Workbench shell and support SSE reads and short write requests.

## Client-only Datastar

Static HTML applets may use Datastar for local UI signals. Load the pinned public bundle explicitly:

```html
<script
  type="module"
  src="https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js"
></script>
```

The same online file should work unchanged through `file://` and in the Vamos iframe. Do not depend on `/js/datastar-pro-v1.js`, parent Workbench signals, or Datastar fetch/SSE actions. The parent and child documents have independent Datastar runtimes and state. Use an HTTP applet for server-backed behavior.

Conditional content should fail closed before Datastar hydrates:

```html
<body data-signals="{_answer: ''}">
  <button data-on:click="$_answer = 'correct'">Choose answer</button>
  <p style="display: none" data-show="$_answer === 'correct'">Correct…</p>
</body>
```

Put `style="display: none"` on each conditional element. Do not use a permanent `[data-show] { display: none }` stylesheet rule: Datastar toggles inline display state, so that selector can keep the element hidden after its condition becomes true.

## Optional embedded styling and theme

Applets rendered by Vamos may use stable shared assets to match the app shell:

```html
<link rel="stylesheet" href="/css/out.css">
<script type="module" src="/js/vamos-html-applet.js"></script>
<style>
  /* applet-local overrides after Vamos CSS */
</style>
```

These root-relative assets are embedded-only enhancements and are not available when opening the file directly through `file://`. Do not require them for standalone behavior.

Use `/css/out.css` when an embedded applet should share Vamos styles. The parent may use build-hashed CSS internally, but authored Thoughts files should use the stable URL. Put local overrides after the shared stylesheet so later rules win.

Use `/js/vamos-html-applet.js` when an embedded applet should follow the parent light/dark toggle. The helper reads the initial theme from the iframe URL and listens for parent theme changes. Do not hard-code the root element into dark mode for durable applets.

## Sandbox boundary

Applet scripts run in an opaque-origin iframe. Parent DOM access is intentionally blocked; the sandbox does not grant `allow-same-origin`. Applets should communicate only through documented, narrow browser messages such as the theme helper's `dark` / `light` update.

Vamos serves authored HTML as raw bytes with applet response headers. It does not inject CSS, scripts, runtime state, or theme classes into applet documents.
