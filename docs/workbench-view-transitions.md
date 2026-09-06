# Workbench V2 View Transitions

Authoritative best practices for **cross-document sibling artifact navigation** in Workbench V2. This captures what we learned on box Chrome and Brave Android (proven at `147e21d+`) — not aspirational theory.

## Goal

Plain **GET** sibling artifact links + **CSS View Transitions**. Chrome (tabs, threads, chat, comments, path-header, browser) stays painted; only document content changes. Minimal JS.

## Do

1. Opt in to cross-document VT in head: `<meta name="view-transition" content="same-origin">` (`server/layouts/root.templ`) — easy to forget when copying the pattern.
2. Opt in in CSS: `@view-transition { navigation: auto }` (`static/css/index.css`) — pairs with the meta; also easy to miss when copying.
3. Give **stable unique `view-transition-name`s** on both old and new pages for chrome that should persist: **desktop app header** (`#app-header`), threads, chat, comments, path-header, browser. Same name = shared element across the GET. Mobile freezes under-tabs chrome; desktop freezes the top header plus the three columns. **Never name `md:hidden` mobile-only chrome for desktop VT** — `#workbench-mobile-tabs` is named only inside `@media (max-width: 767px)` and gets `view-transition-name: none` at `min-width: 768px` (hidden elements can still snapshot/flash during sibling GET).
4. Parent `#workbench-root` / `#workbench-regions` / `#workbench-v2-artifact` (and pane wrappers) stay `view-transition-name: none` so the whole pane does not crossfade as one unit while children are named. Do **not** name whole regions flex as one unit.
5. Only `#thread-artifact-document` is the live/changing named region.
6. Freeze chrome with **explicit per-name** `::view-transition-{group,old,new}(name) { animation: none }`. Don’t rely on `view-transition-class` alone on any engine; keep explicit per-name freeze.
7. Unchanged chrome (`workbench-v2-chat|threads|comments`, `app-header`, `workbench-mobile-tabs`): **`animation: none` on both `::view-transition-old` and `::view-transition-new`** — do **not** `display: none` either side (blanking old or new flashes/remounts the column). Path/browser selection *does* change — hide **old** there so the new selection shows. Document: hide old only (live swap). Do **not** set `view-transition-name: none` on chat.
8. Root: `animation: none` on group/old/new; **`display: none` only on `::view-transition-old(root)`**. Never blank both old **and** new root — that wipes unmatched chrome and causes under-tabs black on mobile Chromium.
9. Prefer real `<a href>` sibling GETs. No fetch/morph click intercept for file nav.
10. Minimal JS only (`static/js/workbench-history.js`): Enter/Up `pushState({ workbenchArtifactPatch })` + popstate reload gate so Back never blanks; after `pagereveal` / `viewTransition.finished` and `pageshow(!persisted)`, `pinChatToBottom` (double rAF + `fonts.ready`) pins whichever of `#agent-chat-messages` / `#agent-chat-scroll-region` actually overflows, `scrollIntoView` on `#chat-latest`, then focuses with `preventScroll`; Files cookie on templ button + SSR; history.js stays tiny.
11. SSR selected mobile tab classes + `ActiveRegionID=workbench-v2-artifact` on `?artifact=` deep-links (hardening against Datastar bind flash).
12. Regression gates: Playwright Story `workbench-v2-mobile-sibling-doc-keeps-chrome-under-tabs` (mobile under-tabs) and `workbench-v2-desktop-sibling-doc-keeps-chrome-under-header` (desktop under-header: header + threads/chat/path/browser stay painted; chat column opacity stays visible across frames; `#agent-chat-messages.scrollTop` near `scrollHeight`).

## Don’t

- Blank all chrome to `view-transition-name: none` and only name the document (browser has nothing shared → black flash).
- Name `md:hidden` / mobile-only chrome globally (e.g. `#workbench-mobile-tabs`) — it still participates in desktop sibling VT snapshots and flashes in-frame. Name only under `max-md`; set `view-transition-name: none` on `md+`.
- Rely on `view-transition-class` alone for freeze.
- `display: none` on both `::view-transition-old(root)` and `::view-transition-new(root)`.
- `display: none` on unchanged-chrome old **or** new (chat/threads/comments/header/tabs) — blanks the column during sibling GET.
- Reintroduce fetch/morph intercept for sibling docs (rejected as overcomplicated).
- Opacity-dim whole panes as a “soft” transition.
- Blame the browser before auditing names: unmatched or missing shared names are our bug.

## Name map (current)

| selector | name | class |
| --- | --- | --- |
| `#app-header` | `app-header` | `workbench-chrome` |
| `#workbench-mobile-tabs` | `workbench-mobile-tabs` **only `@media (max-width: 767px)`**; `none` on `md+` | `workbench-chrome` (max-md only) |
| `#workbench-v2-threads` | `workbench-v2-threads` | `workbench-chrome` |
| `#workbench-v2-chat` | `workbench-v2-chat` | `workbench-chrome` |
| `#workbench-v2-comments` | `workbench-v2-comments` | `workbench-chrome` |
| `#thread-artifact-path-header` | `thread-artifact-path-header` | `workbench-chrome` |
| `#thread-artifact-browser` | `thread-artifact-browser` | `workbench-chrome` |
| `#thread-artifact-document` | `thread-artifact-document` | — |
| `#workbench-root` / `#workbench-regions` / `#workbench-v2-artifact` / pane | `none` | — |

## Key files

- `static/css/index.css` — `@view-transition { navigation: auto }`, names (incl. desktop `#app-header`), per-name freeze (`animation: none` both sides for unchanged chrome), root old-only hide, path-browser/doc old-hide
- `server/layouts/root.templ` — `<meta name="view-transition" content="same-origin">` (cross-document VT opt-in)
- `static/js/workbench-history.js` — pushState patch flag + popstate reload gate + pagereveal `pinChatToBottom` after VT (no morph)
- `server/layouts/workbench/mobile.templ` — SSR selected tab + ActiveRegionID deep-link hardening
- `server/services/markdown/thread_navigation.templ` — sibling `<a href>` GETs / Files cookie
- `pkg/e2e/workbenchv2tests/workbench_v2_doc_vt_e2e_test.go` — under-tabs (mobile) + under-header (desktop) chrome Stories
- `server/layouts/root.templ` — `#app-header` stable id for desktop VT freeze

## History (brief)

We tried morph intercept, then over-cleared names, then class-only freeze + root old+new `display: none`, then unchanged-chrome `display: none` on new (chat blank flash on box) — each caused flashes (including under-tabs black on mobile Chromium). Later: globally named `#workbench-mobile-tabs` (`md:hidden`) still painted a VT snapshot during desktop sibling GETs (in-frame flash). Current shape: stable shared names for **visible** chrome only, mobile-tabs named under max-md / `none` on md+, per-name `animation: none` freeze (unchanged chrome: no display:none on old or new), old-root-only hide, path/browser/doc old-hide, plain sibling GETs, tiny history gate.

## Debugging checklist

1. Confirm matching `view-transition-name`s exist on **both** documents for every chrome region that should persist.
2. Confirm parents stay `none` so they do not swallow child shared elements.
3. Confirm freeze is per-name (not class-only).
4. Confirm root hides **old only**.
5. Run `workbench-v2-mobile-sibling-doc-keeps-chrome-under-tabs` at mobile viewport before blaming the browser.
