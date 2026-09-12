# Design: Declaring Datastar SSE UIs in Vamos

**Status:** LGTM by Vamos Lead (2026-09-12) — land in /q-workspace tip; phase-1 extract not started  
**Author:** Vamos Datastar Architect  
**Audience:** Vamos Lead → then FE/UX implement; DatastarUI Lead boundary **confirmed**  
**Non-goals this turn:** landing code, DatastarUI API changes, VT anti-jank library APIs (research until Corey greenlights)

---

## 1. Problem

Vamos already has a working CQRS/SSE style (best shown by `/workspaces`), Workbench VT chrome contracts, and DatastarUI primitives — but page authors still assemble these ad hoc:

- Route registration, page GET, long-lived `/stream`, short POSTs, and fat morphs are copy-pasted per feature.
- Morph-target IDs, signal namespaces, cookie↔SSR re-seed, and Story assertions are conventions in docs/tribal knowledge, not a predictable declaration surface.
- Risk: chrome patches and feature patches blur; signals grow into app state; SSR first paint lies; Stories assert DOM trivia instead of contracts.

We want a **thin framework-esque layer** (conventions + small helpers) so a new page is predictable without forking DatastarUI or inventing a second component library.

---

## 2. Design principles (locked)

Aligned with `docs/datastar-cqrs-pages.md`, AGENTS.md Tao of Datastar, Workbench VT guide, DatastarUI Lead:

| Principle | Meaning in vamos |
| --- | --- |
| **Backend source of truth** | Application state lives in DB/projections/filesystem; HTML is a projection. |
| **Honest SSR first paint** | Initial GET HTML is useful even if SSE never connects. |
| **Sparse signals** | Signals are UI-only (open/closed, loading/indicators, ephemeral chrome). Never the domain model. |
| **Fat morphs** | Stream/POST responses re-query backend, render **full** components, `PatchElementTempl` onto **stable IDs**. |
| **MPA for new resources** | Sibling/new resource → real `<a>` GET (+ CSS VT where contracted). Same page update → SSE morph. |
| **Consume, don’t fork** | Primitives from `pkg/datastarui` (CLI copy). App structure lives in `server/…` + thin helpers. |
| **Stories assert contracts** | Stories lock morph IDs, SSR seeds, VT chrome names, cookie↔signal ownership — not pixel trivia alone. |

---

## 3. Ownership split (DatastarUI vs vamos)

**Confirmed with DatastarUI Lead (2026-09-12).**

| Layer | Owner | Lives in |
| --- | --- | --- |
| Reusable components, expression builders, Story runtime (`e2e/spec` / `e2e/runtime`), future vt/morph guardrails | **DatastarUI Lead** | `github.com/coreycole/datastarui` → CLI copy → `pkg/datastarui` + Go module pin for e2e |
| Page CQRS declaration, layout/chrome composition, app morph-target ID *values* / name-map, app-domain signal encoding, `pkg/e2e/vamos` Story helpers | **Vamos Datastar Architect (+ FE)** | `server/…`, proposed `server/pages/` helpers, `pkg/e2e/vamos` |
| Auth/run machinery for how app Stories execute | **Vamos E2E Lead** | pins DatastarUI library Story surface |
| Feel / VA / VT chrome naming map | **UX** (with Architect contracts) | docs + Stories; CSS names stay hand-authored |
| Sequencing / q-workspace | **Vamos Lead** | ruby `/q-workspace` only |

### IN (vamos page framework)

- Handler / CQRS page declaration conventions (long read / short write, honest SSR)
- Layout/chrome composition + **app** morph-target ID values and name-map
- App-domain signal encoding (feature-shaped payloads) — not a generic Datastar expression DSL
- App Story helpers that *compose* DatastarUI primitives (`pkg/e2e/vamos` + workbench tests)
- Docs for vamos MVC/SSE structure

### OUT (don’t reinvent — DatastarUI / upstream)

- Component internals / variants / component-specific `expressions.go`
- Generic expression builders (`utils/expressions.go` or component packages)
- Generic Story/runtime (`spec.Story`, viewports, `Visit`, `ConsoleClean`, `Custom`, artifact/runner contract)
- Future generic VT asserts (names / uniqueness / opt-in / no-named-hidden-chrome / optional chrome sampling) — research seam is `e2e/spec` or `e2e/vt`, not a vamos fork
- Fat-morph / stable-ID *library* guardrails when they land upstream
- Local forks of `pkg/datastarui/components/**`

### Gray (start in vamos; promote only if a second consumer)

- Typed chrome name-map schema `{selector,name,media,mustBeNone}`
- `ViewTransitionName(scope,id)` sanitizer (today in workbench)

### Exists today vs do-not-assume

- **Exists:** components + expression builders; Story API + viewports (mobile / desktop-half / desktop-full); `Custom` escape hatch; demo opt-in meta + `@view-transition { navigation: auto }` (no name rules in DSUI demo CSS).
- **Do NOT assume landed:** vtguard / ExpectViewTransition* / rAF chrome sampler / MPA VT runner waits. Treat VT enforcement as app `Custom`+CSS until Corey greenlights a DatastarUI slice. Design page framework so it can *call* upstream asserts later without forking.
- **Morph stable-ID static checks:** deferred in anti-jank v1 research — app MorphMap convention OK for now.

### DatastarUI Lead sharpenings (2026-09-12 sanity check)

Explicitly OUT / wait for DSUI:
1. Do not reimplement `SignalManager` / generic expression builders — import DatastarUI; propose reusable helpers upstream first.
2. No generic VT asserts / runner waits / rAF chrome sampling in the page layer — wait for DatastarUI `e2e/spec` (or `e2e/vt`) after Corey greenlight; until then workbench Custom+CSS stays UX/layout-special.
3. No component forks / app-only props in copied components — composition/wrappers in features, or upstream a real primitive.
4. No second Story runtime — page-contract Stories compose `datastarui/e2e/spec`.
5. No optimistic client state / morph-nav replacing `<a>`+VT for page changes (Tao) — keep out of PageSpec.

Fine in vamos without waiting: app MorphMap values, sparse signal shapes + SSR defaults, thin `PatchMorphs`/`StreamLoop`, name-map schema + `ViewTransitionName` sanitizer until a second consumer.

---

## 4. Proposed page declaration model

Every Datastar SSE page is a small, named bundle — a **Page** — with five sealed parts:

```text
Page = Routes + Model + Document + Morphs + Signals(+SSR)
         ↑        ↑        ↑         ↑           ↑
       wiring   cheap    honest    fat patches  sparse UI
                reads    first paint
```

### 4.1 Mental model (MVC × CQRS × Tao)

| Classic MVC | CQRS/SSE mapping |
| --- | --- |
| **Model** | `buildXPageModel(ctx, filter)` — DB/projection reads only; no shell/git/network fanout |
| **View** | templ Document + morph partials sharing the same model |
| **Controller** | GET page, GET stream, POST commands |
| **Read side** | GET + stream → morph |
| **Write side** | short POST → mutate → morph (or redirect for non-Datastar) |

Northstar takeaway: feature folders with `routes.go` + `handlers.go` + `pages/*.templ` are a good *shape*; vamos already has richer domain packages — we add **declaration helpers**, not a Northstar clone.

### 4.2 Canonical route shape (from `/workspaces`)

```text
GET  /<page>              → HandlePage      (SSR Document; useful alone)
GET  /<page>/stream       → HandleStream    (initial fat morph + notifier loop)
POST /<page>/…            → HandleCommand   (short write → PatchElementTempl)
```

Fixture/read-only hosts may mount **only** GET page + stream (see `RegisterFixtureReadOnlyRoutes`).

### 4.3 Morph boundaries

Declare an explicit **MorphMap** per page — stable element IDs that stream/commands may outer-morph:

| Page | Morph IDs (example) |
| --- | --- |
| Workspaces | `workspaces-header`, `release-queue-panel`, `workspaces-list` |
| Workbench artifact (GET, not morph) | VT-named chrome; document is the live region — see VT guide |

Rules:

1. Every morph target has a **stable unique `id`** on SSR and every patch.
2. Stream patches **full** partials (`WithModeOuter`), not surgical inner edits of domain rows unless a Story contracts it.
3. Dropdown/dialog IDs inside morph trees are **instance-unique** (`workspace-dialog-{slug}`, cleanup confirm scoped).
4. Layout/chrome IDs are **owned by layouts**, not by feature morph maps (feature streams must not morph `#app-header` / workbench chrome).

### 4.4 Signal policy

Three signal classes — do not mix:

| Class | Purpose | Survives sibling GET? | Mechanism |
| --- | --- | --- | --- |
| **A. Ephemeral UI** | loading indicators, open menus | no (same document) | `data-indicator` / DatastarUI `Signals(...)` |
| **B. Chrome open/closed** | threads open, files browser | **yes** | host cookie + `*FromRequest` + SSR `data-signals` re-seed |
| **C. Layout ratios** | column flex | yes (DB) | layoutprefs — **ratios only**; never visibility on Threads |

Underscore signals (`$_…`) = pane-local morph world; they do **not** ride to the next document without a cookie.

Application/domain state stays **out** of signals (list contents, release queue, thread body, etc. come from model → HTML).

### 4.5 SSR honesty checklist (every page)

1. Document renders from the same `build*PageModel` the stream uses (or a documented cheaper subset that still looks correct).
2. Filter/query state comes from real form `name`s / URL — works without JS.
3. Progressive enhancement: POST handlers accept normal form posts + Datastar `@post`.
4. Initial stream failure must not blank the SSR content.
5. After initial stream patch, notify rebuild errors are logged; stream keeps listening when safe.

---

## 5. Thin helpers (proposed API sketch — not implement yet)

Place under `server/pages/` — **vamos-owned**, tiny (Lead lock):

### 5.1 `PageSpec`

```go
type PageSpec struct {
    Name       string          // "workspaces"
    Path       string          // "/workspaces"
    PageType   layouts.PageType
    MorphIDs   []string        // contract list
    StreamPath string          // default Path+"/stream"
}
```

### 5.2 `RegisterCQRS`

```go
// RegisterCQRS wires GET page, GET stream, and optional fixture-only subset.
func RegisterCQRS(g *echo.Group, spec PageSpec, h CQRSHandlers, opts ...Option)
```

Handlers interface (feature implements):

```go
type CQRSHandlers interface {
    ServePage(c echo.Context) error
    ServeStream(c echo.Context) error
}
```

Commands stay explicit on the feature (`POST /:slug/start`, etc.) — the helper does not auto-CRUD.

### 5.3 `PatchMorphs`

```go
// PatchMorphs applies ordered outer PatchElementTempl calls from a MorphPlan.
func PatchMorphs(sse *datastar.ServerSentEventGenerator, plan ...MorphPatch) error

type MorphPatch struct {
    ID       string
    Component templ.Component
}
```

Encodes the Workspaces pattern (`patchWorkspacesModel`) without each page re-handrolling selector/mode.

### 5.4 `StreamLoop`

```go
// StreamLoop: initial build+patch, subscribe notifier, on notify rebuild+patch;
// log notify errors and continue; exit on request cancel.
func StreamLoop(ctx context.Context, sse *datastar.ServerSentEventGenerator, n Notifier, patch func(source string) error) error
```

### 5.5 `SSRChrome` recipe helpers (layout-adjacent)

Documented helpers (some already exist for Workbench):

- `*FromRequest(r)` ↔ cookie write in click action
- `Encode*Signals` for first paint
- Checklist function or Story helper: “chrome toggle X has cookie + FromRequest + SSR seed”

Do **not** put VT name CSS codegen in v1 — keep hand CSS + `WorkbenchV2ChromeNames` table as today.

### 5.6 Story contract helpers (`pkg/e2e/vamos`)

Extend typed helpers so each PageSpec can assert:

1. Morph IDs present on SSR.
2. Stream patches only declared MorphIDs (optional mutation observer / network assert — phase 2).
3. Cookie↔signal ownership for chrome toggles.
4. “SSR useful without stream” smoke (disable stream / fail stream → content still there).

DatastarUI keeps the Story builder/runtime; vamos only adds **page-contract** helpers.

---

## 6. Layout / chrome / feature boundaries

```text
┌─────────────────────────────────────────────────────────┐
│ layouts.Root / Workbench shell                          │
│  - app header, nav, VT chrome names                     │
│  - cookie↔SSR chrome (threads, files)                   │
│  - MUST NOT be morph targets of feature /stream         │
├─────────────────────────────────────────────────────────┤
│ Feature Document (page body)                            │
│  - MorphMap regions (header/list/panels)                │
│  - DatastarUI primitives for controls                   │
│  - forms with name= ; stable ids                        │
└─────────────────────────────────────────────────────────┘
```

**Navigation rule (Tao):**

- Same resource update → SSE morph (Workspaces refresh/lifecycle).
- New resource / sibling artifact → plain GET (+ VT contract). Never fetch-intercept sibling docs.

---

## 7. Reference: Workspaces as the golden page

Treat `/workspaces` as the first **PageSpec exemplar** (already matches the model):

| Part | Implementation today |
| --- | --- |
| Model | `buildWorkspacesPageModel` — list/projection reads; latency logged |
| Document | `WorkspacesDocument` / SSR groups |
| Morphs | `workspaces-header`, `release-queue-panel`, `workspaces-list` |
| Stream | `HandleWorkspacesStream` + notifier; notify errors continue |
| Writes | short POSTs → `patchWorkspaces*` |
| Signals | action indicators + DatastarUI dialog/select signals; filters via real form GET |
| Destructive | confirm dialog before cleanup — never bare dropdown submit |
| Stories | extend to assert MorphMap + SSR-without-stream |

Refactor goal after LGTM: extract helpers **from** Workspaces without behavior change, then adopt on the next new page (not a big-bang rewrite of Workbench).

---

## 8. Story contracts (what “done” means for a page)

Minimum Story/contract suite for any new CQRS page:

1. **SSR first paint** — key content visible with stream blocked/failing.
2. **Morph IDs stable** — declared IDs exist before/after stream initial patch.
3. **Command morph** — one write updates the expected morph region(s) only.
4. **Destructive confirm** — cleanup/delete requires dialog confirm.
5. If chrome toggles: **cookie→SSR** round-trip on full GET.
6. If VT page: existing VT Stories remain the chrome gate (do not duplicate in feature Stories).

---

## 9. Phased rollout

| Phase | Deliverable | Who |
| --- | --- | --- |
| **0. This doc** | Lead LGTM + DatastarUI boundary ack | Architect → Lead / DatastarUI Lead |
| **1. Extract helpers** | `PageSpec` / `PatchMorphs` / `StreamLoop` from Workspaces; no UX change | FE (+ Architect review) in `/q-workspace` |
| **2. Document + Stories** | Expand `docs/datastar-cqrs-pages.md`; add Workspaces contract Stories | FE + E2E |
| **3. Adopt on next page** | First net-new or refactored page uses PageSpec | FE |
| **4. Optional** | MorphMap lint / Story network assert; DatastarUI vtguard when Corey greenlights | DatastarUI Lead |

Out of scope until later: generating templ from schemas; replacing Workbench with PageSpec; auto route codegen.

---

## 10. Lead LGTM locks (2026-09-12)

1. **Package home:** `server/pages` (not `pkg/ssepage`).
2. **Workbench:** stays layout-special in v1 — not forced into PageSpec.
3. **Phase-1 extract:** Workspaces-only.
4. **Doc location:** land as `docs/datastar-sse-ui-declaration.md` in a `/q-workspace` tip; keep `docs/datastar-cqrs-pages.md` as short rules (do not expand it into this).

Also LGTM: PageSpec / MorphMap / signal classes A–C / SSR checklist / Stories contracts; consume `pkg/datastarui`; no VT library API assumed.

**Next after land:** ping Lead with tip path + SHA. FE phase-1 extract when Lead sequences it — not yet.

---

## 11. Success criteria

Lead can LGTM if:

- [ ] Ownership split with DatastarUI is clear (consume primitives; vamos owns declaration).
- [ ] PageSpec / MorphMap / signal classes / SSR checklist are concrete enough for FE to implement without inventing structure.
- [ ] Workspaces is the exemplar; Workbench stays layout-special in v1.
- [ ] No DatastarUI fork; no VT library API assumed.
- [ ] Stories have an explicit contract list.

---

## Appendix A — Source anchors

- `docs/datastar-cqrs-pages.md` — CQRS rules
- `docs/workbench-view-transitions.md` — VT + cookie↔SSR
- `docs/datastarui-development.md` — copy/consume workflow
- `AGENTS.md` — Tao + Story ownership
- `server/services/workspaces/handler.go` — golden CQRS page
- `server/layouts/workbench/signals.go` — chrome signal encoding
- `pkg/datastarui/utils/signals.go` — DatastarUI `SignalManager`
- Northstar (`zangster300/northstar`) — feature folder shape; VT auto commented out (caution, not our path)

## Appendix B — DatastarUI Lead alignment (confirmed 2026-09-12)

- Boundary confirmed: see §3 IN / OUT / Gray.
- Partner **Vamos E2E Lead** for auth/run machinery; DatastarUI owns the library Story surface they pin.
- Anti-jank VT research with UX is research-first — **no DatastarUI API until Corey greenlights a slice**.
- DatastarUI Lead will sanity-check primitives vs app only — not rewrite MVC design.
- Product name-maps / workbench CSS / app sanitizers stay vamos unless a second app forces promotion.
