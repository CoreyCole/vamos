# HTML Applets

Vamos can render trusted `.html` Thoughts documents as sandboxed applets inside the Thoughts workbench. The applet owns its HTML, CSS, and JavaScript; Vamos provides stable shared assets for applets that want to match the app shell.

## Default prelude

Use this prelude for authored applets that should look native and follow the parent theme:

```html
<link rel="stylesheet" href="/css/out.css">
<script type="module" src="/js/vamos-html-applet.js"></script>
<style>
  /* applet-local overrides after Vamos CSS */
</style>
```

Use `/css/out.css` in durable authored HTML. The parent app may load build-hashed CSS internally, but Thoughts artifacts should use stable URLs that survive rebuilds.

Import `/js/vamos-html-applet.js` when the applet should follow the parent light/dark toggle. The helper reads the initial theme from the iframe URL and listens for parent theme changes. Do not hard-code the root element into dark mode for durable applets.

Local overrides belong after `/css/out.css`, either inline or via same-directory assets. Later rules should win over shared Vamos defaults.

## Optional child-local Datastar

Applet-local Datastar behavior is explicit and separate from the parent workbench:

```html
<script type="module" src="/js/datastar-pro-v1.js"></script>
```

Only import Datastar when the child applet needs it. Parent Datastar state and globals do not cross the iframe boundary.

## Sandbox boundary

Applet scripts run inside the iframe. Parent DOM access is intentionally blocked by the sandbox, and applets should communicate only through documented, narrow browser messages such as the theme helper's `dark` / `light` update.

Vamos serves the authored HTML route as raw bytes with applet response headers. It does not inject CSS, scripts, or theme classes into applet documents.
