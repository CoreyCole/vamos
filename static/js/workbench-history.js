function revalidateRestoredThread(event) {
  // Only revalidate drafts restored from bfcache. Native Back/Forward to a
  // full document must stay a traverse so the browser can run View Transitions.
  if (
    event.persisted &&
    document.querySelector("#workbench-v2-chat-body #agent-chat-composer-form")
  ) {
    window.location.reload();
  }
}

function reloadThreadArtifactHistory() {
  const threadRoute =
    window.location.pathname === "/threads" ||
    window.location.pathname.startsWith("/threads/");
  if (threadRoute && document.getElementById("thread-artifact-pane")) {
    window.location.reload();
  }
}

window.addEventListener("pageshow", revalidateRestoredThread);
window.addEventListener("popstate", reloadThreadArtifactHistory);
