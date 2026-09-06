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

function onPageShow(event) {
  // bfcache restore: never reload; chrome is already painted.
  if (event.persisted) {
    return;
  }
  // Desktop: chat region stays mounted; autofocus can race VT/layout. Mirror mobile tab focus.
  queueMicrotask(() => document.getElementById('chat-latest')?.focus());
}

window.addEventListener("popstate", reloadThreadArtifactHistory);
window.addEventListener("pageshow", onPageShow);
