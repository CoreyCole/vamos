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

window.addEventListener("popstate", reloadThreadArtifactHistory);
if ("onpagereveal" in window) {
  window.addEventListener("pagereveal", scheduleChatPinAfterReveal);
} else {
  // No pagereveal: one-shot settle only (pageshow still reinforces).
  queueMicrotask(pinAfterFonts);
}
window.addEventListener("pageshow", scheduleChatPinOnPageshow);
