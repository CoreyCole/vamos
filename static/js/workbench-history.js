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

function pinChatToBottom() {
  const region = document.getElementById("agent-chat-scroll-region");
  if (region) {
    region.scrollTop = region.scrollHeight;
  }
  // Focus may still help a11y / some engines; preventScroll so we do not undo pin.
  document.getElementById("chat-latest")?.focus({ preventScroll: true });
}

function scheduleChatPinAfterReveal(event) {
  // After cross-document VT, wait for finished so pin does not fight the old snapshot.
  const finished = event?.viewTransition?.finished;
  if (finished) {
    finished.then(pinChatToBottom, pinChatToBottom);
    return;
  }
  queueMicrotask(pinChatToBottom);
}

window.addEventListener("popstate", reloadThreadArtifactHistory);
if ("onpagereveal" in window) {
  window.addEventListener("pagereveal", scheduleChatPinAfterReveal);
} else {
  // No pagereveal: one-shot microtask only (avoid pairing with another reveal hook).
  queueMicrotask(pinChatToBottom);
}
