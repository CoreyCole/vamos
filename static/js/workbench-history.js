const COMPOSER_FOCUS_KEY = "workbench-v2:composer-focused";
const DOC_SWITCH_KEY = "workbench-v2:doc-switch";
const DOC_SWITCH_ATTR = "data-workbench-doc-switching";
const DOC_SWITCH_TYPE = "workbench-doc-switch";

const FILE_ACTIVE_CLASS =
  "relative z-0 block min-w-0 truncate rounded-r px-2 py-1 text-xs transition-colors border-l-2 border-primary bg-muted font-medium text-foreground";
const FILE_INACTIVE_CLASS =
  "relative z-0 block min-w-0 truncate rounded-r px-2 py-1 text-xs transition-colors border-l-2 border-transparent text-muted-foreground hover:bg-muted/60 hover:text-foreground";

let artifactDocAbort = null;
let artifactDocSeq = 0;

function isThreadRoute() {
  return (
    window.location.pathname === "/threads" ||
    window.location.pathname.startsWith("/threads/")
  );
}

function sameLocation(href) {
  try {
    const url = new URL(href, window.location.href);
    return (
      url.origin === window.location.origin &&
      url.pathname === window.location.pathname &&
      url.search === window.location.search
    );
  } catch (_) {
    return false;
  }
}

function onPopState() {
  // Enter/Up folder patches pushState({ workbenchArtifactPatch: true }).
  if (history.state?.workbenchArtifactPatch) {
    if (isThreadRoute() && document.getElementById("thread-artifact-pane")) {
      window.location.reload();
    }
    return;
  }
  // Sibling artifact docs: fetch+morph — never location.reload().
  if (history.state?.workbenchArtifactDoc) {
    if (isThreadRoute() && document.getElementById("thread-artifact-pane")) {
      navigateArtifactDoc(window.location.href, { historyMode: "none" });
    }
  }
}

function composerEl() {
  return document.getElementById("agent-chat-composer-input");
}

function isEditableTarget(el) {
  if (!el || el === document.body || el === document.documentElement) {
    return false;
  }
  if (el.isContentEditable) return true;
  const tag = el.tagName;
  return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT";
}

function markDocSwitchPending() {
  document.documentElement.setAttribute(DOC_SWITCH_ATTR, "true");
}

function clearDocSwitchPending() {
  document.documentElement.removeAttribute(DOC_SWITCH_ATTR);
}

function setDocSwitchFlag() {
  try {
    sessionStorage.setItem(DOC_SWITCH_KEY, "1");
  } catch (_) {}
}

function peekDocSwitchFlag() {
  try {
    return sessionStorage.getItem(DOC_SWITCH_KEY) === "1";
  } catch (_) {
    return false;
  }
}

function consumeDocSwitchFlag() {
  try {
    const on = sessionStorage.getItem(DOC_SWITCH_KEY) === "1";
    if (on) sessionStorage.removeItem(DOC_SWITCH_KEY);
    return on;
  } catch (_) {
    return false;
  }
}

function persistComposerFocus() {
  if (!isThreadRoute()) return;
  const active = document.activeElement;
  const composer = composerEl();
  try {
    if (composer && active === composer) {
      sessionStorage.setItem(COMPOSER_FOCUS_KEY, "1");
    } else if (isEditableTarget(active)) {
      sessionStorage.setItem(COMPOSER_FOCUS_KEY, "0");
    }
  } catch (_) {}
}

function restoreComposerFocus() {
  if (!isThreadRoute()) return;
  let wantComposer = false;
  try {
    wantComposer = sessionStorage.getItem(COMPOSER_FOCUS_KEY) === "1";
    sessionStorage.removeItem(COMPOSER_FOCUS_KEY);
  } catch (_) {}
  const active = document.activeElement;
  if (isEditableTarget(active)) return;
  const composer = composerEl();
  if (!composer) return;
  if (
    !wantComposer &&
    active &&
    active !== document.body &&
    active !== document.documentElement
  ) {
    return;
  }
  try {
    composer.focus({ preventScroll: true });
  } catch (_) {
    composer.focus();
  }
}

function setArtifactLoading(on) {
  const pane = document.getElementById("thread-artifact-pane");
  if (!pane) return;
  if (on) {
    pane.setAttribute("aria-busy", "true");
    markDocSwitchPending();
  } else {
    pane.removeAttribute("aria-busy");
    clearDocSwitchPending();
  }
}

function syncPathHeader(nextDoc) {
  const liveHeader = document.getElementById("thread-artifact-path-header");
  const nextHeader = nextDoc.getElementById("thread-artifact-path-header");
  if (!liveHeader || !nextHeader) return;

  const livePath = liveHeader.querySelector("span.min-w-0.flex-1.truncate");
  const nextPath = nextHeader.querySelector("span.min-w-0.flex-1.truncate");
  if (livePath && nextPath) {
    livePath.textContent = nextPath.textContent;
  }

  // Keep Files toggle; replace trailing actions (Thoughts / Copy path / …).
  while (liveHeader.children.length > 2) {
    liveHeader.removeChild(liveHeader.lastElementChild);
  }
  const nextKids = Array.from(nextHeader.children);
  for (let i = 2; i < nextKids.length; i++) {
    liveHeader.appendChild(document.importNode(nextKids[i], true));
  }
}

function syncFileSelection(href) {
  let target;
  try {
    target = new URL(href, window.location.href);
  } catch (_) {
    return;
  }
  document.querySelectorAll("a[data-thread-artifact-file]").forEach((a) => {
    let active = false;
    try {
      const u = new URL(a.href, window.location.href);
      active = u.pathname === target.pathname && u.search === target.search;
    } catch (_) {}
    if (active) {
      a.setAttribute("aria-current", "page");
      a.className = FILE_ACTIVE_CLASS;
    } else {
      a.removeAttribute("aria-current");
      a.className = FILE_INACTIVE_CLASS;
    }
  });
}

function patchBrowserList(nextDoc) {
  const liveNav = document.getElementById("thread-artifact-browser");
  const nextNav = nextDoc.getElementById("thread-artifact-browser");
  if (!liveNav || !nextNav) return false;
  const liveCwd = liveNav.getAttribute("data-thread-artifact-cwd") || "";
  const nextCwd = nextNav.getAttribute("data-thread-artifact-cwd") || "";
  if (liveCwd === nextCwd) return false;

  const cloned = document.importNode(nextNav, true);
  // Preserve open/closed visibility from the live shell (Datastar may own it).
  if (liveNav.hasAttribute("style")) {
    cloned.setAttribute("style", liveNav.getAttribute("style") || "");
  } else {
    cloned.removeAttribute("style");
  }
  const liveShow = liveNav.getAttribute("data-show");
  if (liveShow != null) cloned.setAttribute("data-show", liveShow);
  liveNav.replaceWith(cloned);
  return true;
}

function morphArtifactDocument(nextDoc) {
  const liveDoc = document.getElementById("thread-artifact-document");
  const nextArtifact = nextDoc.getElementById("thread-artifact-document");
  if (!liveDoc || !nextArtifact) return false;
  liveDoc.replaceWith(document.importNode(nextArtifact, true));
  return true;
}

function applyArtifactMorph(nextDoc, href) {
  if (!morphArtifactDocument(nextDoc)) {
    throw new Error("missing #thread-artifact-document");
  }
  syncPathHeader(nextDoc);
  patchBrowserList(nextDoc);
  syncFileSelection(href);
  if (nextDoc.title) {
    document.title = nextDoc.title;
  }
}

async function fetchArtifactHtml(href, signal) {
  const res = await fetch(href, {
    method: "GET",
    credentials: "same-origin",
    headers: { Accept: "text/html" },
    signal,
  });
  if (!res.ok) {
    throw new Error("artifact fetch failed: " + res.status);
  }
  return res.text();
}

async function navigateArtifactDoc(href, { historyMode }) {
  if (!document.getElementById("thread-artifact-pane")) {
    window.location.assign(href);
    return;
  }
  if (historyMode !== "none" && sameLocation(href)) {
    return;
  }

  if (artifactDocAbort) {
    try {
      artifactDocAbort.abort();
    } catch (_) {}
  }
  const ac = new AbortController();
  artifactDocAbort = ac;
  const seq = ++artifactDocSeq;

  setArtifactLoading(true);
  try {
    const html = await fetchArtifactHtml(href, ac.signal);
    if (seq !== artifactDocSeq) return;
    const nextDoc = new DOMParser().parseFromString(html, "text/html");
    if (!nextDoc.getElementById("thread-artifact-document")) {
      window.location.assign(href);
      return;
    }
    applyArtifactMorph(nextDoc, href);

    if (historyMode === "push") {
      // Tag the current entry so Back morphs instead of falling through.
      if (
        !history.state?.workbenchArtifactDoc &&
        !history.state?.workbenchArtifactPatch
      ) {
        const cur =
          history.state && typeof history.state === "object"
            ? { ...history.state }
            : {};
        cur.workbenchArtifactDoc = true;
        history.replaceState(cur, "", window.location.href);
      }
      history.pushState({ workbenchArtifactDoc: true }, "", href);
    }
  } catch (err) {
    if (err && err.name === "AbortError") return;
    window.location.assign(href);
  } finally {
    if (seq === artifactDocSeq) {
      setArtifactLoading(false);
      if (artifactDocAbort === ac) artifactDocAbort = null;
    }
  }
}

function onArtifactFileClick(event) {
  if (event.defaultPrevented) return;
  if (event.button !== 0) return;
  if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  const link = event.target?.closest?.("a[data-thread-artifact-file]");
  if (!link || !link.href) return;

  let url;
  try {
    url = new URL(link.href, window.location.href);
  } catch (_) {
    return;
  }
  if (url.origin !== window.location.origin) return;
  // Thoughts / other routes: still full navigation.
  if (!url.pathname.startsWith("/threads/") && url.pathname !== "/threads") {
    return;
  }
  if (!document.getElementById("thread-artifact-pane")) return;

  event.preventDefault();
  try {
    sessionStorage.setItem(COMPOSER_FOCUS_KEY, "1");
  } catch (_) {}

  navigateArtifactDoc(url.href, { historyMode: "push" });
}

function prefetchArtifactHref(href) {
  if (!href) return;
  try {
    const url = new URL(href, window.location.href);
    if (url.origin !== window.location.origin) return;
    if (
      url.pathname === window.location.pathname &&
      url.search === window.location.search
    ) {
      return;
    }
    const abs = url.pathname + url.search;
    if (
      document.head.querySelector(
        `link[rel="prefetch"][data-workbench-prefetch="${CSS.escape(abs)}"]`,
      )
    ) {
      return;
    }
    const link = document.createElement("link");
    link.rel = "prefetch";
    link.href = url.href;
    link.setAttribute("data-workbench-prefetch", abs);
    document.head.appendChild(link);
  } catch (_) {}
}

function onArtifactPrefetchIntent(event) {
  const link = event.target?.closest?.("a[data-thread-artifact-file]");
  if (!link?.href) return;
  prefetchArtifactHref(link.href);
}

function onPageSwap(event) {
  if (!peekDocSwitchFlag()) return;
  if (event.viewTransition) {
    try {
      event.viewTransition.types.add(DOC_SWITCH_TYPE);
    } catch (_) {}
  }
}

function onPageReveal(event) {
  const switching = consumeDocSwitchFlag();
  if (event.viewTransition && switching) {
    try {
      event.viewTransition.types.add(DOC_SWITCH_TYPE);
    } catch (_) {}
    markDocSwitchPending();
    const done = () => clearDocSwitchPending();
    try {
      event.viewTransition.finished.then(done, done);
    } catch (_) {
      done();
    }
    return;
  }
  // No VT (unsupported, reduced-motion, skipped): do not leave opacity hacks.
  clearDocSwitchPending();
}

function onPageShow(event) {
  // bfcache restore: never reload; chrome is already painted.
  if (event.persisted) {
    clearDocSwitchPending();
    return;
  }
}

function initNavPolish() {
  // pagereveal / pageshow(persisted) own DOC_SWITCH_ATTR lifecycle — do not clear here.
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      restoreComposerFocus();
    });
  });
}

window.addEventListener("popstate", onPopState);
window.addEventListener("pageshow", onPageShow);
window.addEventListener("pageswap", onPageSwap);
window.addEventListener("pagereveal", onPageReveal);
window.addEventListener("pagehide", () => persistComposerFocus());
document.addEventListener("click", onArtifactFileClick, true);
document.addEventListener("pointerdown", onArtifactPrefetchIntent, true);
document.addEventListener("focusin", onArtifactPrefetchIntent, true);
initNavPolish();
