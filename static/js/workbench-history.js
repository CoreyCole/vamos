const COMPOSER_FOCUS_KEY = "workbench-v2:composer-focused";
const DOC_SWITCH_KEY = "workbench-v2:doc-switch";
const DOC_SWITCH_ATTR = "data-workbench-doc-switching";
const DOC_SWITCH_TYPE = "workbench-doc-switch";

function isThreadRoute() {
  return (
    window.location.pathname === "/threads" ||
    window.location.pathname.startsWith("/threads/")
  );
}

function reloadThreadArtifactHistory() {
  // Enter/Up folder patches pushState({ workbenchArtifactPatch: true }).
  // Native Back/Forward between real artifact GETs (and bfcache) must NOT reload
  // — that blanks path/tree/document under stable Workspace tabs.
  if (!history.state?.workbenchArtifactPatch) {
    return;
  }
  if (isThreadRoute() && document.getElementById("thread-artifact-pane")) {
    window.location.reload();
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

function onArtifactFileClick(event) {
  if (event.defaultPrevented) return;
  if (event.button !== 0) return;
  if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  const link = event.target?.closest?.("a[data-thread-artifact-file]");
  if (!link || !link.href) return;
  // Keep real GET anchors; only add continuity + light loading affordance.
  try {
    sessionStorage.setItem(COMPOSER_FOCUS_KEY, "1");
  } catch (_) {}
  setDocSwitchFlag();
  markDocSwitchPending();
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

window.addEventListener("popstate", reloadThreadArtifactHistory);
window.addEventListener("pageshow", onPageShow);
window.addEventListener("pageswap", onPageSwap);
window.addEventListener("pagereveal", onPageReveal);
window.addEventListener("pagehide", () => persistComposerFocus());
document.addEventListener("click", onArtifactFileClick, true);
document.addEventListener("pointerdown", onArtifactPrefetchIntent, true);
document.addEventListener("focusin", onArtifactPrefetchIntent, true);
initNavPolish();
