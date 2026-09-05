const CHAT_SCROLL_KEY = "workbench-v2:chat-scroll";
const CHAT_SCROLL_PENDING_KEY = "workbench-v2:chat-scroll-pending";
const COMPOSER_FOCUS_KEY = "workbench-v2:composer-focused";
const DOC_SWITCH_ATTR = "data-workbench-doc-switching";

/** @type {number|null} */
let pendingRestoreTop = null;

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

function chatScrollEl() {
  // Real transcript scroller (workbench-v2-chat-body is overflow-hidden).
  return (
    document.getElementById("agent-chat-scroll-region") ||
    document.querySelector("#workbench-v2-chat-body #agent-chat-scroll-region")
  );
}

function chatRegionVisible(chat) {
  if (!chat) return false;
  if (chat.clientHeight <= 0) return false;
  const style = window.getComputedStyle(chat);
  if (style.display === "none" || style.visibility === "hidden") return false;
  let node = chat;
  while (node && node !== document.documentElement) {
    const cs = window.getComputedStyle(node);
    if (cs.display === "none") return false;
    node = node.parentElement;
  }
  return true;
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

function persistChatContinuity({ pending = false } = {}) {
  if (!isThreadRoute()) return;
  const chat = chatScrollEl();
  if (chat) {
    try {
      // Only persist a meaningful mid-scroll. Hidden panes often report 0 and
      // must not pin the next Chat visit to the top.
      const top = chat.scrollTop;
      if (chatRegionVisible(chat) && top > 0) {
        sessionStorage.setItem(
          CHAT_SCROLL_KEY,
          JSON.stringify({
            path: window.location.pathname,
            top,
          }),
        );
        if (pending) {
          sessionStorage.setItem(CHAT_SCROLL_PENDING_KEY, "1");
        }
      } else if (pending) {
        // Intentional sibling nav without a restorable mid-scroll: clear stale.
        sessionStorage.removeItem(CHAT_SCROLL_KEY);
        sessionStorage.removeItem(CHAT_SCROLL_PENDING_KEY);
      }
    } catch (_) {}
  }
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

function consumeScrollPending() {
  try {
    const pending = sessionStorage.getItem(CHAT_SCROLL_PENDING_KEY) === "1";
    sessionStorage.removeItem(CHAT_SCROLL_PENDING_KEY);
    return pending;
  } catch (_) {
    return false;
  }
}

function scrollChatToLatest() {
  const chat = chatScrollEl();
  if (!chat) return;
  const top = chat.scrollHeight;
  chat.scrollTop = top;
  try {
    chat.scrollTo(0, top);
  } catch (_) {}
  chat.dataset.follow = "true";
  delete chat.dataset.scrollRestored;
  chat.dataset.pendingLatest = "false";
}

function scrollChatToLatestWithRetries() {
  const run = () => scrollChatToLatest();
  run();
  requestAnimationFrame(() => {
    run();
    requestAnimationFrame(run);
  });
  for (const ms of [50, 150, 350, 700]) {
    setTimeout(run, ms);
  }
}

function restoreChatScroll() {
  if (!isThreadRoute()) {
    pendingRestoreTop = null;
    return;
  }
  const pending = consumeScrollPending();
  if (!pending) {
    // First open / normal navigation: land on latest, ignore stale scroll.
    try {
      sessionStorage.removeItem(CHAT_SCROLL_KEY);
    } catch (_) {}
    pendingRestoreTop = null;
    return;
  }
  let saved;
  try {
    saved = JSON.parse(sessionStorage.getItem(CHAT_SCROLL_KEY) || "null");
    sessionStorage.removeItem(CHAT_SCROLL_KEY);
  } catch (_) {
    pendingRestoreTop = null;
    return;
  }
  // Never restore 0/empty — that pins Chat to the top on phones.
  if (!saved || typeof saved.top !== "number" || saved.top <= 0) {
    pendingRestoreTop = null;
    return;
  }
  // Same thread family: pathname may keep the same /threads/:id across artifacts.
  if (
    typeof saved.path === "string" &&
    saved.path.split("?")[0] !== window.location.pathname
  ) {
    pendingRestoreTop = null;
    return;
  }
  pendingRestoreTop = saved.top;
}

function applyChatScrollIntent() {
  const chat = chatScrollEl();
  if (!chat) return false;
  if (!chatRegionVisible(chat)) {
    // Defer until Chat tab reveal; keep pending restore if any.
    if (pendingRestoreTop == null) {
      chat.dataset.pendingLatest = "true";
    }
    return false;
  }
  if (pendingRestoreTop != null && pendingRestoreTop > 0) {
    chat.scrollTop = pendingRestoreTop;
    chat.dataset.follow = "false";
    chat.dataset.scrollRestored = "true";
    chat.dataset.pendingLatest = "false";
    pendingRestoreTop = null;
    return true;
  }
  pendingRestoreTop = null;
  scrollChatToLatestWithRetries();
  return true;
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
  // Prefer restoring when we left the composer focused; otherwise only
  // reclaim focus when the browser parked it on body/html after remount.
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
  persistChatContinuity({ pending: true });
  try {
    sessionStorage.setItem(COMPOSER_FOCUS_KEY, "1");
  } catch (_) {}
  markDocSwitchPending();
}

function onMobileTabClick(event) {
  const btn = event.target?.closest?.(
    '[role="tablist"][aria-label="Workbench regions"] button[role="tab"]',
  );
  if (!btn) return;
  const controls = btn.getAttribute("aria-controls") || "";
  if (controls !== "workbench-v2-chat") return;
  // After Datastar applies max-md:!flex, scroll to latest (or apply restore).
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      applyChatScrollIntent();
    });
  });
}

function bindMobileActiveObserver() {
  const root = document.getElementById("workbench-root");
  if (!root || root.dataset.chatScrollMobileBound === "true") return;
  root.dataset.chatScrollMobileBound = "true";
  const observer = new MutationObserver(() => {
    if (root.dataset.workbenchMobileActive !== "workbenchV2Chat") return;
    requestAnimationFrame(() => {
      applyChatScrollIntent();
    });
  });
  observer.observe(root, {
    attributes: true,
    attributeFilter: ["data-workbench-mobile-active"],
  });
}

function initNavPolish() {
  clearDocSwitchPending();
  restoreChatScroll();
  bindMobileActiveObserver();
  // Defer focus/scroll until after layout/VT paint so we don't fight the browser.
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      restoreComposerFocus();
      applyChatScrollIntent();
    });
  });
}

window.addEventListener("popstate", reloadThreadArtifactHistory);
window.addEventListener("pagehide", () => persistChatContinuity());
document.addEventListener("click", onArtifactFileClick, true);
document.addEventListener("click", onMobileTabClick);
initNavPolish();
