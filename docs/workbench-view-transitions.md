# Workbench V2 View Transitions

Authoritative best practices for **cross-document sibling artifact navigation** in Workbench V2. This captures what we learned on box Chrome and Brave Android (proven at `147e21d+`) — not aspirational theory.

## Goal

Plain **GET** sibling artifact links + **CSS View Transitions**. Chrome (tabs, threads, chat, comments, path-header, browser) stays painted; only document content changes. Minimal JS.

## Do

1. Opt in to cross-document VT in head: `<meta name="view-transition" content="same-origin">` (`server/layouts/root.templ`) — easy to forget when copying the pattern.
2. Opt in in CSS: `@view-transition { navigation: auto }` (`static/css/index.css`) — pairs with the meta; also easy to miss when copying.
3. Give **stable unique `view-transition-name`s** on both old and new pages for chrome that should persist: mobile tabs, threads, chat, comments, path-header, browser. Same name = shared element across the GET.
4. Parent `#workbench-v2-artifact` (and pane wrappers / root) stay `view-transition-name: none` so the whole pane does not crossfade as one unit while children are named.
5. Only `#thread-artifact-document` is the live/changing named region.
6. Freeze chrome with **explicit per-name** `::view-transition-{group,old,new}(name) { animation: none }`. Don’t rely on `view-transition-class` alone on any engine; keep explicit per-name freeze.
7. Ghost kill when selection/path tree changes: `::view-transition-old(thread-artifact-browser|path-header) { display: none }` (avoids double-tree flicker).
8. Root: `animation: none` on group/old/new; **`display: none` only on `::view-transition-old(root)`**. Never blank both old **and** new root — that wipes unmatched chrome and causes under-tabs black on mobile Chromium.
9. Prefer real `<a href>` sibling GETs. No fetch/morph click intercept for file nav.
10. Minimal JS only (`static/js/workbench-history.js`): Enter/Up `pushState({ workbenchArtifactPatch })` + popstate reload gate so Back never blanks; Files cookie on templ button + SSR; history.js stays tiny.
11. SSR selected mobile tab classes + `ActiveRegionID=workbench-v2-artifact` on `?artifact=` deep-links (hardening against Datastar bind flash).
12. Regression gate: Playwright Story `workbench-v2-mobile-sibling-doc-keeps-chrome-under-tabs` (`TestWorkbenchV2MobileSiblingDocKeepsChromeUnderTabs`) — fails if path/browser go height≈0 / invisible while tabs remain visible during sibling nav. Mobile Chrome viewport (~390).

## Don’t

- Blank all chrome to `view-transition-name: none` and only name the document (browser has nothing shared → black flash).
- Rely on `view-transition-class` alone for freeze.
- `display: none` on both `::view-transition-old(root)` and `::view-transition-new(root)`.
- Reintroduce fetch/morph intercept for sibling docs (rejected as overcomplicated).
- Opacity-dim whole panes as a “soft” transition.
- Blame the browser before auditing names: unmatched or missing shared names are our bug.

## Name map (current)

| selector | name | class |
| --- | --- | --- |
| `#workbench-mobile-tabs` | `workbench-mobile-tabs` | `workbench-chrome` |
| `#workbench-v2-threads` | `workbench-v2-threads` | `workbench-chrome` |
| `#workbench-v2-chat` | `workbench-v2-chat` | `workbench-chrome` |
| `#workbench-v2-comments` | `workbench-v2-comments` | `workbench-chrome` |
| `#thread-artifact-path-header` | `thread-artifact-path-header` | `workbench-chrome` |
| `#thread-artifact-browser` | `thread-artifact-browser` | `workbench-chrome` |
| `#thread-artifact-document` | `thread-artifact-document` | — |
| `#workbench-v2-artifact` / pane / root | `none` | — |

## Key files

- `static/css/index.css` — `@view-transition { navigation: auto }`, names, per-name freeze, root old-only hide, ghost kill
- `server/layouts/root.templ` — `<meta name="view-transition" content="same-origin">` (cross-document VT opt-in)
- `static/js/workbench-history.js` — pushState patch flag + popstate reload gate (no morph)
- `server/layouts/workbench/mobile.templ` — SSR selected tab + ActiveRegionID deep-link hardening
- `server/services/markdown/thread_navigation.templ` — sibling `<a href>` GETs / Files cookie
- `pkg/e2e/workbenchv2tests/workbench_v2_doc_vt_e2e_test.go` — under-tabs chrome Story

## History (brief)

We tried morph intercept, then over-cleared names, then class-only freeze + root old+new `display: none` — each caused flashes (including under-tabs black on mobile Chromium). Current shape (`147e21d+`) is the proven one on box Chrome + Brave Android: stable shared names, per-name freeze, old-root-only hide, plain sibling GETs, tiny history gate.

## Debugging checklist

1. Confirm matching `view-transition-name`s exist on **both** documents for every chrome region that should persist.
2. Confirm parents stay `none` so they do not swallow child shared elements.
3. Confirm freeze is per-name (not class-only).
4. Confirm root hides **old only**.
5. Run `workbench-v2-mobile-sibling-doc-keeps-chrome-under-tabs` at mobile viewport before blaming the browser.
