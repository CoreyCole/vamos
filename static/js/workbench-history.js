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

function chatOverflowScroller() {
  // Prefer #agent-chat-messages when it is the real overflow scroller; fall back
  // to #agent-chat-scroll-region (SharedThreadChat in-flow composer layout).
  // overflow-x-hidden → computed overflow-y:auto can make either scroll.
  const candidates = [
    document.getElementById("agent-chat-messages"),
    document.getElementById("agent-chat-scroll-region"),
  ].filter(Boolean);
  let best = candidates[0] || null;
  let bestOverflow = -1;
  for (const el of candidates) {
    const overflow = el.scrollHeight - el.clientHeight;
    if (overflow > bestOverflow) {
      best = el;
      bestOverflow = overflow;
    }
  }
  return best;
}

function pinChatToBottom() {
  const region = chatOverflowScroller();
  const latest = document.getElementById("chat-latest");
  if (region) {
    region.scrollTop = region.scrollHeight;
    // Prefer anchoring the SSR sentinel inside the scroller (not the window).
    latest?.scrollIntoView({ block: "end", inline: "nearest" });
    region.scrollTop = region.scrollHeight;
  }
  // Focus may still help a11y / some engines; preventScroll so we do not undo pin.
  latest?.focus({ preventScroll: true });
}

function settleChatPin() {
  pinChatToBottom();
  // Double rAF: first frame applies scrollTop; second catches post-layout growth
  // (fonts, images, SSE morph) before the user sees leftover room.
  requestAnimationFrame(() => {
    pinChatToBottom();
    requestAnimationFrame(pinChatToBottom);
  });
}

function pinAfterFonts() {
  settleChatPin();
  const fonts = document.fonts;
  if (fonts?.ready) {
    fonts.ready.then(settleChatPin, settleChatPin);
  }
}

function scheduleChatPinAfterReveal(event) {
  // After cross-document VT, wait for finished so pin does not fight the old snapshot.
  const run = pinAfterFonts;
  const finished = event?.viewTransition?.finished;
  if (finished) {
    finished.then(run, run);
    return;
  }
  queueMicrotask(run);
}

function scheduleChatPinOnPageshow(event) {
  // Full refresh / non-bfcache navigations: pagereveal may fire before layout
  // settles; pageshow(!persisted) re-pins after the document is shown.
  if (event?.persisted) {
    return;
  }
  pinAfterFonts();
}

const WB2_VT_THREAD_SWITCH = "thread-switch";
const WB2_VT_THREAD_SWITCH_FLAG = "wb2-vt-thread-switch";

function threadIdFromURL(urlLike) {
  try {
    const u = new URL(urlLike, window.location.origin);
    const parts = u.pathname.split("/");
    if (parts[1] === "threads" && parts[2]) {
      return decodeURIComponent(parts[2]);
    }
  } catch (_) {}
  return null;
}

function isThreadToThreadNavigation(fromURL, toURL) {
  const fromId = threadIdFromURL(fromURL);
  const toId = threadIdFromURL(toURL);
  return Boolean(fromId && toId && fromId !== toId);
}

function setThreadSwitchChatUnname() {
  document.documentElement.setAttribute("data-wb2-vt-nav", WB2_VT_THREAD_SWITCH);
}

function clearThreadSwitchChatUnname() {
  document.documentElement.removeAttribute("data-wb2-vt-nav");
}

function clearThreadSwitchChatUnnameAfter(vt) {
  const clear = clearThreadSwitchChatUnname;
  if (vt?.finished) {
    vt.finished.then(clear, clear);
    return;
  }
  queueMicrotask(clear);
}

// Cross-document VT: unname #workbench-v2-chat only on thread→thread GETs.
// Sibling artifact GETs (same thread id) keep CSS name + freeze. Do not
// display:none ::view-transition-new(workbench-v2-chat).
function armThreadSwitchChatUnname(fromURL, toURL, vt) {
  if (!isThreadToThreadNavigation(fromURL, toURL)) {
    return false;
  }
  setThreadSwitchChatUnname();
  try {
    sessionStorage.setItem(WB2_VT_THREAD_SWITCH_FLAG, "1");
  } catch (_) {}
  if (vt) {
    clearThreadSwitchChatUnnameAfter(vt);
  }
  return true;
}

try {
  if (sessionStorage.getItem(WB2_VT_THREAD_SWITCH_FLAG) === "1") {
    sessionStorage.removeItem(WB2_VT_THREAD_SWITCH_FLAG);
    setThreadSwitchChatUnname();
  }
} catch (_) {}

if ("onpageswap" in window) {
  window.addEventListener("pageswap", (event) => {
    if (!event.viewTransition) {
      return;
    }
    const from = event.activation?.from?.url || window.location.href;
    const to = event.activation?.entry?.url;
    if (!to) {
      return;
    }
    armThreadSwitchChatUnname(from, to, event.viewTransition);
  });
}

// Capture-phase fallback when pageswap activation URLs are unavailable:
// thread sidebar anchors that change /threads/:id.
document.addEventListener(
  "click",
  (event) => {
    if (event.defaultPrevented || event.button !== 0) {
      return;
    }
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
      return;
    }
    const anchor = event.target?.closest?.(
      "#workbench-v2-threads a[href], #workbench-v2-threads-body a[href]",
    );
    if (!anchor) {
      return;
    }
    if (!isThreadToThreadNavigation(window.location.href, anchor.href)) {
      return;
    }
    setThreadSwitchChatUnname();
    try {
      sessionStorage.setItem(WB2_VT_THREAD_SWITCH_FLAG, "1");
    } catch (_) {}
  },
  true,
);

window.addEventListener("popstate", reloadThreadArtifactHistory);

function scheduleThreadSwitchChatUnnameOnReveal(event) {
  // New document snapshot: keep chat unnamed for thread→thread VT, then restore
  // CSS name after finished so the next sibling-artifact GET freezes chat again.
  const nav = window.navigation;
  const from = nav?.activation?.from?.url;
  const to = nav?.activation?.entry?.url || window.location.href;
  if (from && event?.viewTransition && isThreadToThreadNavigation(from, to)) {
    setThreadSwitchChatUnname();
    clearThreadSwitchChatUnnameAfter(event.viewTransition);
    try {
      sessionStorage.removeItem(WB2_VT_THREAD_SWITCH_FLAG);
    } catch (_) {}
    return;
  }
  if (document.documentElement.getAttribute("data-wb2-vt-nav") === WB2_VT_THREAD_SWITCH) {
    clearThreadSwitchChatUnnameAfter(event?.viewTransition);
  }
}

if ("onpagereveal" in window) {
  window.addEventListener("pagereveal", (event) => {
    scheduleThreadSwitchChatUnnameOnReveal(event);
    scheduleChatPinAfterReveal(event);
  });
} else {
  // No pagereveal: one-shot settle only (pageshow still reinforces).
  if (document.documentElement.getAttribute("data-wb2-vt-nav") === WB2_VT_THREAD_SWITCH) {
    queueMicrotask(clearThreadSwitchChatUnname);
  }
  queueMicrotask(pinAfterFonts);
}
window.addEventListener("pageshow", scheduleChatPinOnPageshow);
