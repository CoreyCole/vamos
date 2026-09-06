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

function focusChatLatest() {
  document.getElementById("chat-latest")?.focus();
}

function scheduleChatLatestFocus(event) {
  // After cross-document VT, wait for finished so focus does not fight the old snapshot.
  const finished = event?.viewTransition?.finished;
  if (finished) {
    finished.then(focusChatLatest, focusChatLatest);
    return;
  }
  queueMicrotask(focusChatLatest);
}

window.addEventListener("popstate", reloadThreadArtifactHistory);
if ("onpagereveal" in window) {
  window.addEventListener("pagereveal", scheduleChatLatestFocus);
} else {
  // No pagereveal: one-shot microtask only (avoid pairing with another reveal hook).
  queueMicrotask(focusChatLatest);
}
