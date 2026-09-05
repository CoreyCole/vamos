const COMPOSER_FOCUS_KEY = "workbench-v2:composer-focused";
const DOC_SWITCH_ATTR = "data-workbench-doc-switching";

function reloadThreadArtifactHistory() {
  const navigation = performance.getEntriesByType("navigation")[0];
  // Same-document popstate after Enter/Up pushState keeps type "navigate".
  // Native document Back/Forward is type "back_forward" -- do not blast it.
  if (navigation && navigation.type === "back_forward") {
    return;
  }
  const threadRoute =
    window.location.pathname === "/threads" ||
    window.location.pathname.startsWith("/threads/");
  if (threadRoute && document.getElementById("thread-artifact-pane")) {
    window.location.reload();
  }
}

function isThreadRoute() {
  return (
    window.location.pathname === "/threads" ||
    window.location.pathname.startsWith("/threads/")
  );
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
  markDocSwitchPending();
}

function initNavPolish() {
  clearDocSwitchPending();
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      restoreComposerFocus();
    });
  });
}

window.addEventListener("popstate", reloadThreadArtifactHistory);
window.addEventListener("pagehide", () => persistComposerFocus());
document.addEventListener("click", onArtifactFileClick, true);
initNavPolish();
