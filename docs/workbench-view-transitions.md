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
7. Unchanged chrome (`workbench-v2-chat|threads|comments`, `app-header`, `workbench-mobile-tabs`): **`animation: none` on both `::view-transition-old` and `::view-transition-new`** — do **not** `display: none` either side (blanking old or new flashes/remounts the column). Path/browser selection *does* change — hide **old** there so the new selection shows. Document: hide old only (live swap). **Chat naming is contextual:** keep `#workbench-v2-chat` named + frozen for **sibling artifact** GETs (same `/threads/:id`, chat DOM unchanged). On **thread→thread** GETs (`SharedThreadChat` remounts), unname chat for that navigation only (`html[data-wb2-vt-nav=thread-switch]` → `view-transition-name: none` via `workbench-history.js` pageswap/pagereveal + click fallback). Never `display: none` on `::view-transition-new(workbench-v2-chat)`.
8. Root: `animation: none` on group/old/new; **`display: none` only on `::view-transition-old(root)`**. Never blank both old **and** new root — that wipes unmatched chrome and causes under-tabs black on mobile Chromium.
9. Prefer real `<a href>` sibling GETs. No fetch/morph click intercept for file nav.
10. Minimal JS only (`static/js/workbench-history.js`): Enter/Up `pushState({ workbenchArtifactPatch })` + popstate reload gate so Back never blanks; after `pagereveal` / `viewTransition.finished` and `pageshow(!persisted)`, `pinChatToBottom` (double rAF + `fonts.ready`) pins whichever of `#agent-chat-messages` / `#agent-chat-scroll-region` actually overflows, `scrollIntoView` on `#chat-latest`, then focuses with `preventScroll`; Files cookie on templ button + SSR; history.js stays tiny.
11. SSR selected mobile tab classes + `ActiveRegionID=workbench-v2-artifact` on `?artifact=` deep-links (hardening against Datastar bind flash).
12. Regression gates: Playwright Story `workbench-v2-mobile-sibling-doc-keeps-chrome-under-tabs` (mobile under-tabs), `workbench-v2-desktop-sibling-doc-keeps-chrome-under-header` (desktop under-header: header + threads/chat/path/browser stay painted; chat column opacity stays visible across frames; `#agent-chat-messages.scrollTop` near `scrollHeight`), and `workbench-v2-desktop-sibling-doc-keeps-threads-reopen-chrome` (closed threads: `#workbench-v2-threads-reopen` stays named + painted across sibling GET; `wb2_threads_open=0`). Thread→thread (chat remount) residual gate: `workbench-v2-desktop-thread-switch-keeps-chat-under-header` (beta→Alpha with threads open: chat unnamed for that VT; chat/threads stay painted under header across VT frames; no large header→chat / chat→scroll gap — ~16px gutter is OK; pin tight).

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

| selector | name | class | media |
| --- | --- | --- | --- |
| `#app-header` | `app-header` | `workbench-chrome` | all |
| `#workbench-mobile-tabs` | `workbench-mobile-tabs` / `none` | `workbench-chrome` | named `@media (max-width: 767px)` only; `view-transition-name: none` on `md+` |
| `#workbench-v2-threads` | `workbench-v2-threads` | `workbench-chrome` | all |
| `#workbench-v2-threads-reopen` | `workbench-v2-threads-reopen` | `workbench-chrome` | desktop (`max-md:hidden`) |
| `#workbench-v2-chat` | `workbench-v2-chat` / `none` on thread→thread | `workbench-chrome` | named by default (sibling artifact freeze); `view-transition-name: none` while `html[data-wb2-vt-nav=thread-switch]` (thread→thread only) |
| `#workbench-v2-comments` | `workbench-v2-comments` | `workbench-chrome` | all |
| `#thread-artifact-path-header` | `thread-artifact-path-header` | `workbench-chrome` | all |
| `#thread-artifact-browser` | `thread-artifact-browser` | `workbench-chrome` | all |
| `#thread-artifact-document` | `thread-artifact-document` | — | all |
| `#workbench-root` / `#workbench-regions` / `#workbench-v2-artifact` / pane | `none` | — | all |

`/thoughts` document and directory workbench panes reuse `ThreadArtifactPane` (same path-header / Files browser / document IDs) so sibling GETs keep identical chrome. Header overflow reuses `BuildThreadArtifactHeaderActions` and adds a **Chat** link on thoughts pages. `#workbench-v2-threads-reopen` has stable name + per-name `animation: none` freeze; desktop Story `workbench-v2-desktop-sibling-doc-keeps-threads-reopen-chrome` gates closed-threads reopen paint across sibling GETs.


## Ephemeral chrome cookies vs layout prefs (SSR)

Sibling artifact navigation is a **full same-origin GET**. Client Datastar signals die with the document. Anything that must stay open/closed across that GET has to arrive again on first paint from the server.

### Two stores (do not mix)

| Store | Survives sibling GET? | What it holds on Threads |
| --- | --- | --- |
| **Layout prefs** (DB via `layoutprefs`, grip drag / `workbench-layout-save`) | ratios yes; **visibility no** | Column **ratios** only |
| **Host cookies** (`wb2_*`, `path=/; SameSite=Lax`) | yes (browser sends them on the next GET) | Ephemeral chrome open/closed |

**Why prefs drop `Visible` on Threads**

- `MergeWorkbenchConfig` applies saved **ratios** for every page, but only merges saved `Visible` when `defaults.Page != WorkbenchPageThreads` (desktop). Threads never rehydrate open/closed from prefs.
- `StripDurableInteractionState` (used on layoutprefs Upsert/Get as the “ratioOnly” strip) resets `Visible` (and Threads mobile `ActiveRegionID`) back to page defaults before storage/read — so a closed threads column cannot hide in the DB and surprise the next paint.

Route-owned visibility for Threads therefore comes from **Serve* args**, not from saved prefs:

- `ServeThreads` / `ServeThread` → `ThreadsOpen: workbench.ThreadsOpenFromRequest(r)` → `BuildWorkbenchV2State` → region `.Visible` → `EncodeWorkbenchSignals` / `data-signals` on first paint.
- Files browser: `ArtifactBrowserOpenFromRequest` → `ThreadArtifactBrowserArgs.BrowserOpen` → SSR `$_artifactBrowserOpen` seed in `thread_navigation.templ`.

| Cookie | Local signal | FromRequest | SSR seed |
| --- | --- | --- | --- |
| `wb2_threads_open` | `$workbench.regions.workbenchV2Threads.visible` (region) | `ThreadsOpenFromRequest` | `EncodeWorkbenchSignals` |
| `wb2_artifact_browser` | `$_artifactBrowserOpen` (underscore / pane-local) | `ArtifactBrowserOpenFromRequest` | `data-signals` on `#thread-artifact-pane` |

Toggle handlers write **both** the live signal and the cookie (see `ThreadsHideClickAction` / `ThreadsShowClickAction`, Files button `data-on:click`). Missing/invalid cookie ⇒ **open** (matches prior always-open first visit).

Underscore signals (`$_…`) are local to the pane morph world; they do **not** automatically ride to the next document. The cookie is what the next GET sends so SSR can re-seed them.

### Add-toggle recipe (SSR checklist)

1. **Classify**: workbench **region** visibility (`$workbench.regions.<SignalKey>.visible`) vs pane-local **`$_…`** signal. Regions go through `BuildWorkbenchV2State` + `EncodeWorkbenchSignals`; locals seed via templ `data-signals`.
2. **Persist for GET**: host cookie (`wb2_<name>`, `0`/`1`, `path=/; SameSite=Lax; Max-Age=…`) written in the click action alongside the signal flip. Optional `sessionStorage` mirror is fine; cookie is authoritative for SSR. **Default: missing/invalid cookie ⇒ open** (`*FromRequest` must not invent “default closed”).
3. **Read on Serve**: `*FromRequest(r)` in `ServeThreads` / `ServeThread` (and any sibling artifact Serve that must match) → pass into build/args → first-paint signals. Do **not** rely on client-only flips surviving navigation.
4. **Never** put chrome open/closed into layout-save / prefs for Threads visibility — prefs stay ratio-only (`Merge` + `StripDurableInteractionState`).
5. **Tests**: FromRequest defaults + `0`/`1`; cookie write present in click action; cookie drives `EncodeWorkbenchSignals` / SSR attribute (see `threads_open_test.go`, `TestArtifactBrowserOpenFromRequest`).
6. **VT first paint**: if the toggle reveals/hides named chrome (e.g. `#workbench-v2-threads` vs `#workbench-v2-threads-reopen`), keep stable `view-transition-name` + per-name `animation: none` freeze on **both** old and new so a closed→closed sibling GET does not flash remount. Do not `display: none` unchanged-chrome `::view-transition-new`.

## Key files

- `static/css/index.css` — `@view-transition { navigation: auto }`, names (incl. desktop `#app-header`, `#workbench-v2-threads-reopen`), per-name freeze (`animation: none` both sides for unchanged chrome), `html[data-wb2-vt-nav=thread-switch] #workbench-v2-chat { view-transition-name: none }`, root old-only hide, path-browser/doc old-hide
- `server/layouts/root.templ` — `<meta name="view-transition" content="same-origin">` (cross-document VT opt-in); `#app-header` stable id for desktop VT freeze
- `static/js/workbench-history.js` — pushState patch flag + popstate reload gate + pagereveal `pinChatToBottom` after VT (no morph); thread→thread chat unname (`data-wb2-vt-nav=thread-switch` via pageswap/pagereveal + sidebar click fallback; cleared after `viewTransition.finished`)
- `server/layouts/workbench/mobile.templ` — SSR selected tab + ActiveRegionID deep-link hardening
- `server/layouts/workbench/threads_open.go` — `wb2_threads_open` cookie ↔ `ThreadsOpenFromRequest` / click actions
- `server/layouts/workbench/defaults.go` — `MergeWorkbenchConfig` (Threads skips saved `Visible`) + `StripDurableInteractionState` (ratioOnly strip)
- `server/services/markdown/thread_workbench.go` — `ServeThreads` / `ServeThread` pass `ThreadsOpenFromRequest` into `BuildWorkbenchV2State`
- `server/services/markdown/thread_navigation.templ` — sibling `<a href>` GETs / Files `wb2_artifact_browser` + `$_artifactBrowserOpen` SSR seed
- `pkg/e2e/workbenchv2tests/workbench_v2_doc_vt_e2e_test.go` — under-tabs (mobile) + under-header (desktop) chrome Stories

## History (brief)

We tried morph intercept, then over-cleared names, then class-only freeze + root old+new `display: none`, then unchanged-chrome `display: none` on new (chat blank flash on box) — each caused flashes (including under-tabs black on mobile Chromium). Later: globally named `#workbench-mobile-tabs` (`md:hidden`) still painted a VT snapshot during desktop sibling GETs (in-frame flash). Current shape: stable shared names for **visible** chrome only, mobile-tabs named under max-md / `none` on md+, per-name `animation: none` freeze (unchanged chrome: no display:none on old or new), chat named for sibling artifact / **unnamed for thread→thread** (`data-wb2-vt-nav`), old-root-only hide, path/browser/doc old-hide, plain sibling GETs, tiny history gate.

## Debugging checklist

1. Confirm matching `view-transition-name`s exist on **both** documents for every chrome region that should persist.
2. Confirm parents stay `none` so they do not swallow child shared elements.
3. Confirm freeze is per-name (not class-only).
4. Confirm root hides **old only**.
5. Run `workbench-v2-mobile-sibling-doc-keeps-chrome-under-tabs` at mobile viewport before blaming the browser.
